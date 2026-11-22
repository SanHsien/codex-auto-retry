//go:build windows

package main

import (
	"context"
	"encoding/binary"
	"errors"
	"strconv"
	"testing"
	"time"
)

func fixtureUint32(t *testing.T, value any) uint32 {
	t.Helper()
	switch v := value.(type) {
	case float64:
		return uint32(v)
	case int:
		return uint32(v)
	case nil:
		return 0
	}
	t.Fatalf("fixture number has type %T", value)
	return 0
}

func fixtureString(value any) string {
	text, _ := value.(string)
	return text
}

func fixtureSnapshot(t *testing.T, processes, connections []map[string]any) transportSnapshot {
	t.Helper()
	var snapshot transportSnapshot
	for _, p := range processes {
		snapshot.Processes = append(snapshot.Processes, transportProcess{
			ProcessID:       fixtureUint32(t, p["ProcessId"]),
			ParentProcessID: fixtureUint32(t, p["ParentProcessId"]),
			Name:            fixtureString(p["Name"]),
			ExecutablePath:  fixtureString(p["ExecutablePath"]),
			CommandLine:     fixtureString(p["CommandLine"]),
		})
	}
	for _, c := range connections {
		snapshot.Connections = append(snapshot.Connections, transportConnection{
			OwningProcess: fixtureUint32(t, c["OwningProcess"]),
			RemoteAddress: fixtureString(c["RemoteAddress"]),
			RemotePort:    int(fixtureUint32(t, c["RemotePort"])),
		})
	}
	return snapshot
}

// runTransportFixture runs the production checker with only the two system
// reads replaced by the fixture. Connections are handed over only when the
// checker asks for them, as the live snapshot does.
func runTransportFixture(t *testing.T, processes, connections []map[string]any, port int) string {
	t.Helper()
	fixture := fixtureSnapshot(t, processes, connections)
	checker := nativeDesktopTransportChecker{
		expectedEndpoint: func() string { return "ws://127.0.0.1:" + strconv.Itoa(port) },
		snapshot: func(wantConnections func([]transportProcess) bool) (transportSnapshot, error) {
			result := transportSnapshot{Processes: fixture.Processes}
			if wantConnections(fixture.Processes) {
				result.Connections = fixture.Connections
			}
			return result, nil
		},
	}
	state, err := checker.State(context.Background())
	if err != nil {
		t.Fatalf("probe failed: %v", err)
	}
	return string(state)
}

func TestDesktopTransportDistinguishesListenServerFromLegacyStdio(t *testing.T) {
	exe := `C:\Program Files\WindowsApps\OpenAI.Codex_test\app\ChatGPT.exe`
	main := map[string]any{"ProcessId": 10, "ParentProcessId": 1, "Name": "ChatGPT.exe", "ExecutablePath": exe, "CommandLine": `"` + exe + `"`}
	for _, tc := range []struct {
		command string
		want    string
	}{
		{`codex.exe app-server`, "legacy_stdio"},
		{`codex.exe app-server --listen ws://127.0.0.1:49622`, "unknown"},
		{`codex.exe app-server --listen=ws://127.0.0.1:49622`, "unknown"},
		{`codex.exe exec-server`, "unknown"},
	} {
		child := map[string]any{"ProcessId": 20, "ParentProcessId": 10, "Name": "codex.exe", "ExecutablePath": `C:\codex.exe`, "CommandLine": tc.command}
		if got := runTransportFixture(t, []map[string]any{main, child}, nil, 49622); got != tc.want {
			t.Fatalf("%s: got %q want %s", tc.command, got, tc.want)
		}
	}
}

func TestDesktopTransportRequiresActualTargetConnection(t *testing.T) {
	exe := `C:\Program Files\WindowsApps\OpenAI.Codex_test\app\ChatGPT.exe`
	main := map[string]any{"ProcessId": 10, "ParentProcessId": 1, "Name": "ChatGPT.exe", "ExecutablePath": exe, "CommandLine": `"` + exe + `"`}
	for _, tc := range []struct {
		name       string
		connection map[string]any
		want       string
	}{
		{"target", map[string]any{"OwningProcess": 10, "RemoteAddress": "127.0.0.1", "RemotePort": 49622}, "shared_server"},
		{"other_port", map[string]any{"OwningProcess": 10, "RemoteAddress": "127.0.0.1", "RemotePort": 49623}, "unknown"},
		{"other_address", map[string]any{"OwningProcess": 10, "RemoteAddress": "10.0.0.1", "RemotePort": 49622}, "unknown"},
		{"other_owner", map[string]any{"OwningProcess": 11, "RemoteAddress": "127.0.0.1", "RemotePort": 49622}, "unknown"},
	} {
		if got := runTransportFixture(t, []map[string]any{main}, []map[string]any{tc.connection}, 49622); got != tc.want {
			t.Fatalf("%s: got %q want %s", tc.name, got, tc.want)
		}
	}
	for _, checker := range []nativeDesktopTransportChecker{
		{},
		{expectedEndpoint: func() string { return "ws://example.com:49622" }},
	} {
		state, err := checker.State(context.Background())
		if err != nil || state != desktopUnknown {
			t.Fatalf("unverified endpoint was treated as connected: %s %v", state, err)
		}
	}
}

// The live snapshot reads paths and command lines only for ChatGPT.exe and
// Codex.exe. That shortcut must not change any answer: classifying the full
// captured trees and the same trees reduced to the candidates must agree, with
// and without a connection from each process.
func TestDesktopTransportCandidateFilterIsLossless(t *testing.T) {
	for _, name := range []string{transportTreeNewShape, transportTreeOldShape} {
		fixture := loadTransportTreeFixture(t, name)
		full := fixtureSnapshot(t, fixture.Processes, nil)
		// Decoys a name filter drops: a node.exe that copies the NodeService
		// flags under the main process, with an app-server of its own.
		var mainPID uint32
		for _, p := range full.Processes {
			if isDesktopMainProcess(p) {
				mainPID = p.ProcessID
			}
		}
		full.Processes = append(full.Processes,
			transportProcess{ProcessID: 70000, ParentProcessID: mainPID, Name: "node.exe", ExecutablePath: `C:\Program Files\nodejs\node.exe`,
				CommandLine: `node.exe --type=utility --utility-sub-type=node.mojom.NodeService`},
			transportProcess{ProcessID: 70001, ParentProcessID: 70000, Name: "codex.exe", ExecutablePath: `C:\codex.exe`, CommandLine: `codex.exe app-server`},
		)
		var reduced transportSnapshot
		dropped := 0
		for _, p := range full.Processes {
			if transportCandidateName(p.Name) {
				reduced.Processes = append(reduced.Processes, p)
			} else {
				dropped++
			}
		}
		if dropped == 0 {
			t.Fatalf("%s has no non-candidate process, so the filter is not exercised", name)
		}
		owners := []uint32{0}
		for _, p := range full.Processes {
			owners = append(owners, p.ProcessID)
		}
		for _, owner := range owners {
			var connections []transportConnection
			if owner != 0 {
				connections = []transportConnection{{OwningProcess: owner, RemoteAddress: "127.0.0.1", RemotePort: transportTreeTestPort}}
			}
			full.Connections, reduced.Connections = connections, connections
			if a, b := classifyDesktopTransport(full, transportTreeTestPort), classifyDesktopTransport(reduced, transportTreeTestPort); a != b {
				t.Fatalf("%s owner %d: full tree %q, candidates only %q", name, owner, a, b)
			}
		}
	}
}

// Like the former script, a machine without Desktop answers "stopped" without
// reading the TCP table.
func TestDesktopTransportSkipsTCPTableWhenDesktopStopped(t *testing.T) {
	asked := false
	checker := nativeDesktopTransportChecker{
		expectedEndpoint: func() string { return "ws://127.0.0.1:49622" },
		snapshot: func(wantConnections func([]transportProcess) bool) (transportSnapshot, error) {
			processes := []transportProcess{{ProcessID: 5, ParentProcessID: 1, Name: "codex.exe", ExecutablePath: `C:\codex.exe`, CommandLine: "codex.exe app-server"}}
			asked = wantConnections(processes)
			return transportSnapshot{Processes: processes}, nil
		},
	}
	state, err := checker.State(context.Background())
	if err != nil || state != desktopStopped || asked {
		t.Fatalf("got %s %v, TCP table requested=%v", state, err, asked)
	}
}

// MIB_TCPROW_OWNER_PID stores address and port in network byte order and the
// state as a number. A wrong byte order would silently miss the shared port.
func TestParseTCPOwnerPIDTableDecodesNetworkOrder(t *testing.T) {
	row := func(state uint32, a, b, c, d byte, port int, pid uint32) []byte {
		r := make([]byte, tcpRowOwnerPIDSize)
		binary.LittleEndian.PutUint32(r[0:], state)
		r[12], r[13], r[14], r[15] = a, b, c, d
		r[16], r[17] = byte(port>>8), byte(port)
		binary.LittleEndian.PutUint32(r[20:], pid)
		return r
	}
	table := binary.LittleEndian.AppendUint32(nil, 3)
	table = append(table, row(mibTCPStateEstablished, 127, 0, 0, 1, 49622, 13328)...)
	table = append(table, row(2 /* LISTEN */, 127, 0, 0, 1, 49622, 1)...)
	table = append(table, row(11 /* TIME_WAIT */, 127, 0, 0, 1, 49622, 2)...)
	connections, err := parseTCPOwnerPIDTable(table)
	if err != nil {
		t.Fatal(err)
	}
	want := transportConnection{OwningProcess: 13328, RemoteAddress: "127.0.0.1", RemotePort: 49622}
	if len(connections) != 1 || connections[0] != want {
		t.Fatalf("got %+v, want only %+v", connections, want)
	}
	if _, err := parseTCPOwnerPIDTable(table[:len(table)-1]); err == nil {
		t.Fatal("truncated table was accepted")
	}
}

func TestDesktopTransportHungSnapshotReportsControllerTimeout(t *testing.T) {
	release := make(chan struct{})
	defer close(release)
	checker := nativeDesktopTransportChecker{
		expectedEndpoint: func() string { return "ws://127.0.0.1:49622" },
		snapshot: func(func([]transportProcess) bool) (transportSnapshot, error) {
			<-release
			return transportSnapshot{}, nil
		},
	}
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	if _, err := checker.State(ctx); !errors.Is(err, errControllerTimeout) {
		t.Fatalf("got %v, want errControllerTimeout", err)
	}
}

// The PowerShell probe took 5-15 s against the 20 s desktopTransportCheckTimeout
// and timed out in 3 of 4 real Readiness runs (docs/DECISIONS.md 2026-11-22).
// The native probe must stay far below that on the live machine, whether or
// not Codex Desktop is running. 1 s is 1/20 of the limit; the native probe
// measured in milliseconds.
func TestDesktopTransportLiveProbeStaysFarBelowTimeout(t *testing.T) {
	checker := nativeDesktopTransportChecker{expectedEndpoint: func() string { return "ws://127.0.0.1:49622" }}
	for i := 0; i < 5; i++ {
		start := time.Now()
		state, err := checker.State(context.Background())
		elapsed := time.Since(start)
		if err != nil {
			t.Fatalf("live probe failed: %v", err)
		}
		switch state {
		case desktopStopped, desktopLegacyStdio, desktopSharedServer, desktopUnknown:
		default:
			t.Fatalf("live probe returned %q", state)
		}
		if elapsed > time.Second {
			t.Fatalf("live probe run %d took %v (state %s), budget 1s of the %v limit", i+1, elapsed, state, desktopTransportCheckTimeout)
		}
	}
}
