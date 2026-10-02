// Command codex-auto-retry-setup is the single-file Windows installer.
//
// build-release.ps1 appends the release ZIP to this executable. At run time the
// program reads that ZIP from its own file, extracts it to a private temporary
// folder, and hands over to the PowerShell scripts shipped in the package, so
// the integrity checks and rollback logic stay in one place (deploy.ps1).
package main

import (
	"archive/zip"
	"bufio"
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

type options struct {
	uninstall   bool
	removeData  bool
	safeDisable bool
	extractTo   string
	extractOnly bool
	noPause     bool
}

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	opts, err := parseOptions(args, stderr)
	if err != nil {
		return exitUsage
	}
	code := execute(opts, stdout, stderr)
	if !opts.noPause && !opts.extractOnly {
		fmt.Fprintln(stdout)
		fmt.Fprint(stdout, "Press Enter to close this window...")
		_, _ = bufio.NewReader(os.Stdin).ReadString('\n')
	}
	return code
}

func parseOptions(args []string, stderr io.Writer) (options, error) {
	var opts options
	flags := flag.NewFlagSet("codex-auto-retry-setup", flag.ContinueOnError)
	flags.SetOutput(stderr)
	flags.BoolVar(&opts.uninstall, "uninstall", false, "uninstall the watchdog and plugin, keeping settings and logs")
	flags.BoolVar(&opts.removeData, "remove-data", false, "with -uninstall, also delete settings, state and logs")
	flags.BoolVar(&opts.safeDisable, "safe-disable", false, "disable the shared backend and return Codex to the official direct mode")
	flags.StringVar(&opts.extractTo, "extract", "", "only extract the release package into `folder`")
	flags.BoolVar(&opts.noPause, "no-pause", false, "do not wait for Enter before exiting")
	if err := flags.Parse(args); err != nil {
		return opts, err
	}
	if flags.NArg() > 0 {
		fmt.Fprintf(stderr, "unexpected argument: %s\n", flags.Arg(0))
		return opts, errors.New("unexpected argument")
	}
	if opts.removeData && !opts.uninstall {
		fmt.Fprintln(stderr, "-remove-data requires -uninstall")
		return opts, errors.New("invalid flags")
	}
	if opts.uninstall && opts.safeDisable {
		fmt.Fprintln(stderr, "-uninstall and -safe-disable cannot be combined")
		return opts, errors.New("invalid flags")
	}
	flags.Visit(func(f *flag.Flag) {
		if f.Name == "extract" {
			opts.extractOnly = true
		}
	})
	if opts.extractOnly && (strings.TrimSpace(opts.extractTo) == "" || strings.HasPrefix(opts.extractTo, "-")) {
		// An empty folder must never fall through to a real installation, and
		// a value that looks like a flag means the folder was forgotten.
		fmt.Fprintln(stderr, "-extract requires a folder")
		return opts, errors.New("invalid flags")
	}
	if opts.extractOnly && (opts.uninstall || opts.safeDisable) {
		fmt.Fprintln(stderr, "-extract cannot be combined with -uninstall or -safe-disable")
		return opts, errors.New("invalid flags")
	}
	return opts, nil
}

func execute(opts options, stdout, stderr io.Writer) int {
	self, err := os.Executable()
	if err != nil {
		fmt.Fprintln(stderr, "Cannot locate the installer file:", err)
		return exitInternal
	}

	if opts.extractOnly {
		root, err := extractPackage(self, opts.extractTo)
		if err != nil {
			fmt.Fprintln(stderr, "Extraction failed:", err)
			return exitInternal
		}
		fmt.Fprintln(stdout, root)
		return 0
	}

	workDir, err := os.MkdirTemp("", "codex-auto-retry-setup-")
	if err != nil {
		fmt.Fprintln(stderr, "Cannot create a temporary folder:", err)
		return exitInternal
	}
	defer os.RemoveAll(workDir)

	fmt.Fprintln(stdout, "Codex Auto Retry - single-file installer")
	fmt.Fprintln(stdout, "Extracting the release package...")
	root, err := extractPackage(self, workDir)
	if err != nil {
		fmt.Fprintln(stderr, "Extraction failed:", err)
		return exitInternal
	}

	script, scriptArgs := "deploy.ps1", []string{"-WaitForCodexExit"}
	switch {
	case opts.uninstall:
		script, scriptArgs = "uninstall-release.ps1", nil
		if opts.removeData {
			scriptArgs = []string{"-RemoveData"}
		}
	case opts.safeDisable:
		script, scriptArgs = "startup-manager.ps1", []string{"-Action", "safe-disable"}
	}

	code, err := runPowerShell(filepath.Join(root, script), scriptArgs, stdout, stderr)
	if err != nil {
		fmt.Fprintln(stderr, "Cannot start Windows PowerShell:", err)
		return exitInternal
	}
	fmt.Fprintln(stdout)
	switch {
	case code == 0:
		fmt.Fprintln(stdout, "Completed successfully.")
	case code == 2 && script == "deploy.ps1":
		fmt.Fprintln(stdout, "Installation cancelled. No plugin or runtime changes were made.")
	default:
		fmt.Fprintf(stdout, "Failed with exit code %d. Review the error above.\n", code)
	}
	return code
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

func runPowerShell(script string, args []string, stdout, stderr io.Writer) (int, error) {
	systemRoot := os.Getenv("SystemRoot")
	if systemRoot == "" {
		return 0, errors.New("SystemRoot is not set")
	}
	powershell := filepath.Join(systemRoot, "System32", "WindowsPowerShell", "v1.0", "powershell.exe")
	command := exec.Command(powershell, append([]string{
		"-NoLogo", "-NoProfile", "-ExecutionPolicy", "Bypass", "-File", script,
	}, args...)...)
	command.Stdin = os.Stdin
	command.Stdout = stdout
	command.Stderr = stderr

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
