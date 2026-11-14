//go:build windows

package main

import (
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"
	"testing"
	"time"

	"golang.org/x/sys/windows/registry"
)

// testEnvironmentRoot holds one scratch key per test process. Tests read and
// write user environment values there instead of the live HKCU\Environment.
const testEnvironmentRoot = `Software\codex-auto-retry-tests`

// liveTestEnvironmentPrefix covers every name Go tests and PowerShell smoke
// tests generate (CODEX_AUTO_RETRY_TEST_*, CODEX_AUTO_RETRY_ENV_TEST_*).
const liveTestEnvironmentPrefix = "CODEX_AUTO_RETRY_"

// Even an interrupted test suite must never change the live Desktop route or
// leave values behind in the live user environment.
func TestMain(m *testing.M) {
	id := fmt.Sprintf("%d_%d", os.Getpid(), time.Now().UnixNano())
	sharedAppServerEnvironmentName = "CODEX_AUTO_RETRY_TEST_" + id
	before, err := liveTestEnvironmentNames()
	if err != nil {
		fmt.Fprintln(os.Stderr, "live user environment could not be read:", err)
		os.Exit(1)
	}
	userEnvironmentSubKey = testEnvironmentRoot + `\` + id
	broadcastEnvironmentChange = func() {}

	code := m.Run()

	if err := registry.DeleteKey(registry.CURRENT_USER, userEnvironmentSubKey); err != nil && !errors.Is(err, registry.ErrNotExist) {
		fmt.Fprintln(os.Stderr, "test environment cleanup failed:", err)
		code = 1
	}
	// Fails while another test process still owns a scratch key; that is fine.
	_ = registry.DeleteKey(registry.CURRENT_USER, testEnvironmentRoot)

	after, err := liveTestEnvironmentNames()
	if err != nil {
		fmt.Fprintln(os.Stderr, "live user environment could not be re-read:", err)
		code = 1
	}
	var leaked []string
	for name := range after {
		if !before[name] {
			leaked = append(leaked, name)
		}
	}
	if len(leaked) > 0 {
		sort.Strings(leaked)
		fmt.Fprintln(os.Stderr, "tests leaked values into the live HKCU\\Environment:", strings.Join(leaked, ", "))
		code = 1
	}
	os.Exit(code)
}

// liveTestEnvironmentNames reads the real user environment key directly, not
// through userEnvironmentSubKey, so the leak check cannot be redirected.
func liveTestEnvironmentNames() (map[string]bool, error) {
	names := map[string]bool{}
	key, err := registry.OpenKey(registry.CURRENT_USER, `Environment`, registry.QUERY_VALUE)
	if errors.Is(err, registry.ErrNotExist) {
		return names, nil
	}
	if err != nil {
		return nil, err
	}
	defer key.Close()
	all, err := key.ReadValueNames(-1)
	if err != nil {
		return nil, err
	}
	for _, name := range all {
		if strings.HasPrefix(strings.ToUpper(name), liveTestEnvironmentPrefix) {
			names[name] = true
		}
	}
	return names, nil
}

func TestUserEnvironmentIsIsolatedDuringTests(t *testing.T) {
	if !strings.HasPrefix(userEnvironmentSubKey, testEnvironmentRoot+`\`) {
		t.Fatalf("tests use the live user environment key %q", userEnvironmentSubKey)
	}
	name := fmt.Sprintf("CODEX_AUTO_RETRY_TEST_ISOLATION_%d", time.Now().UnixNano())
	t.Cleanup(func() { _ = restoreUserEnvironment(name, "", false) })
	if err := writeUserEnvironment(name, "ws://127.0.0.1:1"); err != nil {
		t.Fatal(err)
	}
	live, err := liveTestEnvironmentNames()
	if err != nil {
		t.Fatal(err)
	}
	if live[name] {
		t.Fatal("writeUserEnvironment reached the live HKCU\\Environment")
	}
}

func TestPersistentProductionRouteCannotBePublished(t *testing.T) {
	for _, name := range []string{"CODEX_APP_SERVER_WS_URL", "codex_app_server_ws_url"} {
		if _, err := setOwnedSharedEnvironmentNamed(t.TempDir(), name, "ws://127.0.0.1:49622"); err == nil {
			t.Fatal("production routing publication was not rejected")
		}
	}
}
