package main

import (
	"archive/zip"
	"bufio"
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeSelfExtractor mimics build-release.ps1: arbitrary executable bytes
// followed by the release ZIP.
func writeSelfExtractor(t *testing.T, entries map[string]string) string {
	t.Helper()
	var archive bytes.Buffer
	writer := zip.NewWriter(&archive)
	for name, content := range entries {
		entry, err := writer.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := entry.Write([]byte(content)); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "setup.exe")
	stub := bytes.Repeat([]byte("MZ stub "), 4096)
	if err := os.WriteFile(path, append(stub, archive.Bytes()...), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestExtractPackageReadsArchiveAppendedToExecutable(t *testing.T) {
	path := writeSelfExtractor(t, map[string]string{
		"Package/deploy.ps1":            "deploy",
		"Package/安裝.cmd":                "launcher",
		"Package/payload/nested/a.json": "{}",
	})
	root, err := extractPackage(path, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Base(root) != "Package" {
		t.Fatalf("unexpected package root: %s", root)
	}
	for name, want := range map[string]string{"deploy.ps1": "deploy", "安裝.cmd": "launcher", `payload\nested\a.json`: "{}"} {
		got, err := os.ReadFile(filepath.Join(root, name))
		if err != nil || string(got) != want {
			t.Fatalf("%s = %q, %v", name, got, err)
		}
	}
}

func TestExtractPackageRejectsUnsafeArchives(t *testing.T) {
	for name, entries := range map[string]map[string]string{
		"parent traversal": {"Package/a.txt": "a", "Package/../../evil.txt": "x"},
		"absolute path":    {"/evil.txt": "x"},
		"drive path":       {"C:/evil.txt": "x"},
		"two roots":        {"One/a.txt": "a", "Two/b.txt": "b"},
	} {
		t.Run(name, func(t *testing.T) {
			destination := t.TempDir()
			if _, err := extractPackage(writeSelfExtractor(t, entries), destination); err == nil {
				t.Fatal("unsafe archive was accepted")
			}
			if _, err := os.Stat(filepath.Join(filepath.Dir(destination), "evil.txt")); err == nil {
				t.Fatal("a file escaped the destination")
			}
		})
	}
}

func TestExtractPackageRejectsExecutableWithoutPayload(t *testing.T) {
	path := filepath.Join(t.TempDir(), "setup.exe")
	if err := os.WriteFile(path, []byte("MZ no payload"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := extractPackage(path, t.TempDir()); err == nil {
		t.Fatal("an executable without a payload was accepted")
	}
}

func TestParseOptionsRejectsInvalidCombinations(t *testing.T) {
	for _, args := range [][]string{
		{"-remove-data"},
		{"-uninstall", "-safe-disable"},
		{"-install", "-uninstall"},
		{"stray"},
		{"-unknown"},
		{"-extract", ""},
		{"-extract", "  "},
		{"-extract", "-uninstall"},
		{"-extract", "out", "-uninstall"},
		{"-extract", "out", "-safe-disable"},
		{"-startup-manager", "-uninstall"},
		{"-startup-manager", "-extract", "out"},
	} {
		if _, err := parseOptions(args, io.Discard); err == nil {
			t.Fatalf("accepted %s", strings.Join(args, " "))
		}
	}
	for _, valid := range []struct {
		args   []string
		action action
	}{
		{nil, actNone},
		{[]string{"-install"}, actInstall},
		{[]string{"-uninstall"}, actUninstall},
		{[]string{"-uninstall", "-remove-data"}, actUninstallRemoveData},
		{[]string{"-safe-disable"}, actSafeDisable},
		{[]string{"-startup-manager"}, actManager},
		{[]string{"-extract", "out"}, actExtract},
	} {
		opts, err := parseOptions(append(valid.args, "-no-pause"), io.Discard)
		if err != nil || opts.action != valid.action || !opts.noPause {
			t.Fatalf("%v: got %+v, %v", valid.args, opts, err)
		}
	}
}

// menuRun feeds scripted answers to the menu and records what it would run.
func menuRun(answers string) (actions []action, folders []string, output string) {
	var out strings.Builder
	runMenu(bufio.NewReader(strings.NewReader(answers)), &out, func() string { return "status" }, `C:\default`, func(selected action, folder string) int {
		actions = append(actions, selected)
		folders = append(folders, folder)
		return 0
	})
	return actions, folders, out.String()
}

func TestMenuRunsTheChosenActionAndReturnsToTheMenu(t *testing.T) {
	actions, _, output := menuRun("2\n\n1\n\n0\n")
	if len(actions) != 2 || actions[0] != actManager || actions[1] != actInstall {
		t.Fatalf("unexpected actions: %v", actions)
	}
	if strings.Count(output, "請輸入數字") != 3 {
		t.Fatalf("menu was not shown again after each action:\n%s", output)
	}
}

func TestMenuIgnoresUnknownAnswersAndStopsAtEndOfInput(t *testing.T) {
	actions, _, output := menuRun("9\nabc\n")
	if len(actions) != 0 || !strings.Contains(output, "沒有這個選項") {
		t.Fatalf("unknown answers ran an action: %v\n%s", actions, output)
	}
}

func TestMenuRemoveDataNeedsConfirmation(t *testing.T) {
	if actions, _, _ := menuRun("5\nn\n0\n"); len(actions) != 0 {
		t.Fatalf("data was removed without confirmation: %v", actions)
	}
	if actions, _, _ := menuRun("5\n\n0\n"); len(actions) != 0 {
		t.Fatalf("an empty answer confirmed data removal: %v", actions)
	}
	if actions, _, _ := menuRun("5\ny\n\n0\n"); len(actions) != 1 || actions[0] != actUninstallRemoveData {
		t.Fatalf("confirmed removal did not run: %v", actions)
	}
}

func TestMenuExtractUsesDefaultOrTypedFolder(t *testing.T) {
	_, folders, _ := menuRun("6\n\n\n6\n\"D:\\out\"\n\n0\n")
	if len(folders) != 2 || folders[0] != `C:\default` || folders[1] != `D:\out` {
		t.Fatalf("unexpected folders: %q", folders)
	}
}

func TestStatusReadsPackagedAndInstalledVersions(t *testing.T) {
	path := writeSelfExtractor(t, map[string]string{
		"Package/payload/codex-auto-retry/.codex-plugin/plugin.json": `{"version":"9.9.9+test"}`,
	})
	if version, err := packagedVersion(path); err != nil || version != "9.9.9+test" {
		t.Fatalf("packaged version = %q, %v", version, err)
	}
	profile := t.TempDir()
	t.Setenv("USERPROFILE", profile)
	if status := describeStatus(path); !strings.Contains(status, "9.9.9+test") || !strings.Contains(status, "尚未安裝") {
		t.Fatalf("unexpected status before install: %s", status)
	}
	manifest := filepath.Join(profile, "plugins", "codex-auto-retry", ".codex-plugin")
	if err := os.MkdirAll(manifest, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(manifest, "plugin.json"), []byte(`{"version":"1.0.0"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if status := describeStatus(path); !strings.Contains(status, "已安裝 1.0.0") {
		t.Fatalf("unexpected status after install: %s", status)
	}
}

func TestWindowsPowerShellEnvironmentDropsInheritedModulePath(t *testing.T) {
	// PowerShell 7 puts its own module folders first in PSModulePath. Windows
	// PowerShell 5.1 then loads the 7.x Utility module and loses Get-FileHash.
	got := windowsPowerShellEnvironment([]string{
		`PATH=C:\Windows`,
		`PSModulePath=C:\Program Files\PowerShell\Modules`,
		`psmodulepath=C:\other`,
		`PSModulePathExtra=keep`,
	})
	want := []string{`PATH=C:\Windows`, `PSModulePathExtra=keep`}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("environment = %q, want %q", got, want)
	}
}
