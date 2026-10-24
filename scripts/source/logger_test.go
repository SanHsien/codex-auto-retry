package main

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestLoggerRotatesWithinBoundedBackupCount(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "daemon.log")
	if err := os.WriteFile(path, []byte(strings.Repeat("x", maxLogBytes+1)), 0o600); err != nil {
		t.Fatal(err)
	}
	for index := 1; index < maxLogFiles; index++ {
		if err := os.WriteFile(path+"."+itoa(index), []byte("backup"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	logger, err := newSafeLogger(path)
	if err != nil {
		t.Fatal(err)
	}
	defer logger.Close()
	if _, err := os.Stat(path + "." + itoa(maxLogFiles)); !os.IsNotExist(err) {
		t.Fatalf("logger created an unbounded backup: %v", err)
	}
	for index := 1; index < maxLogFiles; index++ {
		if _, err := os.Stat(path + "." + itoa(index)); err != nil {
			t.Fatalf("expected bounded backup %d: %v", index, err)
		}
	}
}

func itoa(value int) string {
	return strconv.Itoa(value)
}

func TestLoggerKeepsEachRecordOnOneLine(t *testing.T) {
	path := filepath.Join(t.TempDir(), "daemon.log")
	logger, err := newSafeLogger(path)
	if err != nil {
		t.Fatal(err)
	}
	logger.Printf("thread %s done", "abc\r\n2026/01/01 forged entry")
	if err := logger.Close(); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimRight(string(data), "\r\n"), "\n")
	if len(lines) != 1 {
		t.Fatalf("expected one log line, got %d: %q", len(lines), data)
	}
	if !strings.Contains(lines[0], `abc\r\n2026/01/01 forged entry`) {
		t.Fatalf("control characters were not escaped: %q", lines[0])
	}
}
