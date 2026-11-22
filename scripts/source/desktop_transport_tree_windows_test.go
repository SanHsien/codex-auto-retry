//go:build windows

package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

type transportTreeFixture struct {
	Processes   []map[string]any `json:"processes"`
	Connections []map[string]any `json:"connections"`
}

const (
	transportTreeNewShape = "desktop_process_tree_26.1117.4957.0.json"
	transportTreeOldShape = "desktop_process_tree_pre_26.1117.json"
	transportTreeTestPort = 49622

	// Process ids in the captured 26.1117.4957.0 tree.
	treeMainPID       = 55000
	treeNodeHostPID   = 13328
	treeOtherNodePID  = 9456
	treeNetworkPID    = 50660
	treeRendererPID   = 16456
	treeAppServerPID  = 48584
	treeExecServerPID = 55868
)

func loadTransportTreeFixture(t *testing.T, name string) transportTreeFixture {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	var fixture transportTreeFixture
	if err := json.Unmarshal(raw, &fixture); err != nil {
		t.Fatal(err)
	}
	if len(fixture.Processes) == 0 {
		t.Fatalf("fixture %s has no processes", name)
	}
	if fixture.Connections == nil {
		fixture.Connections = []map[string]any{}
	}
	return fixture
}

// runTransportTreeProbe runs the production classifier against a fixture.
// Only the two system reads (process tree, ESTABLISHED TCP table) are replaced.
func runTransportTreeProbe(t *testing.T, fixture transportTreeFixture) string {
	t.Helper()
	return runTransportFixture(t, fixture.Processes, fixture.Connections, transportTreeTestPort)
}

func transportTreeProcess(t *testing.T, fixture transportTreeFixture, id int) map[string]any {
	t.Helper()
	for _, process := range fixture.Processes {
		if pid, ok := process["ProcessId"].(float64); ok && int(pid) == id {
			return process
		}
	}
	t.Fatalf("fixture has no process %d", id)
	return nil
}

// Desktop 26.1117 starts app-server from its NodeService utility instead of
// the main process. The probe used to answer "unknown" for that tree, so
// preflight never asked whether official IPC was reachable and the status
// described the Desktop transport as unproven.
func TestDesktopTransportRecognisesAppServerUnderNodeServiceHost(t *testing.T) {
	fixture := loadTransportTreeFixture(t, transportTreeNewShape)
	if got := runTransportTreeProbe(t, fixture); got != "legacy_stdio" {
		t.Fatalf("captured 26.1117.4957.0 tree classified as %q, want legacy_stdio", got)
	}
}

// The shape every Desktop before 26.1117 had must keep its classification.
func TestDesktopTransportStillRecognisesAppServerUnderMainProcess(t *testing.T) {
	fixture := loadTransportTreeFixture(t, transportTreeOldShape)
	if got := runTransportTreeProbe(t, fixture); got != "legacy_stdio" {
		t.Fatalf("pre-26.1117 tree classified as %q, want legacy_stdio", got)
	}
}

// Walking up through a utility must not turn unrelated processes into Desktop
// app-server owners. Each case rewires the captured tree so the app-server is
// no longer under a verified Desktop host, and expects "unknown".
func TestDesktopTransportRejectsAppServerUnderUnrelatedParents(t *testing.T) {
	otherElectron := `C:\Program Files\Other\ChatGPT.exe`
	for _, tc := range []struct {
		name   string
		mutate func(t *testing.T, fixture *transportTreeFixture)
	}{
		{"node_service_of_another_executable", func(t *testing.T, f *transportTreeFixture) {
			host := transportTreeProcess(t, *f, treeNodeHostPID)
			host["ExecutablePath"] = otherElectron
			host["CommandLine"] = `"` + otherElectron + `" --type=utility --utility-sub-type=node.mojom.NodeService`
		}},
		{"node_service_not_a_child_of_main", func(t *testing.T, f *transportTreeFixture) {
			transportTreeProcess(t, *f, treeNodeHostPID)["ParentProcessId"] = 4
		}},
		{"node_service_under_another_utility", func(t *testing.T, f *transportTreeFixture) {
			transportTreeProcess(t, *f, treeNodeHostPID)["ParentProcessId"] = treeOtherNodePID
		}},
		{"parent_is_renderer", func(t *testing.T, f *transportTreeFixture) {
			transportTreeProcess(t, *f, treeAppServerPID)["ParentProcessId"] = treeRendererPID
		}},
		{"parent_is_network_service", func(t *testing.T, f *transportTreeFixture) {
			transportTreeProcess(t, *f, treeAppServerPID)["ParentProcessId"] = treeNetworkPID
		}},
		{"parent_is_exec_server", func(t *testing.T, f *transportTreeFixture) {
			transportTreeProcess(t, *f, treeAppServerPID)["ParentProcessId"] = treeExecServerPID
		}},
		{"parent_is_plain_node", func(t *testing.T, f *transportTreeFixture) {
			f.Processes = append(f.Processes, map[string]any{
				"ProcessId": 70000, "ParentProcessId": treeMainPID, "Name": "node.exe",
				"ExecutablePath": `C:\Program Files\nodejs\node.exe`,
				"CommandLine":    `node.exe server.js --type=utility --utility-sub-type=node.mojom.NodeService`,
			})
			transportTreeProcess(t, *f, treeAppServerPID)["ParentProcessId"] = 70000
		}},
		{"sub_type_only_inside_another_argument", func(t *testing.T, f *transportTreeFixture) {
			host := transportTreeProcess(t, *f, treeNodeHostPID)
			host["CommandLine"] = `"` + host["ExecutablePath"].(string) + `" --type=renderer --note=--utility-sub-type=node.mojom.NodeService`
		}},
		{"standalone_cli_app_server", func(t *testing.T, f *transportTreeFixture) {
			transportTreeProcess(t, *f, treeAppServerPID)["ParentProcessId"] = 4
		}},
		{"child_of_host_is_not_app_server", func(t *testing.T, f *transportTreeFixture) {
			transportTreeProcess(t, *f, treeAppServerPID)["CommandLine"] = `codex.exe exec-server --concurrent-requests 32`
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fixture := loadTransportTreeFixture(t, transportTreeNewShape)
			tc.mutate(t, &fixture)
			if got := runTransportTreeProbe(t, fixture); got != "unknown" {
				t.Fatalf("got %q, want unknown", got)
			}
		})
	}
}

// With a shared backend the Desktop-side WebSocket client on 26.1117 is the
// app-server host utility. A connection from it counts; a connection from any
// other process to the same port does not.
func TestDesktopTransportSharedConnectionOwnerOnNodeServiceTree(t *testing.T) {
	for _, tc := range []struct {
		name   string
		owner  int
		mutate func(t *testing.T, fixture *transportTreeFixture)
		want   string
	}{
		{"app_server_host_utility", treeNodeHostPID, nil, "shared_server"},
		{"main_process", treeMainPID, nil, "shared_server"},
		{"renderer_is_not_a_client", treeRendererPID, nil, "legacy_stdio"},
		{"exec_server_is_not_a_client", treeExecServerPID, nil, "legacy_stdio"},
		{"unrelated_process_is_not_a_client", 4242, nil, "legacy_stdio"},
		{"node_service_of_another_executable", treeNodeHostPID, func(t *testing.T, f *transportTreeFixture) {
			transportTreeProcess(t, *f, treeNodeHostPID)["ExecutablePath"] = `C:\Program Files\Other\ChatGPT.exe`
		}, "unknown"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fixture := loadTransportTreeFixture(t, transportTreeNewShape)
			if tc.mutate != nil {
				tc.mutate(t, &fixture)
			}
			fixture.Connections = []map[string]any{{
				"OwningProcess": tc.owner, "RemoteAddress": "127.0.0.1", "RemotePort": transportTreeTestPort,
			}}
			if got := runTransportTreeProbe(t, fixture); got != tc.want {
				t.Fatalf("got %q, want %s", got, tc.want)
			}
		})
	}
}
