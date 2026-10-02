package main

import (
	"archive/zip"
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
		{"stray"},
		{"-unknown"},
		{"-extract", ""},
		{"-extract", "  "},
		{"-extract", "-uninstall"},
		{"-extract", "out", "-uninstall"},
		{"-extract", "out", "-safe-disable"},
	} {
		if _, err := parseOptions(args, io.Discard); err == nil {
			t.Fatalf("accepted %s", strings.Join(args, " "))
		}
	}
	opts, err := parseOptions([]string{"-uninstall", "-remove-data", "-no-pause"}, io.Discard)
	if err != nil || !opts.uninstall || !opts.removeData || !opts.noPause {
		t.Fatalf("valid flags were rejected: %+v, %v", opts, err)
	}
	opts, err = parseOptions([]string{"-extract", "out"}, io.Discard)
	if err != nil || !opts.extractOnly || opts.extractTo != "out" {
		t.Fatalf("extract flags were rejected: %+v, %v", opts, err)
	}
}
