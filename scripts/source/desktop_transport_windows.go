//go:build windows

package main

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"
)

type desktopTransportState string

const (
	desktopStopped      desktopTransportState = "stopped"
	desktopLegacyStdio  desktopTransportState = "legacy_stdio"
	desktopSharedServer desktopTransportState = "shared_server"
	desktopOfficialIPC  desktopTransportState = "official_ipc"
	desktopUnknown      desktopTransportState = "unknown"
)

type desktopTransportChecker interface {
	State(context.Context) (desktopTransportState, error)
}

// nativeDesktopTransportChecker proves which transport Codex Desktop uses by
// reading the process tree and the TCP table through Win32 directly.
//
// It replaced a PowerShell script (Get-CimInstance Win32_Process plus
// Get-NetTCPConnection) that took 5-15 s per run against the 20 s
// desktopTransportCheckTimeout: PowerShell start-up, the WMI provider and the
// NetTCPIP module load were nearly all of that time. See docs/DECISIONS.md
// 2026-11-22 "傳輸判斷改走原生查詢".
type nativeDesktopTransportChecker struct {
	expectedEndpoint func() string
	// snapshot is replaced in tests; nil means the live system.
	snapshot func(wantConnections func([]transportProcess) bool) (transportSnapshot, error)
}

type transportProcess struct {
	ProcessID       uint32
	ParentProcessID uint32
	Name            string
	ExecutablePath  string
	CommandLine     string
}

// transportConnection is an ESTABLISHED IPv4 TCP connection.
type transportConnection struct {
	OwningProcess uint32
	RemoteAddress string
	RemotePort    int
}

type transportSnapshot struct {
	Processes   []transportProcess
	Connections []transportConnection
}

// Every -match in the former PowerShell probe was case-insensitive, and so is
// every pattern here.
var (
	desktopMainPathPatterns = []*regexp.Regexp{
		regexp.MustCompile(`(?i)\\WindowsApps\\OpenAI\.Codex_[^\\]+\\app\\(?:ChatGPT|Codex)\.exe$`),
		regexp.MustCompile(`(?i)\\OpenAI\\Codex\\(?:ChatGPT|Codex)\.exe$`),
	}
	electronTypeFlag      = regexp.MustCompile(`(?i)(?:^|\s)--type=`)
	electronUtilityFlag   = regexp.MustCompile(`(?i)(?:^|\s)--type=utility(?:\s|$)`)
	nodeServiceSubType    = regexp.MustCompile(`(?i)(?:^|\s)--utility-sub-type=node\.mojom\.NodeService(?:\s|$)`)
	networkServiceSubType = regexp.MustCompile(`(?i)--utility-sub-type=network\.mojom\.NetworkService`)
	appServerArgument     = regexp.MustCompile(`(?i)(?:^|\s)app-server(?:\s|$)`)
	appServerListenFlag   = regexp.MustCompile(`(?i)(?:^|\s)--listen(?:=|\s)`)
)

// transportCandidateName reports whether a process can take any role in
// classifyDesktopTransport. Every role requires the image name ChatGPT.exe or
// Codex.exe (app-server is codex.exe; the NodeService host and the network
// process run the main process's own executable), so the live snapshot reads
// paths and command lines only for these processes.
func transportCandidateName(name string) bool {
	return strings.EqualFold(name, "ChatGPT.exe") || strings.EqualFold(name, "Codex.exe")
}

func isDesktopMainProcess(p transportProcess) bool {
	if !transportCandidateName(p.Name) || p.ExecutablePath == "" {
		return false
	}
	matched := false
	for _, pattern := range desktopMainPathPatterns {
		if pattern.MatchString(p.ExecutablePath) {
			matched = true
			break
		}
	}
	return matched && (p.CommandLine == "" || !electronTypeFlag.MatchString(p.CommandLine))
}

// classifyDesktopTransport is the pure decision. It takes the whole snapshot
// and does not rely on transportCandidateName having filtered it.
func classifyDesktopTransport(snapshot transportSnapshot, expectedPort int) desktopTransportState {
	var mains []transportProcess
	mainIDs := map[uint32]bool{}
	for _, p := range snapshot.Processes {
		if isDesktopMainProcess(p) {
			mains = append(mains, p)
			mainIDs[p.ProcessID] = true
		}
	}
	if len(mains) == 0 {
		return desktopStopped
	}
	mainExecutable := func(path string) bool {
		for _, m := range mains {
			if strings.EqualFold(m.ExecutablePath, path) {
				return true
			}
		}
		return false
	}
	// Desktop 26.1117 moved the app-server out of the main process: it is now
	// started by the "app-server-host" Electron utility
	// (--type=utility --utility-sub-type=node.mojom.NodeService). Accept that
	// host only when it is a direct child of a verified main process AND runs
	// the very same executable as that parent. A NodeService of another
	// Electron app, a plain node.exe, or a utility further down the tree is
	// never a Desktop host.
	nodeHostIDs := map[uint32]bool{}
	for _, p := range snapshot.Processes {
		if p.ExecutablePath == "" ||
			!electronUtilityFlag.MatchString(p.CommandLine) ||
			!nodeServiceSubType.MatchString(p.CommandLine) {
			continue
		}
		for _, m := range mains {
			if m.ProcessID == p.ParentProcessID && strings.EqualFold(m.ExecutablePath, p.ExecutablePath) {
				nodeHostIDs[p.ProcessID] = true
				break
			}
		}
	}
	// The client is Desktop (its main process, its Electron network process, or
	// its app-server host utility), not the listening app-server. Do not count
	// arbitrary task/CLI descendants as Desktop clients.
	clientIDs := map[uint32]bool{}
	for id := range mainIDs {
		clientIDs[id] = true
	}
	for id := range nodeHostIDs {
		clientIDs[id] = true
	}
	for _, p := range snapshot.Processes {
		if mainIDs[p.ParentProcessID] && p.ExecutablePath != "" && mainExecutable(p.ExecutablePath) &&
			networkServiceSubType.MatchString(p.CommandLine) {
			clientIDs[p.ProcessID] = true
		}
	}
	legacy := false
	for _, p := range snapshot.Processes {
		if strings.EqualFold(p.Name, "codex.exe") &&
			(mainIDs[p.ParentProcessID] || nodeHostIDs[p.ParentProcessID]) &&
			appServerArgument.MatchString(p.CommandLine) &&
			!appServerListenFlag.MatchString(p.CommandLine) {
			legacy = true
		}
	}
	// Newer Desktop builds may keep an auxiliary stdio app-server child while
	// the main client is already connected to the verified shared endpoint.
	// Connection evidence is stronger than the presence of that auxiliary child.
	for _, c := range snapshot.Connections {
		if clientIDs[c.OwningProcess] && c.RemoteAddress == "127.0.0.1" && c.RemotePort == expectedPort {
			return desktopSharedServer
		}
	}
	if legacy {
		return desktopLegacyStdio
	}
	return desktopUnknown
}

func (c nativeDesktopTransportChecker) State(ctx context.Context) (desktopTransportState, error) {
	if c.expectedEndpoint == nil {
		return desktopUnknown, nil
	}
	endpoint := c.expectedEndpoint()
	port := endpointPort(endpoint)
	if !validSharedServerEndpoint(endpoint, port) {
		return desktopUnknown, nil
	}
	snapshot := c.snapshot
	if snapshot == nil {
		snapshot = liveTransportSnapshot
	}
	checkCtx, cancel := context.WithTimeout(ctx, desktopTransportCheckTimeout)
	defer cancel()
	type outcome struct {
		snapshot transportSnapshot
		err      error
	}
	done := make(chan outcome, 1)
	go func() {
		// A system call that panics must not take the watchdog down with it;
		// report it as an error so preflight treats the transport as unknown.
		defer func() {
			if recovered := recover(); recovered != nil {
				done <- outcome{err: fmt.Errorf("desktop transport probe panicked: %v", recovered)}
			}
		}()
		// Like the former script, the TCP table is read only when a verified
		// Desktop main process exists.
		s, err := snapshot(func(processes []transportProcess) bool {
			for _, p := range processes {
				if isDesktopMainProcess(p) {
					return true
				}
			}
			return false
		})
		done <- outcome{s, err}
	}()
	select {
	case <-checkCtx.Done():
		return "", wrapControllerCommandError(checkCtx, checkCtx.Err())
	case result := <-done:
		if result.err != nil {
			return "", result.err
		}
		return classifyDesktopTransport(result.snapshot, port), nil
	}
}

func liveTransportSnapshot(wantConnections func([]transportProcess) bool) (transportSnapshot, error) {
	if err := transportAPIsAvailable(); err != nil {
		return transportSnapshot{}, err
	}
	processes, err := snapshotTransportProcesses()
	if err != nil {
		return transportSnapshot{}, err
	}
	snapshot := transportSnapshot{Processes: processes}
	if wantConnections(processes) {
		snapshot.Connections, err = establishedIPv4Connections()
		if err != nil {
			return transportSnapshot{}, err
		}
	}
	return snapshot, nil
}

// snapshotTransportProcesses lists every process and reads the image path and
// command line of the candidates only. A process that cannot be opened keeps
// empty path and command line, which is what Win32_Process reported for it
// too; every role that needs either field then rejects it.
func snapshotTransportProcesses() ([]transportProcess, error) {
	handle, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return nil, fmt.Errorf("process snapshot: %w", err)
	}
	defer windows.CloseHandle(handle)
	var processes []transportProcess
	entry := windows.ProcessEntry32{Size: uint32(unsafe.Sizeof(windows.ProcessEntry32{}))}
	for err = windows.Process32First(handle, &entry); err == nil; err = windows.Process32Next(handle, &entry) {
		name := windows.UTF16ToString(entry.ExeFile[:])
		if !transportCandidateName(name) {
			continue
		}
		p := transportProcess{ProcessID: entry.ProcessID, ParentProcessID: entry.ParentProcessID, Name: name}
		p.ExecutablePath, p.CommandLine = processPathAndCommandLine(entry.ProcessID)
		processes = append(processes, p)
	}
	if !errors.Is(err, windows.ERROR_NO_MORE_FILES) {
		return nil, fmt.Errorf("process snapshot: %w", err)
	}
	return processes, nil
}

func processPathAndCommandLine(pid uint32) (string, string) {
	handle, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, pid)
	if err != nil {
		return "", ""
	}
	defer windows.CloseHandle(handle)
	var path string
	buffer := make([]uint16, windows.MAX_LONG_PATH)
	size := uint32(len(buffer))
	if windows.QueryFullProcessImageName(handle, 0, &buffer[0], &size) == nil && size > 0 {
		path = windows.UTF16ToString(buffer[:size])
	}
	return path, processCommandLine(handle)
}

// processCommandLine reads ProcessCommandLineInformation (Windows 8.1+), which
// needs only PROCESS_QUERY_LIMITED_INFORMATION and returns the same string
// Win32_Process.CommandLine does.
func processCommandLine(handle windows.Handle) string {
	length := uint32(4096)
	for attempt := 0; attempt < 3; attempt++ {
		buffer := make([]byte, length)
		var needed uint32
		err := windows.NtQueryInformationProcess(handle, windows.ProcessCommandLineInformation,
			unsafe.Pointer(&buffer[0]), length, &needed)
		if err == nil {
			text := (*windows.NTUnicodeString)(unsafe.Pointer(&buffer[0]))
			if text.Buffer == nil || text.Length == 0 {
				return ""
			}
			return windows.UTF16ToString(unsafe.Slice(text.Buffer, text.Length/2))
		}
		if needed <= length {
			return ""
		}
		length = needed
	}
	return ""
}

var (
	modTransportKernel32    = windows.NewLazySystemDLL("kernel32.dll")
	modTransportNTDLL       = windows.NewLazySystemDLL("ntdll.dll")
	modIPHelper             = windows.NewLazySystemDLL("iphlpapi.dll")
	procGetExtendedTCPTable = modIPHelper.NewProc("GetExtendedTcpTable")

	// transportRequiredProcs lists every Win32 entry point the live snapshot
	// calls. LazyProc.Call and the x/sys/windows wrappers (which use
	// LazyProc.Addr) panic when a DLL or function cannot be found, so each one
	// is resolved with Find first and a missing one becomes an error.
	transportRequiredProcs = []*windows.LazyProc{
		modTransportKernel32.NewProc("CreateToolhelp32Snapshot"),
		modTransportKernel32.NewProc("Process32FirstW"),
		modTransportKernel32.NewProc("Process32NextW"),
		modTransportKernel32.NewProc("OpenProcess"),
		modTransportKernel32.NewProc("QueryFullProcessImageNameW"),
		modTransportNTDLL.NewProc("NtQueryInformationProcess"),
		procGetExtendedTCPTable,
	}
)

func transportAPIsAvailable() error {
	for _, proc := range transportRequiredProcs {
		if err := proc.Find(); err != nil {
			return fmt.Errorf("desktop transport probe API unavailable: %w", err)
		}
	}
	return nil
}

const (
	tcpTableOwnerPIDAll    = 5 // TCP_TABLE_OWNER_PID_ALL
	mibTCPStateEstablished = 5 // MIB_TCP_STATE_ESTAB
	tcpRowOwnerPIDSize     = 24
)

// establishedIPv4Connections reads MIB_TCPTABLE_OWNER_PID. The state is the
// numeric MIB value, so unlike netstat output it does not depend on the UI
// language.
func establishedIPv4Connections() ([]transportConnection, error) {
	size := uint32(64 * 1024)
	for attempt := 0; attempt < 4; attempt++ {
		buffer := make([]byte, size)
		ret, _, _ := procGetExtendedTCPTable.Call(uintptr(unsafe.Pointer(&buffer[0])),
			uintptr(unsafe.Pointer(&size)), 0, windows.AF_INET, tcpTableOwnerPIDAll, 0)
		if windows.Errno(ret) == windows.ERROR_INSUFFICIENT_BUFFER {
			size += 4096
			continue
		}
		if ret != 0 {
			return nil, fmt.Errorf("GetExtendedTcpTable: %w", windows.Errno(ret))
		}
		return parseTCPOwnerPIDTable(buffer)
	}
	return nil, errors.New("GetExtendedTcpTable: table kept growing")
}

func parseTCPOwnerPIDTable(table []byte) ([]transportConnection, error) {
	if len(table) < 4 {
		return nil, errors.New("TCP table is truncated")
	}
	count := int(binary.LittleEndian.Uint32(table))
	if len(table) < 4+count*tcpRowOwnerPIDSize {
		return nil, errors.New("TCP table is truncated")
	}
	var connections []transportConnection
	for i := 0; i < count; i++ {
		row := table[4+i*tcpRowOwnerPIDSize:]
		if binary.LittleEndian.Uint32(row[0:]) != mibTCPStateEstablished {
			continue
		}
		// dwRemoteAddr and dwRemotePort hold network byte order.
		address := fmt.Sprintf("%d.%d.%d.%d", row[12], row[13], row[14], row[15])
		port := int(row[16])<<8 | int(row[17])
		connections = append(connections, transportConnection{
			OwningProcess: binary.LittleEndian.Uint32(row[20:]),
			RemoteAddress: address,
			RemotePort:    port,
		})
	}
	return connections, nil
}
