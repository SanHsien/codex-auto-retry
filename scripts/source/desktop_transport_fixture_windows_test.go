//go:build windows

package main

import "testing"

func TestDesktopTransportClientEvidence(t *testing.T) {
	exe := `C:\Program Files\WindowsApps\OpenAI.Codex_test\app\ChatGPT.exe`
	process := func(id, parent int, name, path, cmd string) map[string]any {
		return map[string]any{"ProcessId": id, "ParentProcessId": parent, "Name": name, "ExecutablePath": path, "CommandLine": cmd}
	}
	main := process(10, 1, "ChatGPT.exe", exe, `"`+exe+`"`)
	network := process(11, 10, "ChatGPT.exe", exe, "--type=utility --utility-sub-type=network.mojom.NetworkService")
	for _, tc := range []struct {
		name   string
		owner  int
		legacy bool
		want   string
	}{
		{"listener_only", 0, false, "unknown"},
		{"official_plus_unrelated_listener", 0, true, "legacy_stdio"},
		{"desktop_connected_with_stdio_child", 10, true, "shared_server"},
		{"desktop_connected", 10, false, "shared_server"},
		{"network_child_connected", 11, false, "shared_server"},
		{"arbitrary_task_client", 99, false, "unknown"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			processes := []map[string]any{main, network, process(200, 500, "codex.exe", `C:\codex.exe`, "app-server --listen ws://127.0.0.1:49622")}
			if tc.legacy {
				processes = append(processes, process(201, 10, "codex.exe", `C:\codex.exe`, "app-server"))
			}
			connections := []map[string]any{}
			if tc.owner != 0 {
				connections = append(connections, map[string]any{"OwningProcess": tc.owner, "RemoteAddress": "127.0.0.1", "RemotePort": 49622})
			}
			if got := runTransportFixture(t, processes, connections, 49622); got != tc.want {
				t.Fatalf("got %q want %s", got, tc.want)
			}
		})
	}
}

func TestDesktopTransportAcceptsCodexDesktopExecutable(t *testing.T) {
	exe := `C:\Program Files\WindowsApps\OpenAI.Codex_test\app\Codex.exe`
	process := func(id, parent int, name, path, cmd string) map[string]any {
		return map[string]any{"ProcessId": id, "ParentProcessId": parent, "Name": name, "ExecutablePath": path, "CommandLine": cmd}
	}
	processes := []map[string]any{
		process(10, 1, "Codex.exe", exe, `"`+exe+`"`),
		process(200, 500, "codex.exe", `C:\codex.exe`, "app-server --listen ws://127.0.0.1:49622"),
	}
	connections := []map[string]any{{"OwningProcess": 10, "RemoteAddress": "127.0.0.1", "RemotePort": 49622}}
	if got := runTransportFixture(t, processes, connections, 49622); got != "shared_server" {
		t.Fatalf("got %q want shared_server", got)
	}
}
