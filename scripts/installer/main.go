// Command codex-auto-retry-setup is the single-file Windows installer.
//
// build-release.ps1 appends the release ZIP to this executable. At run time the
// program reads that ZIP from its own file, extracts it to a private temporary
// folder, and hands over to the PowerShell scripts shipped in the package, so
// the integrity checks and rollback logic stay in one place (deploy.ps1).
//
// Started without flags (a double-click) it shows a menu; flags run one action
// directly for terminals and shortcuts.
package main

import (
	"archive/zip"
	"bufio"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

const (
	exitUsage    = 64
	exitInternal = 70
)

type action int

const (
	actNone action = iota
	actInstall
	actManager
	actSafeDisable
	actUninstall
	actUninstallRemoveData
	actExtract
)

type options struct {
	action    action
	extractTo string
	noPause   bool
}

type menuItem struct {
	key    string
	action action
	label  string
}

var menuItems = []menuItem{
	{"1", actInstall, "安裝或更新 / Install or update"},
	{"2", actManager, "開啟啟動管理員 / Open the startup manager"},
	{"3", actSafeDisable, "緊急停用共用後端 / Safe-disable the shared backend"},
	{"4", actUninstall, "解除安裝，保留設定與日誌 / Uninstall, keep settings and logs"},
	{"5", actUninstallRemoveData, "解除安裝並刪除全部資料 / Uninstall and delete all data"},
	{"6", actExtract, "只取出整包檔案 / Extract the package only"},
	{"0", actNone, "離開 / Exit"},
}

func main() {
	os.Exit(run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr))
}

func run(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	opts, err := parseOptions(args, stderr)
	if err != nil {
		return exitUsage
	}
	self, err := os.Executable()
	if err != nil {
		fmt.Fprintln(stderr, "找不到安裝程式本身的檔案 / Cannot locate this executable:", err)
		return exitInternal
	}
	input := bufio.NewReader(stdin)
	if opts.action == actNone {
		perform := func(selected action, extractTo string) int {
			return perform(self, selected, extractTo, stdout, stderr)
		}
		status := func() string { return describeStatus(self) }
		return runMenu(input, stdout, status, defaultExtractFolder(self), perform)
	}
	code := perform(self, opts.action, opts.extractTo, stdout, stderr)
	if !opts.noPause && opts.action != actExtract && opts.action != actManager {
		fmt.Fprintln(stdout)
		fmt.Fprint(stdout, "按 Enter 關閉視窗 / Press Enter to close…")
		_, _ = input.ReadString('\n')
	}
	return code
}

func parseOptions(args []string, stderr io.Writer) (options, error) {
	var opts options
	var install, uninstall, removeData, safeDisable, manager bool
	flags := flag.NewFlagSet("Codex-Auto-Retry", flag.ContinueOnError)
	flags.SetOutput(stderr)
	flags.BoolVar(&install, "install", false, "install or update without showing the menu")
	flags.BoolVar(&uninstall, "uninstall", false, "uninstall the watchdog and plugin, keeping settings and logs")
	flags.BoolVar(&removeData, "remove-data", false, "with -uninstall, also delete settings, state and logs")
	flags.BoolVar(&safeDisable, "safe-disable", false, "disable the shared backend and return Codex to the official direct mode")
	flags.BoolVar(&manager, "startup-manager", false, "open the startup manager window (status, startup, service, uninstall)")
	flags.StringVar(&opts.extractTo, "extract", "", "only extract the release package into `folder`")
	flags.BoolVar(&opts.noPause, "no-pause", false, "do not wait for Enter before exiting")
	if err := flags.Parse(args); err != nil {
		return opts, err
	}
	if flags.NArg() > 0 {
		fmt.Fprintf(stderr, "unexpected argument: %s\n", flags.Arg(0))
		return opts, errors.New("unexpected argument")
	}
	extract := false
	flags.Visit(func(f *flag.Flag) {
		if f.Name == "extract" {
			extract = true
		}
	})
	if removeData && !uninstall {
		fmt.Fprintln(stderr, "-remove-data requires -uninstall")
		return opts, errors.New("invalid flags")
	}
	if extract && (strings.TrimSpace(opts.extractTo) == "" || strings.HasPrefix(opts.extractTo, "-")) {
		// An empty folder must never fall through to a real installation, and
		// a value that looks like a flag means the folder was forgotten.
		fmt.Fprintln(stderr, "-extract requires a folder")
		return opts, errors.New("invalid flags")
	}
	selected := 0
	for candidate, chosen := range map[action]bool{
		actInstall: install, actUninstall: uninstall, actSafeDisable: safeDisable, actManager: manager, actExtract: extract,
	} {
		if chosen {
			selected++
			opts.action = candidate
		}
	}
	if selected > 1 {
		fmt.Fprintln(stderr, "choose only one of -install, -uninstall, -safe-disable, -startup-manager and -extract")
		return opts, errors.New("invalid flags")
	}
	if opts.action == actUninstall && removeData {
		opts.action = actUninstallRemoveData
	}
	return opts, nil
}

// runMenu shows the choices until the user leaves. Destructive choices need an
// explicit confirmation, and an unreadable answer never selects anything.
func runMenu(input *bufio.Reader, out io.Writer, status func() string, extractDefault string, perform func(action, string) int) int {
	last := 0
	for {
		fmt.Fprintln(out)
		fmt.Fprintln(out, "Codex Auto Retry 安裝程式 / Installer")
		fmt.Fprintln(out, status())
		fmt.Fprintln(out)
		for _, item := range menuItems {
			fmt.Fprintf(out, "  %s. %s\n", item.key, item.label)
		}
		fmt.Fprintln(out)
		fmt.Fprint(out, "請輸入數字後按 Enter / Type a number and press Enter: ")
		answer, err := input.ReadString('\n')
		if err != nil && strings.TrimSpace(answer) == "" {
			return last
		}
		selected, ok := lookupMenu(strings.TrimSpace(answer))
		if !ok {
			fmt.Fprintln(out, "沒有這個選項，請重新輸入。 / No such option, try again.")
			continue
		}
		if selected == actNone {
			return last
		}

		extractTo := ""
		switch selected {
		case actUninstallRemoveData:
			fmt.Fprint(out, "這會刪除全部設定、狀態與日誌，無法復原。確定請輸入 Y / This deletes all settings, state and logs and cannot be undone. Type Y to confirm: ")
			confirm, _ := input.ReadString('\n')
			if !strings.EqualFold(strings.TrimSpace(confirm), "y") {
				fmt.Fprintln(out, "已取消，沒有做任何變更。 / Cancelled; nothing was changed.")
				continue
			}
		case actExtract:
			fmt.Fprintf(out, "要取出到哪個資料夾？直接按 Enter 使用 / Folder to extract to (Enter for default) %s: ", extractDefault)
			folder, _ := input.ReadString('\n')
			extractTo = strings.Trim(strings.TrimSpace(folder), `"`)
			if extractTo == "" {
				extractTo = extractDefault
			}
		}

		last = perform(selected, extractTo)
		fmt.Fprintln(out)
		fmt.Fprint(out, "按 Enter 回到選單 / Press Enter to return to the menu…")
		if _, err := input.ReadString('\n'); err != nil {
			return last
		}
	}
}

func lookupMenu(key string) (action, bool) {
	for _, item := range menuItems {
		if item.key == key {
			return item.action, true
		}
	}
	return actNone, false
}

func perform(self string, selected action, extractTo string, stdout, stderr io.Writer) int {
	if selected == actExtract {
		root, err := extractPackage(self, extractTo)
		if err != nil {
			fmt.Fprintln(stderr, "取出失敗 / Extraction failed:", err)
			return exitInternal
		}
		fmt.Fprintln(stdout, root)
		return 0
	}

	workDir, err := os.MkdirTemp("", "codex-auto-retry-setup-")
	if err != nil {
		fmt.Fprintln(stderr, "無法建立暫存資料夾 / Cannot create a temporary folder:", err)
		return exitInternal
	}
	defer os.RemoveAll(workDir)

	fmt.Fprintln(stdout, "正在解開安裝檔… / Unpacking the package…")
	root, err := extractPackage(self, workDir)
	if err != nil {
		fmt.Fprintln(stderr, "取出失敗 / Extraction failed:", err)
		return exitInternal
	}

	script, scriptArgs := "deploy.ps1", []string{"-WaitForCodexExit"}
	switch selected {
	case actUninstall:
		script, scriptArgs = "uninstall-release.ps1", nil
	case actUninstallRemoveData:
		script, scriptArgs = "uninstall-release.ps1", []string{"-RemoveData"}
	case actSafeDisable:
		script, scriptArgs = "startup-manager.ps1", []string{"-Action", "safe-disable"}
	case actManager:
		script, scriptArgs = "startup-manager.ps1", []string{"-Action", "gui"}
		// The manager hides its own console window; give it a separate one so
		// this window (or the terminal that started setup) stays visible.
		fmt.Fprintln(stdout, "已開啟啟動管理員，關閉它之後這裡會繼續。 / The startup manager is open; this window continues when you close it.")
	}

	code, err := runPowerShell(filepath.Join(root, script), scriptArgs, selected == actManager, stdout, stderr)
	if err != nil {
		fmt.Fprintln(stderr, "無法啟動 Windows PowerShell / Cannot start Windows PowerShell:", err)
		return exitInternal
	}
	fmt.Fprintln(stdout)
	switch {
	case code == 0:
		fmt.Fprintln(stdout, "完成。 / Done.")
	case code == 2 && selected == actInstall:
		fmt.Fprintln(stdout, "已取消安裝，外掛與執行環境都沒有變更。 / Installation cancelled; no plugin or runtime changes were made.")
	default:
		fmt.Fprintf(stdout, "失敗（錯誤狀態 %d），請看上方的訊息。 / Failed (exit code %[1]d); see the messages above.\n", code)
	}
	return code
}

// describeStatus compares this package with what is installed for the current
// user. It only reads files; a missing or unreadable manifest is reported as
// such instead of guessed.
func describeStatus(self string) string {
	packaged := "未知 / unknown"
	if version, err := packagedVersion(self); err == nil {
		packaged = version
	}
	installed := "尚未安裝 / not installed"
	if profile := os.Getenv("USERPROFILE"); profile != "" {
		manifest := filepath.Join(profile, "plugins", "codex-auto-retry", ".codex-plugin", "plugin.json")
		if version, err := manifestVersion(manifest); err == nil {
			installed = "已安裝 " + version + " / installed"
		} else if !errors.Is(err, os.ErrNotExist) {
			installed = "無法判斷 / unknown"
		}
	}
	return fmt.Sprintf("這個安裝檔 / This package: %s\n目前電腦上 / On this computer: %s", packaged, installed)
}

func packagedVersion(self string) (string, error) {
	reader, err := zip.OpenReader(self)
	if err != nil {
		return "", err
	}
	defer reader.Close()
	for _, file := range reader.File {
		if strings.HasSuffix(strings.ReplaceAll(file.Name, "\\", "/"), "/payload/codex-auto-retry/.codex-plugin/plugin.json") {
			source, err := file.Open()
			if err != nil {
				return "", err
			}
			defer source.Close()
			return decodeVersion(source)
		}
	}
	return "", os.ErrNotExist
}

func manifestVersion(path string) (string, error) {
	source, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer source.Close()
	return decodeVersion(source)
}

func decodeVersion(source io.Reader) (string, error) {
	var manifest struct {
		Version string `json:"version"`
	}
	if err := json.NewDecoder(source).Decode(&manifest); err != nil {
		return "", err
	}
	if manifest.Version == "" {
		return "", errors.New("manifest has no version")
	}
	return manifest.Version, nil
}

func defaultExtractFolder(self string) string {
	return filepath.Join(filepath.Dir(self), strings.TrimSuffix(filepath.Base(self), filepath.Ext(self)))
}

// extractPackage reads the ZIP appended to archivePath, extracts it under
// destination, and returns the single top-level folder it contains.
func extractPackage(archivePath, destination string) (string, error) {
	reader, err := zip.OpenReader(archivePath)
	if err != nil {
		return "", fmt.Errorf("this file does not contain a release package: %w", err)
	}
	defer reader.Close()

	base, err := filepath.Abs(destination)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(base, 0o700); err != nil {
		return "", err
	}

	topLevel := ""
	for _, file := range reader.File {
		name := strings.ReplaceAll(file.Name, "\\", "/")
		if !filepath.IsLocal(filepath.FromSlash(name)) {
			return "", fmt.Errorf("unsafe path in package: %q", file.Name)
		}
		first, _, _ := strings.Cut(name, "/")
		if topLevel == "" {
			topLevel = first
		} else if first != topLevel {
			return "", errors.New("the package must contain exactly one top-level folder")
		}
		if file.Mode()&os.ModeSymlink != 0 {
			return "", fmt.Errorf("links are not allowed in the package: %q", file.Name)
		}

		target := filepath.Join(base, filepath.FromSlash(name))
		if file.FileInfo().IsDir() || strings.HasSuffix(name, "/") {
			if err := os.MkdirAll(target, 0o700); err != nil {
				return "", err
			}
			continue
		}
		if err := extractFile(file, target); err != nil {
			return "", err
		}
	}
	if topLevel == "" {
		return "", errors.New("the release package is empty")
	}
	return filepath.Join(base, topLevel), nil
}

func extractFile(file *zip.File, target string) error {
	if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
		return err
	}
	source, err := file.Open()
	if err != nil {
		return err
	}
	defer source.Close()

	output, err := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}
	if _, err := io.Copy(output, source); err != nil {
		output.Close()
		return err
	}
	return output.Close()
}

func runPowerShell(script string, args []string, ownConsole bool, stdout, stderr io.Writer) (int, error) {
	systemRoot := os.Getenv("SystemRoot")
	if systemRoot == "" {
		return 0, errors.New("SystemRoot is not set")
	}
	powershell := filepath.Join(systemRoot, "System32", "WindowsPowerShell", "v1.0", "powershell.exe")
	command := exec.Command(powershell, append([]string{
		"-NoLogo", "-NoProfile", "-ExecutionPolicy", "Bypass", "-File", script,
	}, args...)...)
	if ownConsole {
		useHiddenConsole(command)
	} else {
		command.Stdin = os.Stdin
		command.Stdout = stdout
		command.Stderr = stderr
	}

	err := command.Run()
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return exitErr.ExitCode(), nil
	}
	if err != nil {
		return 0, err
	}
	return 0, nil
}
