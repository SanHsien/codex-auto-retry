//go:build windows

package main

import (
	"bytes"
	"context"
	_ "embed"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

//go:embed ui/settings.ps1
var settingsPowerShell string

const (
	wmDestroy          = 0x0002
	wmCommand          = 0x0111
	wmTimer            = 0x0113
	wmClose            = 0x0010
	wmNull             = 0x0000
	wmLButtonDblClk    = 0x0203
	wmRButtonUp        = 0x0205
	wmAppTray          = 0x8001
	taskbarCreatedName = "TaskbarCreated"

	nimAdd     = 0
	nimModify  = 1
	nimDelete  = 2
	nifMessage = 0x1
	nifIcon    = 0x2
	nifTip     = 0x4
	nifInfo    = 0x10

	mfString       = 0x0000
	mfGrayed       = 0x0001
	mfSeparator    = 0x0800
	tpmRightButton = 0x0002
	tpmReturnCmd   = 0x0100

	menuOpenSettings = 1001
	menuTogglePause  = 1002
	menuExit         = 1003
	trayTimerID      = 1
)

type trayPoint struct{ X, Y int32 }
type trayMessage struct {
	HWnd     uintptr
	Message  uint32
	WParam   uintptr
	LParam   uintptr
	Time     uint32
	Pt       trayPoint
	LPrivate uint32
}
type windowClass struct {
	Size       uint32
	Style      uint32
	WndProc    uintptr
	ClsExtra   int32
	WndExtra   int32
	Instance   uintptr
	Icon       uintptr
	Cursor     uintptr
	Background uintptr
	MenuName   *uint16
	ClassName  *uint16
	IconSmall  uintptr
}
type notifyIconData struct {
	Size             uint32
	HWnd             uintptr
	ID               uint32
	Flags            uint32
	CallbackMessage  uint32
	Icon             uintptr
	Tip              [128]uint16
	State            uint32
	StateMask        uint32
	Info             [256]uint16
	TimeoutOrVersion uint32
	InfoTitle        [64]uint16
	InfoFlags        uint32
	GUID             windows.GUID
	BalloonIcon      uintptr
}

var (
	user32                  = windows.NewLazySystemDLL("user32.dll")
	shell32                 = windows.NewLazySystemDLL("shell32.dll")
	kernel32                = windows.NewLazySystemDLL("kernel32.dll")
	procRegisterClassEx     = user32.NewProc("RegisterClassExW")
	procRegisterWindowMsg   = user32.NewProc("RegisterWindowMessageW")
	procCreateWindowEx      = user32.NewProc("CreateWindowExW")
	procDefWindowProc       = user32.NewProc("DefWindowProcW")
	procDestroyWindow       = user32.NewProc("DestroyWindow")
	procPostMessage         = user32.NewProc("PostMessageW")
	procGetMessage          = user32.NewProc("GetMessageW")
	procTranslateMessage    = user32.NewProc("TranslateMessage")
	procDispatchMessage     = user32.NewProc("DispatchMessageW")
	procPostQuitMessage     = user32.NewProc("PostQuitMessage")
	procSetTimer            = user32.NewProc("SetTimer")
	procKillTimer           = user32.NewProc("KillTimer")
	procLoadIcon            = user32.NewProc("LoadIconW")
	procCreatePopupMenu     = user32.NewProc("CreatePopupMenu")
	procAppendMenu          = user32.NewProc("AppendMenuW")
	procTrackPopupMenu      = user32.NewProc("TrackPopupMenu")
	procDestroyMenu         = user32.NewProc("DestroyMenu")
	procGetCursorPos        = user32.NewProc("GetCursorPos")
	procSetForegroundWindow = user32.NewProc("SetForegroundWindow")
	procShellNotifyIcon     = shell32.NewProc("Shell_NotifyIconW")
	procGetModuleHandle     = kernel32.NewProc("GetModuleHandleW")
	trayWindowProc          = syscall.NewCallback(trayWndProc)
	trayApps                sync.Map
)

type trayApp struct {
	hwnd                uintptr
	dataDir             string
	logger              *safeLogger
	cancel              context.CancelFunc
	service             *managementService
	icons               map[string]uintptr
	lastTip             string
	lastIcon            string
	lastStopped         int
	lastGoalStopped     int
	lastGoalFailed      int
	lastRestartRequired int
	lastCodexStopped    int
	lastSharedDisabled  int
	initialized         bool
	settingsMu          sync.Mutex
	settingsOpen        bool
	taskbarCreated      uint32
	restoreIconFunc     func()
}

func runTray(ctx context.Context, cancel context.CancelFunc, dataDir string, logger *safeLogger) error {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	taskbarCreated, err := registerTaskbarCreatedMessage()
	if err != nil {
		return err
	}
	className, _ := windows.UTF16PtrFromString(fmt.Sprintf("CodexAutoRetryTray-%d", os.Getpid()))
	instance, _, _ := procGetModuleHandle.Call(0)
	icon, _, _ := procLoadIcon.Call(0, 32512)
	wc := windowClass{Size: uint32(unsafe.Sizeof(windowClass{})), WndProc: trayWindowProc, Instance: instance, Icon: icon, IconSmall: icon, ClassName: className}
	if result, _, err := procRegisterClassEx.Call(uintptr(unsafe.Pointer(&wc))); result == 0 {
		return fmt.Errorf("register tray window: %w", err)
	}
	hwnd, _, err := procCreateWindowEx.Call(0, uintptr(unsafe.Pointer(className)), uintptr(unsafe.Pointer(className)), 0, 0, 0, 0, 0, 0, 0, instance, 0)
	if hwnd == 0 {
		return fmt.Errorf("create tray window: %w", err)
	}
	app := &trayApp{
		hwnd: hwnd, dataDir: dataDir, logger: logger, cancel: cancel,
		taskbarCreated: taskbarCreated,
		service:        newManagementService(dataDir),
		icons: map[string]uintptr{
			"running": loadSharedIcon(32516),
			"waiting": loadSharedIcon(32515),
			"paused":  loadSharedIcon(32515),
			"active":  loadSharedIcon(32516),
			"stopped": loadSharedIcon(32513),
		},
	}
	trayApps.Store(hwnd, app)
	defer trayApps.Delete(hwnd)
	if !app.addIcon() {
		procDestroyWindow.Call(hwnd)
		return fmt.Errorf("add tray icon")
	}
	procSetTimer.Call(hwnd, trayTimerID, 1000, 0)
	app.refresh()
	go func() {
		<-ctx.Done()
		procPostMessage.Call(hwnd, wmClose, 0, 0)
	}()
	var message trayMessage
	for {
		result, _, _ := procGetMessage.Call(uintptr(unsafe.Pointer(&message)), 0, 0, 0)
		if int32(result) <= 0 {
			break
		}
		procTranslateMessage.Call(uintptr(unsafe.Pointer(&message)))
		procDispatchMessage.Call(uintptr(unsafe.Pointer(&message)))
	}
	return nil
}

func trayWndProc(hwnd uintptr, message uint32, wParam, lParam uintptr) uintptr {
	value, ok := trayApps.Load(hwnd)
	if !ok {
		result, _, _ := procDefWindowProc.Call(hwnd, uintptr(message), wParam, lParam)
		return result
	}
	app := value.(*trayApp)
	if app.taskbarCreated != 0 && message == app.taskbarCreated {
		app.handleTaskbarCreated()
		return 0
	}
	switch message {
	case wmTimer:
		app.refresh()
		return 0
	case wmAppTray:
		switch uint32(lParam) {
		case wmLButtonDblClk:
			app.openSettings()
		case wmRButtonUp:
			app.showMenu()
		}
		return 0
	case wmCommand:
		app.handleCommand(uint16(wParam & 0xffff))
		return 0
	case wmClose:
		procDestroyWindow.Call(hwnd)
		return 0
	case wmDestroy:
		procKillTimer.Call(hwnd, trayTimerID)
		app.removeIcon()
		procPostQuitMessage.Call(0)
		return 0
	}
	result, _, _ := procDefWindowProc.Call(hwnd, uintptr(message), wParam, lParam)
	return result
}

func (a *trayApp) handleTaskbarCreated() {
	if a.restoreIconFunc != nil {
		a.restoreIconFunc()
		return
	}
	a.restoreIcon()
}

func registerTaskbarCreatedMessage() (uint32, error) {
	name, err := windows.UTF16PtrFromString(taskbarCreatedName)
	if err != nil {
		return 0, fmt.Errorf("encode %s message name: %w", taskbarCreatedName, err)
	}
	result, _, callErr := procRegisterWindowMsg.Call(uintptr(unsafe.Pointer(name)))
	if result == 0 {
		if callErr == nil {
			callErr = windows.GetLastError()
		}
		return 0, fmt.Errorf("register %s message: %w", taskbarCreatedName, callErr)
	}
	return uint32(result), nil
}

func (a *trayApp) addIcon() bool {
	data := notifyIconData{Size: uint32(unsafe.Sizeof(notifyIconData{})), HWnd: a.hwnd, ID: 1, Flags: nifMessage | nifIcon | nifTip, CallbackMessage: wmAppTray, Icon: a.icons["running"]}
	copyUTF16(data.Tip[:], "Codex Auto Retry")
	result, _, _ := procShellNotifyIcon.Call(nimAdd, uintptr(unsafe.Pointer(&data)))
	return result != 0
}

func (a *trayApp) removeIcon() {
	data := notifyIconData{Size: uint32(unsafe.Sizeof(notifyIconData{})), HWnd: a.hwnd, ID: 1}
	procShellNotifyIcon.Call(nimDelete, uintptr(unsafe.Pointer(&data)))
}

func (a *trayApp) restoreIcon() {
	// Explorer owns the notification area. When it restarts, it forgets every
	// icon even though this process and its hidden window are still alive. Clear
	// the local cache so refresh cannot incorrectly skip the first NIM_MODIFY.
	a.lastTip = ""
	a.lastIcon = ""
	if !restoreTrayIcon(a.addIcon, a.removeIcon, a.refresh) {
		a.logger.Printf("tray icon restore failed category=tray_icon")
	}
}

func restoreTrayIcon(add func() bool, remove func(), refresh func()) bool {
	remove()
	if !add() {
		return false
	}
	refresh()
	return true
}

func (a *trayApp) refresh() {
	language := uiLanguage(a.dataDir)
	L := func(zh, en string) string { return text(language, zh, en) }
	snapshot, err := a.service.snapshot(time.Now().UTC())
	if err != nil {
		a.setTip(L("Codex Auto Retry - 狀態讀取失敗", "Codex Auto Retry - Status unavailable"))
		return
	}
	tip := L("Codex Auto Retry - 執行中", "Codex Auto Retry - Running")
	iconState := "running"
	if snapshot.ControllerState == "codex_restart_required" {
		tip = L("Codex Auto Retry - 目前為官方後端；請透過安全啟動入口接入共用通道", "Codex Auto Retry - On the official backend; relaunch Codex with the safe launcher")
		iconState = "paused"
	} else if snapshot.ControllerState == "codex_not_running" && snapshot.StoppedRetries > 0 {
		tip = L("Codex Auto Retry - Codex 已結束，重試已停止", "Codex Auto Retry - Codex exited; retries stopped")
		iconState = "stopped"
	} else if snapshot.ControllerState == "shared_app_server_disabled" {
		tip = L("Codex Auto Retry - 共用後端已關閉，重試未執行", "Codex Auto Retry - Shared backend off; retry not run")
		iconState = "paused"
	} else if snapshot.ControllerState == "shared_app_server_port_reserved" {
		tip = L("Codex Auto Retry - 共用埠被 Windows 保留，重試未執行", "Codex Auto Retry - Port reserved by Windows; retry not run")
		iconState = "stopped"
	} else if snapshot.ControllerState == "shared_app_server_port_conflict" {
		tip = fmt.Sprintf(L("Codex Auto Retry - 偏好埠不可用，目前埠 %d", "Codex Auto Retry - Preferred port unavailable; using %d"), snapshot.SharedAppServerPort)
		iconState = "paused"
	} else if snapshot.ControllerState == "shared_app_server_migration_deferred" {
		tip = L("Codex Auto Retry - 等待 Codex 關閉後完成後端遷移", "Codex Auto Retry - Backend migration waits for Codex to close")
		iconState = "paused"
	} else if snapshot.Paused {
		tip = L("Codex Auto Retry - 已暫停", "Codex Auto Retry - Paused")
		iconState = "paused"
	} else if snapshot.ActiveRetries > 0 {
		tip = fmt.Sprintf(L("Codex Auto Retry - 正在重試 %d 個任務", "Codex Auto Retry - Retrying %d task(s)"), snapshot.ActiveRetries)
		iconState = "active"
	} else if seconds, ok := nextRetrySeconds(snapshot.Retries); ok {
		tip = fmt.Sprintf(L("Codex Auto Retry - %d 秒後自動重試", "Codex Auto Retry - Retrying in %d s"), seconds)
		iconState = "waiting"
	} else if snapshot.StoppedRetries > 0 {
		tip = fmt.Sprintf(L("Codex Auto Retry - %d 個任務已停止重試", "Codex Auto Retry - %d task(s) stopped retrying"), snapshot.StoppedRetries)
		iconState = "stopped"
	}
	a.setVisual(iconState, tip)
	goalStopped := goalEmptyResponseStoppedCount(snapshot.Retries)
	goalFailed := goalEmptyResponseBlockFailedCount(snapshot.Retries)
	restartRequired := stoppedReasonCount(snapshot.Retries, "codex_restart_required")
	codexStopped := stoppedReasonCount(snapshot.Retries, "codex_not_running")
	sharedDisabled := stoppedReasonCount(snapshot.Retries, "shared_app_server_disabled")
	limitStopped := retryLimitStoppedCount(snapshot.Retries)
	if a.initialized && snapshot.ShowNotifications && restartRequired > a.lastRestartRequired {
		a.notify(L("需要重新啟動 Codex", "Restart Codex"), L("重新啟動一次 Codex 後，等待中的自動重試會自行恢復。", "Restart Codex once; pending automatic retries then resume on their own."))
	} else if a.initialized && snapshot.ShowNotifications && codexStopped > a.lastCodexStopped {
		a.notify(L("Codex 已結束", "Codex exited"), L("相關任務已停止自動重試。啟動 Codex 後可從設定中重新開始。", "Automatic retries for these tasks stopped. Start Codex, then restart them from the settings."))
	} else if a.initialized && snapshot.ShowNotifications && sharedDisabled > a.lastSharedDisabled {
		a.notify(L("共用後端已關閉", "Shared backend is off"), L("自動恢復未執行。請開啟共用後端模式，然後重新開始該任務。", "Automatic recovery did not run. Turn on the shared backend, then restart the task."))
	} else if a.initialized && snapshot.ShowNotifications && goalFailed > a.lastGoalFailed {
		a.notify(L("目標停止失敗", "Goal stop failed"), L("目標恢復已停止，但自動設為受阻失敗。請從面板重新開始或檢查 Codex 狀態。", "Goal recovery stopped, but marking the goal as blocked failed. Restart it from the panel or check Codex."))
	} else if a.initialized && snapshot.ShowNotifications && goalStopped > a.lastGoalStopped {
		a.notify(L("目標已自動停止", "Goal stopped automatically"), L("目標連續空回覆達到上限，目標恢復已停止。", "The goal reached its empty-reply limit; goal recovery stopped."))
	} else if a.initialized && snapshot.ShowNotifications && limitStopped > a.lastStopped {
		a.notify(L("自動重試已停止", "Automatic retry stopped"), fmt.Sprintf(L("有 %d 個任務已達到重試上限。", "%d task(s) reached the retry limit."), limitStopped))
	}
	a.lastStopped = limitStopped
	a.lastGoalStopped = goalStopped
	a.lastGoalFailed = goalFailed
	a.lastRestartRequired = restartRequired
	a.lastCodexStopped = codexStopped
	a.lastSharedDisabled = sharedDisabled
	a.initialized = true
}

func stoppedReasonCount(retries []ManagedRetry, reason string) int {
	count := 0
	for _, retry := range retries {
		if retry.State == "stopped" && retry.StopReason == reason {
			count++
		}
	}
	return count
}

func retryLimitStoppedCount(retries []ManagedRetry) int {
	count := 0
	for _, retry := range retries {
		if retry.State != "stopped" {
			continue
		}
		switch retry.StopReason {
		case "recovery_attempt_limit", "consecutive_retry_limit", "retry_limit", "auth_attempt_limit":
			count++
		}
	}
	return count
}

func goalEmptyResponseStoppedCount(retries []ManagedRetry) int {
	count := 0
	for _, retry := range retries {
		if retry.State == "stopped" && retry.StopReason == goalEmptyResponseStopReason {
			count++
		}
	}
	return count
}

func goalEmptyResponseBlockFailedCount(retries []ManagedRetry) int {
	count := 0
	for _, retry := range retries {
		if retry.State == "stopped" && retry.StopReason == goalEmptyResponseBlockFailReason {
			count++
		}
	}
	return count
}

func nextRetrySeconds(retries []ManagedRetry) (int64, bool) {
	var seconds int64
	found := false
	for _, retry := range retries {
		if retry.State != "pending" {
			continue
		}
		if !found || retry.SecondsRemaining < seconds {
			seconds, found = retry.SecondsRemaining, true
		}
	}
	return seconds, found
}

func (a *trayApp) setTip(tip string) {
	a.setVisual(a.lastIcon, tip)
}

func (a *trayApp) setVisual(iconState, tip string) {
	if iconState == "" {
		iconState = "running"
	}
	if tip == a.lastTip {
		if iconState == a.lastIcon {
			return
		}
	}
	a.lastTip = tip
	a.lastIcon = iconState
	data := notifyIconData{Size: uint32(unsafe.Sizeof(notifyIconData{})), HWnd: a.hwnd, ID: 1, Flags: nifTip | nifIcon, Icon: a.icons[iconState]}
	copyUTF16(data.Tip[:], tip)
	procShellNotifyIcon.Call(nimModify, uintptr(unsafe.Pointer(&data)))
}

func loadSharedIcon(resourceID uintptr) uintptr {
	icon, _, _ := procLoadIcon.Call(0, resourceID)
	return icon
}

func (a *trayApp) notify(title, text string) {
	data := notifyIconData{Size: uint32(unsafe.Sizeof(notifyIconData{})), HWnd: a.hwnd, ID: 1, Flags: nifInfo, InfoFlags: 0x1}
	copyUTF16(data.InfoTitle[:], title)
	copyUTF16(data.Info[:], text)
	procShellNotifyIcon.Call(nimModify, uintptr(unsafe.Pointer(&data)))
}

func (a *trayApp) showMenu() {
	language := uiLanguage(a.dataDir)
	L := func(zh, en string) string { return text(language, zh, en) }
	snapshot, _ := a.service.snapshot(time.Now().UTC())
	menu, _, _ := procCreatePopupMenu.Call()
	if menu == 0 {
		return
	}
	defer procDestroyMenu.Call(menu)
	status := a.lastTip
	if strings.HasPrefix(status, "Codex Auto Retry - ") {
		status = strings.TrimPrefix(status, "Codex Auto Retry - ")
	}
	appendTrayMenu(menu, mfGrayed, 0, status)
	appendTrayMenu(menu, mfSeparator, 0, "")
	appendTrayMenu(menu, mfString, menuOpenSettings, L("開啟設定…", "Open settings…"))
	pauseText := L("暫停自動重試", "Pause automatic retry")
	if snapshot.Paused {
		pauseText = L("恢復自動重試", "Resume automatic retry")
	}
	appendTrayMenu(menu, mfString, menuTogglePause, pauseText)
	appendTrayMenu(menu, mfSeparator, 0, "")
	appendTrayMenu(menu, mfString, menuExit, L("結束", "Exit"))
	var point trayPoint
	procGetCursorPos.Call(uintptr(unsafe.Pointer(&point)))
	procSetForegroundWindow.Call(a.hwnd)
	command, _, _ := procTrackPopupMenu.Call(menu, tpmRightButton|tpmReturnCmd, uintptr(point.X), uintptr(point.Y), 0, a.hwnd, 0)
	if command != 0 {
		a.handleCommand(uint16(command))
	}
	procPostMessage.Call(a.hwnd, wmNull, 0, 0)
}

func appendTrayMenu(menu uintptr, flags uint32, id uint16, label string) {
	var ptr *uint16
	if label != "" {
		ptr, _ = windows.UTF16PtrFromString(label)
	}
	procAppendMenu.Call(menu, uintptr(flags), uintptr(id), uintptr(unsafe.Pointer(ptr)))
}

func (a *trayApp) handleCommand(command uint16) {
	switch command {
	case menuOpenSettings:
		a.openSettings()
	case menuTogglePause:
		snapshot, err := a.service.snapshot(time.Now().UTC())
		if err == nil {
			_, _ = a.service.setPaused(!snapshot.Paused, time.Now().UTC())
		}
		a.refresh()
	case menuExit:
		a.cancel()
		procPostMessage.Call(a.hwnd, wmClose, 0, 0)
	}
}

func (a *trayApp) openSettings() {
	a.settingsMu.Lock()
	if a.settingsOpen {
		a.settingsMu.Unlock()
		return
	}
	a.settingsOpen = true
	a.settingsMu.Unlock()
	scriptPath, err := ensureSettingsScript(a.dataDir)
	if err != nil {
		a.logger.Printf("settings open failed category=settings_script")
		a.settingsFinished()
		return
	}
	config, err := loadOrCreateConfig(filepath.Join(a.dataDir, "config.json"))
	if err != nil {
		a.logger.Printf("settings open failed category=settings_config")
		a.settingsFinished()
		return
	}
	powerShell, err := resolvePowerShellExecutable(config.PowerShellExecutable)
	if err != nil {
		a.logger.Printf("settings open failed category=settings_shell")
		a.settingsFinished()
		return
	}
	executable, err := os.Executable()
	if err != nil {
		a.logger.Printf("settings open failed category=settings_executable")
		a.settingsFinished()
		return
	}
	arguments := []string{"-NoProfile", "-ExecutionPolicy", "Bypass", "-STA", "-File", scriptPath, "-DataDir", a.dataDir, "-Executable", executable}
	if os.Getenv("CODEX_AUTO_RETRY_SETTINGS_SMOKE") == "1" {
		arguments = append(arguments, "-SmokeTest")
	}
	command := exec.Command(powerShell, arguments...)
	command.SysProcAttr = &syscall.SysProcAttr{CreationFlags: windows.CREATE_NO_WINDOW}
	if err := command.Start(); err != nil {
		a.logger.Printf("settings open failed category=settings_start")
		a.settingsFinished()
		return
	}
	go func() {
		if err := command.Wait(); err != nil {
			a.logger.Printf("settings process stopped category=settings_process")
		}
		a.settingsFinished()
	}()
}

func (a *trayApp) settingsFinished() {
	a.settingsMu.Lock()
	a.settingsOpen = false
	a.settingsMu.Unlock()
}

func ensureSettingsScript(dataDir string) (string, error) {
	path := filepath.Join(dataDir, "settings.ps1")
	content := ensureUTF8BOM([]byte(settingsPowerShell))
	current, _ := os.ReadFile(path)
	if string(current) == string(content) {
		return path, nil
	}
	if err := os.WriteFile(path, content, 0o600); err != nil {
		return "", err
	}
	return path, nil
}

func ensureUTF8BOM(content []byte) []byte {
	bom := []byte{0xef, 0xbb, 0xbf}
	if bytes.HasPrefix(content, bom) {
		return content
	}
	return append(bom, content...)
}

func copyUTF16(destination []uint16, value string) {
	encoded, _ := windows.UTF16FromString(value)
	if len(encoded) > len(destination) {
		encoded = encoded[:len(destination)]
	}
	copy(destination, encoded)
	if len(destination) > 0 {
		destination[len(destination)-1] = 0
	}
}
