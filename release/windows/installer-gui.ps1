[CmdletBinding()]
param(
    [string]$PackageRoot = '',
    [string]$UserProfileRoot = $env:USERPROFILE,
    [string]$LocalAppDataRoot = $env:LOCALAPPDATA,
    [ValidateSet('', 'zh', 'en')][string]$Language = '',
    [switch]$SmokeTest
)

$ErrorActionPreference = 'Stop'
Set-StrictMode -Version 2
if ($env:OS -ne 'Windows_NT') { throw '安裝與控制中心只支援 Windows。 / The setup and control center supports Windows only.' }

Add-Type -AssemblyName System.Windows.Forms
Add-Type -AssemblyName System.Drawing
[System.Windows.Forms.Application]::EnableVisualStyles()

$packageRootPath = if ([string]::IsNullOrWhiteSpace($PackageRoot)) { $PSScriptRoot } else { [System.IO.Path]::GetFullPath($PackageRoot) }
$profileRootPath = [System.IO.Path]::GetFullPath($UserProfileRoot)
$localAppDataPath = [System.IO.Path]::GetFullPath($LocalAppDataRoot)
$runtimePath = Join-Path $localAppDataPath 'CodexAutoRetry'
$pluginPath = Join-Path $profileRootPath 'plugins\codex-auto-retry'
$configPath = Join-Path $runtimePath 'config.json'
$statusPath = Join-Path $runtimePath 'status.json'
$logPath = Join-Path $runtimePath 'logs\daemon.log'
$uiLangPath = Join-Path $runtimePath 'ui-language.json'
$manifestPath = Join-Path $packageRootPath 'release-manifest.json'

function Read-JsonSafe {
    param([string]$Path)
    if ([string]::IsNullOrWhiteSpace($Path) -or -not (Test-Path -LiteralPath $Path -PathType Leaf)) { return $null }
    try { return (Get-Content -Raw -Encoding UTF8 -LiteralPath $Path | ConvertFrom-Json) }
    catch { return $null }
}

function Hide-InstallerConsoleWindow {
    try {
        if (-not ('CodexAutoRetry.InstallerNativeWindow' -as [type])) {
            Add-Type @'
using System;
using System.Runtime.InteropServices;
namespace CodexAutoRetry {
    public static class InstallerNativeWindow {
        [DllImport("kernel32.dll")]
        public static extern IntPtr GetConsoleWindow();

        [DllImport("user32.dll")]
        [return: MarshalAs(UnmanagedType.Bool)]
        public static extern bool ShowWindow(IntPtr window, int command);
    }
}
'@
        }
        $console = [CodexAutoRetry.InstallerNativeWindow]::GetConsoleWindow()
        if ($console -ne [IntPtr]::Zero) {
            [void][CodexAutoRetry.InstallerNativeWindow]::ShowWindow($console, 0)
        }
    }
    catch { }
}

$script:lang = 'zh'
if ($Language) {
    $script:lang = $Language
}
else {
    $savedLang = Read-JsonSafe $uiLangPath
    if ($savedLang -and $savedLang.PSObject.Properties['language'] -and [string]$savedLang.language -eq 'en') {
        $script:lang = 'en'
    }
}

$i18n = @{
    title              = @{ zh = 'Codex Auto Retry 安裝與控制中心'; en = 'Codex Auto Retry Setup & Control Center' }
    subtitle           = @{ zh = '一鍵安裝與升級外掛、快速開啟設定檔與執行日誌、管理開機自動啟動'; en = 'One-click install & update, locate config & logs, and manage startup' }
    lang_btn           = @{ zh = 'English'; en = '中文' }
    status_group       = @{ zh = '目前狀態與檔案位置'; en = 'Current Status & File Locations' }
    options_group      = @{ zh = '安裝與系統整合選項'; en = 'Installation & System Integration Options' }
    actions_group      = @{ zh = '執行動作與進度紀錄'; en = 'Actions & Progress Log' }
    check_desktop      = @{ zh = '在桌面建立「Codex Auto Retry」捷徑'; en = 'Create a Desktop shortcut for Codex Auto Retry' }
    check_shared       = @{ zh = '啟用選用的共用後端（安裝前請關閉 Codex Desktop）'; en = 'Enable optional shared backend (close Codex Desktop before install)' }
    integration_note   = @{ zh = '✓ 安裝時會自動建立「開始功能表 > Codex Auto Retry」捷徑，並登錄至 Windows「已安裝的應用程式」。'; en = '✓ Automatically creates Start Menu shortcuts and registers in Windows Installed Apps.' }
    btn_install        = @{ zh = '立即安裝 / 更新並啟動服務'; en = 'Install / Update & Start Service' }
    btn_open_settings  = @{ zh = '開啟參數設定視窗'; en = 'Open Settings Window' }
    btn_open_manager   = @{ zh = '開啟啟動管理員'; en = 'Open Startup Manager' }
    btn_open_folder    = @{ zh = '開啟設定與日誌資料夾'; en = 'Open Config & Log Folder' }
    btn_open_log       = @{ zh = '檢視執行日誌 (daemon.log)'; en = 'View Daemon Log' }
    btn_refresh        = @{ zh = '重新整理狀態'; en = 'Refresh Status' }
    btn_safe_disable   = @{ zh = '緊急停用開機啟動'; en = 'Safe-Disable Startup' }
    btn_uninstall_keep = @{ zh = '解除安裝（保留設定）'; en = 'Uninstall (Keep Data)' }
    btn_uninstall_full = @{ zh = '完整移除（刪除資料）'; en = 'Full Uninstall (Delete Data)' }
    btn_close          = @{ zh = '關閉'; en = 'Close' }
    lbl_pkg_ver        = @{ zh = '安裝包版本'; en = 'Package Version' }
    lbl_inst_status    = @{ zh = '已安裝版本'; en = 'Installed Version' }
    lbl_svc_status     = @{ zh = '背景服務狀態'; en = 'Background Service' }
    lbl_data_dir       = @{ zh = '設定與日誌資料夾'; en = 'Config & Log Folder' }
    lbl_config_file    = @{ zh = '設定檔路徑'; en = 'Config File Path' }
    lbl_log_file       = @{ zh = '執行日誌路徑'; en = 'Daemon Log Path' }
    lbl_plugin_dir     = @{ zh = '外掛安裝位置'; en = 'Plugin Directory' }
    val_not_installed  = @{ zh = '尚未安裝'; en = 'Not installed' }
    val_running        = @{ zh = '執行中 (PID {0})'; en = 'Running (PID {0})' }
    val_stopped        = @{ zh = '未執行'; en = 'Not running' }
    log_ready          = @{ zh = '準備就緒。按一下「立即安裝 / 更新並啟動服務」即可完成部署。'; en = 'Ready. Click "Install / Update & Start Service" to deploy.' }
    log_running        = @{ zh = '正在執行操作，請稍候…'; en = 'Running operation, please wait...' }
    confirm_keep       = @{ zh = '要移除 Codex Auto Retry 外掛與背景服務，但保留設定檔與日誌嗎？'; en = 'Uninstall Codex Auto Retry plugin and background service while keeping config and logs?' }
    confirm_full       = @{ zh = '要完整移除 Codex Auto Retry 並刪除所有重試設定、狀態與日誌嗎？（不會影響 Codex 對話紀錄）'; en = 'Completely uninstall Codex Auto Retry and delete all retry settings, state, and logs? (Codex chat history is not touched.)' }
    confirm_title      = @{ zh = '確認解除安裝'; en = 'Confirm Uninstall' }
    msg_not_installed  = @{ zh = '尚未安裝設定視窗，請先執行安裝。'; en = 'Settings window is not installed yet. Please run Install first.' }
}

function T([string]$Key) {
    return $i18n[$Key][$script:lang]
}

$manifest = Read-JsonSafe $manifestPath
$packageVer = if ($manifest -and $manifest.PSObject.Properties['packageVersion']) { [string]$manifest.packageVersion } else { '1.2.0' }

$form = New-Object System.Windows.Forms.Form
$form.Text = T 'title'
$form.StartPosition = 'CenterScreen'
$form.FormBorderStyle = 'FixedDialog'
$form.MaximizeBox = $false
$form.ClientSize = New-Object System.Drawing.Size(700, 680)
$form.Font = New-Object System.Drawing.Font('Microsoft YaHei UI', 9)

$loadedIcon = $null
foreach ($candidate in @(
    (Join-Path $packageRootPath 'app-icon.ico'),
    (Join-Path $packageRootPath 'payload\codex-auto-retry\scripts\source\ui\app-icon.ico'),
    (Join-Path $runtimePath 'app-icon.ico')
)) {
    if ($candidate -and (Test-Path -LiteralPath $candidate -PathType Leaf)) {
        try { $loadedIcon = [System.Drawing.Icon]::new($candidate); break } catch { }
    }
}
if ($loadedIcon) { $form.Icon = $loadedIcon }
if ($SmokeTest) {
    $form.Opacity = 0
    $form.ShowInTaskbar = $false
}

$titleLabel = New-Object System.Windows.Forms.Label
$titleLabel.Location = New-Object System.Drawing.Point(22, 16)
$titleLabel.Size = New-Object System.Drawing.Size(540, 28)
$titleLabel.Font = New-Object System.Drawing.Font('Microsoft YaHei UI', 14, [System.Drawing.FontStyle]::Bold)
$form.Controls.Add($titleLabel)

$subtitleLabel = New-Object System.Windows.Forms.Label
$subtitleLabel.Location = New-Object System.Drawing.Point(24, 46)
$subtitleLabel.Size = New-Object System.Drawing.Size(540, 22)
$subtitleLabel.ForeColor = [System.Drawing.Color]::DimGray
$form.Controls.Add($subtitleLabel)

$langButton = New-Object System.Windows.Forms.Button
$langButton.Location = New-Object System.Drawing.Point(588, 18)
$langButton.Size = New-Object System.Drawing.Size(90, 28)
$langButton.Cursor = [System.Windows.Forms.Cursors]::Hand
$form.Controls.Add($langButton)

$statusGroup = New-Object System.Windows.Forms.GroupBox
$statusGroup.Location = New-Object System.Drawing.Point(20, 76)
$statusGroup.Size = New-Object System.Drawing.Size(660, 226)
$form.Controls.Add($statusGroup)

$statusBox = New-Object System.Windows.Forms.TextBox
$statusBox.Multiline = $true
$statusBox.ReadOnly = $true
$statusBox.BackColor = [System.Drawing.Color]::FromArgb(248, 250, 252)
$statusBox.BorderStyle = 'FixedSingle'
$statusBox.Font = New-Object System.Drawing.Font('Consolas', 9.5)
$statusBox.Location = New-Object System.Drawing.Point(16, 26)
$statusBox.Size = New-Object System.Drawing.Size(628, 146)
$statusGroup.Controls.Add($statusBox)

$openSettingsBtn = New-Object System.Windows.Forms.Button
$openSettingsBtn.Location = New-Object System.Drawing.Point(16, 182)
$openSettingsBtn.Size = New-Object System.Drawing.Size(134, 30)
$openManagerBtn = New-Object System.Windows.Forms.Button
$openManagerBtn.Location = New-Object System.Drawing.Point(156, 182)
$openManagerBtn.Size = New-Object System.Drawing.Size(124, 30)
$openFolderBtn = New-Object System.Windows.Forms.Button
$openFolderBtn.Location = New-Object System.Drawing.Point(286, 182)
$openFolderBtn.Size = New-Object System.Drawing.Size(146, 30)
$openLogBtn = New-Object System.Windows.Forms.Button
$openLogBtn.Location = New-Object System.Drawing.Point(438, 182)
$openLogBtn.Size = New-Object System.Drawing.Size(112, 30)
$refreshBtn = New-Object System.Windows.Forms.Button
$refreshBtn.Location = New-Object System.Drawing.Point(556, 182)
$refreshBtn.Size = New-Object System.Drawing.Size(88, 30)
$statusGroup.Controls.AddRange(@($openSettingsBtn, $openManagerBtn, $openFolderBtn, $openLogBtn, $refreshBtn))

$optionsGroup = New-Object System.Windows.Forms.GroupBox
$optionsGroup.Location = New-Object System.Drawing.Point(20, 312)
$optionsGroup.Size = New-Object System.Drawing.Size(660, 110)
$form.Controls.Add($optionsGroup)

$desktopShortcutCheck = New-Object System.Windows.Forms.CheckBox
$desktopShortcutCheck.Location = New-Object System.Drawing.Point(18, 26)
$desktopShortcutCheck.Size = New-Object System.Drawing.Size(620, 24)
$desktopShortcutCheck.Checked = $false

$sharedServerCheck = New-Object System.Windows.Forms.CheckBox
$sharedServerCheck.Location = New-Object System.Drawing.Point(18, 52)
$sharedServerCheck.Size = New-Object System.Drawing.Size(620, 24)
$sharedServerCheck.Checked = $false

$integrationNoteLabel = New-Object System.Windows.Forms.Label
$integrationNoteLabel.Location = New-Object System.Drawing.Point(18, 80)
$integrationNoteLabel.Size = New-Object System.Drawing.Size(624, 22)
$integrationNoteLabel.ForeColor = [System.Drawing.Color]::SeaGreen
$optionsGroup.Controls.AddRange(@($desktopShortcutCheck, $sharedServerCheck, $integrationNoteLabel))

$actionsGroup = New-Object System.Windows.Forms.GroupBox
$actionsGroup.Location = New-Object System.Drawing.Point(20, 432)
$actionsGroup.Size = New-Object System.Drawing.Size(660, 232)
$form.Controls.Add($actionsGroup)

$installBtn = New-Object System.Windows.Forms.Button
$installBtn.Location = New-Object System.Drawing.Point(16, 26)
$installBtn.Size = New-Object System.Drawing.Size(218, 36)
$installBtn.BackColor = [System.Drawing.Color]::FromArgb(16, 124, 65)
$installBtn.ForeColor = [System.Drawing.Color]::White
$installBtn.FlatStyle = 'Flat'
$installBtn.Font = New-Object System.Drawing.Font('Microsoft YaHei UI', 9.5, [System.Drawing.FontStyle]::Bold)

$safeDisableBtn = New-Object System.Windows.Forms.Button
$safeDisableBtn.Location = New-Object System.Drawing.Point(242, 26)
$safeDisableBtn.Size = New-Object System.Drawing.Size(132, 36)

$uninstallKeepBtn = New-Object System.Windows.Forms.Button
$uninstallKeepBtn.Location = New-Object System.Drawing.Point(380, 26)
$uninstallKeepBtn.Size = New-Object System.Drawing.Size(130, 36)

$uninstallFullBtn = New-Object System.Windows.Forms.Button
$uninstallFullBtn.Location = New-Object System.Drawing.Point(516, 26)
$uninstallFullBtn.Size = New-Object System.Drawing.Size(128, 36)
$uninstallFullBtn.ForeColor = [System.Drawing.Color]::Firebrick

$logBox = New-Object System.Windows.Forms.TextBox
$logBox.Multiline = $true
$logBox.ReadOnly = $true
$logBox.ScrollBars = 'Vertical'
$logBox.Font = New-Object System.Drawing.Font('Consolas', 9)
$logBox.Location = New-Object System.Drawing.Point(16, 72)
$logBox.Size = New-Object System.Drawing.Size(628, 144)
$actionsGroup.Controls.AddRange(@($installBtn, $safeDisableBtn, $uninstallKeepBtn, $uninstallFullBtn, $logBox))

function Update-InstallerStatusView {
    $installedManifest = Read-JsonSafe (Join-Path $pluginPath '.codex-plugin\plugin.json')
    $status = Read-JsonSafe $statusPath
    $watchdogExe = Join-Path $runtimePath 'codex-auto-retry.exe'
    $installedVer = if ($installedManifest -and $installedManifest.PSObject.Properties['version']) {
        'v' + [string]$installedManifest.version
    }
    elseif ($status -and $status.PSObject.Properties['version']) {
        'v' + [string]$status.version
    }
    else {
        T 'val_not_installed'
    }

    $runningPid = $null
    if ($status -and [bool]$status.running -and [int]$status.pid -gt 0) {
        try {
            $proc = Get-Process -Id ([int]$status.pid) -ErrorAction Stop
            if ([string]::Equals($proc.Path, $watchdogExe, [System.StringComparison]::OrdinalIgnoreCase)) {
                $runningPid = [int]$status.pid
            }
        }
        catch { }
    }
    $svcText = if ($null -ne $runningPid) { [string]::Format((T 'val_running'), $runningPid) } else { T 'val_stopped' }

    $lines = @(
        ("{0,-18} : v{1}" -f (T 'lbl_pkg_ver'), $packageVer),
        ("{0,-18} : {1}" -f (T 'lbl_inst_status'), $installedVer),
        ("{0,-18} : {1}" -f (T 'lbl_svc_status'), $svcText),
        ("{0,-18} : {1}" -f (T 'lbl_data_dir'), $runtimePath),
        ("{0,-18} : {1}" -f (T 'lbl_config_file'), $configPath),
        ("{0,-18} : {1}" -f (T 'lbl_log_file'), $logPath),
        ("{0,-18} : {1}" -f (T 'lbl_plugin_dir'), $pluginPath)
    )
    $statusBox.Text = $lines -join [Environment]::NewLine
}

function Apply-InstallerLanguage {
    $form.Text = T 'title'
    $titleLabel.Text = T 'title'
    $subtitleLabel.Text = T 'subtitle'
    $langButton.Text = T 'lang_btn'
    $statusGroup.Text = T 'status_group'
    $optionsGroup.Text = T 'options_group'
    $actionsGroup.Text = T 'actions_group'
    $desktopShortcutCheck.Text = T 'check_desktop'
    $sharedServerCheck.Text = T 'check_shared'
    $integrationNoteLabel.Text = T 'integration_note'
    $installBtn.Text = T 'btn_install'
    $openSettingsBtn.Text = T 'btn_open_settings'
    $openManagerBtn.Text = T 'btn_open_manager'
    $openFolderBtn.Text = T 'btn_open_folder'
    $openLogBtn.Text = T 'btn_open_log'
    $refreshBtn.Text = T 'btn_refresh'
    $safeDisableBtn.Text = T 'btn_safe_disable'
    $uninstallKeepBtn.Text = T 'btn_uninstall_keep'
    $uninstallFullBtn.Text = T 'btn_uninstall_full'
    if ([string]::IsNullOrWhiteSpace($logBox.Text)) {
        $logBox.Text = T 'log_ready'
    }
    Update-InstallerStatusView
}

function Invoke-PackageScript {
    param([string]$ScriptName, [string[]]$ScriptArgs = @())
    $scriptPath = Join-Path $packageRootPath $ScriptName
    if (-not (Test-Path -LiteralPath $scriptPath -PathType Leaf)) {
        throw "找不到安裝包指令碼：$scriptPath / Package script not found: $scriptPath"
    }
    $psExe = Join-Path $env:WINDIR 'System32\WindowsPowerShell\v1.0\powershell.exe'
    if (-not (Test-Path -LiteralPath $psExe -PathType Leaf)) { $psExe = 'powershell.exe' }

    $installBtn.Enabled = $false
    $safeDisableBtn.Enabled = $false
    $uninstallKeepBtn.Enabled = $false
    $uninstallFullBtn.Enabled = $false
    $logBox.Text = (T 'log_running') + [Environment]::NewLine
    [System.Windows.Forms.Application]::DoEvents()

    try {
        $allArgs = @('-NoLogo', '-NoProfile', '-NonInteractive', '-ExecutionPolicy', 'Bypass', '-File', $scriptPath) + $ScriptArgs
        $output = (& $psExe @allArgs 2>&1 | Out-String)
        $exitCode = $LASTEXITCODE
        $logBox.Text = $output.Trim()
        $logBox.SelectionStart = $logBox.Text.Length
        $logBox.ScrollToCaret()
        Update-InstallerStatusView
        if ($exitCode -ne 0) {
            [System.Windows.Forms.MessageBox]::Show(
                $output.Trim(),
                'Codex Auto Retry',
                [System.Windows.Forms.MessageBoxButtons]::OK,
                [System.Windows.Forms.MessageBoxIcon]::Error
            ) | Out-Null
        }
    }
    finally {
        $installBtn.Enabled = $true
        $safeDisableBtn.Enabled = $true
        $uninstallKeepBtn.Enabled = $true
        $uninstallFullBtn.Enabled = $true
    }
}

$langButton.Add_Click({
    $script:lang = if ($script:lang -eq 'zh') { 'en' } else { 'zh' }
    try {
        if (Test-Path -LiteralPath $runtimePath -PathType Container) {
            [System.IO.File]::WriteAllText($uiLangPath, (@{ language = $script:lang } | ConvertTo-Json -Compress), [System.Text.UTF8Encoding]::new($false))
        }
    }
    catch { }
    Apply-InstallerLanguage
})

$refreshBtn.Add_Click({ Update-InstallerStatusView })

$openFolderBtn.Add_Click({
    New-Item -ItemType Directory -Force -Path $runtimePath | Out-Null
    Start-Process -FilePath 'explorer.exe' -ArgumentList @($runtimePath) | Out-Null
})

$openLogBtn.Add_Click({
    $logDir = Split-Path -Parent $logPath
    New-Item -ItemType Directory -Force -Path $logDir | Out-Null
    if (-not (Test-Path -LiteralPath $logPath -PathType Leaf)) {
        [System.IO.File]::WriteAllText($logPath, '', [System.Text.UTF8Encoding]::new($false))
    }
    Start-Process -FilePath 'notepad.exe' -ArgumentList @($logPath) | Out-Null
})

$openSettingsBtn.Add_Click({
    $settingsScript = Join-Path $runtimePath 'settings.ps1'
    $watchdogExe = Join-Path $runtimePath 'codex-auto-retry.exe'
    if (-not (Test-Path -LiteralPath $settingsScript -PathType Leaf)) {
        $settingsScript = Join-Path $packageRootPath 'payload\codex-auto-retry\scripts\source\ui\settings.ps1'
    }
    if (-not (Test-Path -LiteralPath $watchdogExe -PathType Leaf)) {
        $watchdogExe = Join-Path $packageRootPath 'payload\codex-auto-retry\scripts\bin\codex-auto-retry.exe'
    }
    if (-not (Test-Path -LiteralPath $settingsScript -PathType Leaf) -or -not (Test-Path -LiteralPath $configPath -PathType Leaf)) {
        [System.Windows.Forms.MessageBox]::Show((T 'msg_not_installed'), 'Codex Auto Retry', 'OK', 'Information') | Out-Null
        return
    }
    Start-Process -FilePath 'powershell.exe' -ArgumentList @('-NoProfile', '-ExecutionPolicy', 'Bypass', '-STA', '-File', $settingsScript, '-DataDir', $runtimePath, '-Executable', $watchdogExe) -WindowStyle Hidden | Out-Null
})

$openManagerBtn.Add_Click({
    $mgrScript = Join-Path $packageRootPath 'startup-manager.ps1'
    if (-not (Test-Path -LiteralPath $mgrScript -PathType Leaf)) {
        $mgrScript = Join-Path $runtimePath 'startup-manager.ps1'
    }
    if (Test-Path -LiteralPath $mgrScript -PathType Leaf) {
        Start-Process -FilePath 'powershell.exe' -ArgumentList @('-NoProfile', '-ExecutionPolicy', 'Bypass', '-File', $mgrScript, '-Action', 'gui', '-ReleaseRoot', $packageRootPath) -WindowStyle Hidden | Out-Null
    }
})

$installBtn.Add_Click({
    $deployArgs = @('-UserProfileRoot', $profileRootPath, '-LocalAppDataRoot', $localAppDataPath)
    if ($sharedServerCheck.Checked) { $deployArgs += '-EnableSharedAppServer' }
    if ($desktopShortcutCheck.Checked) { $deployArgs += '-CreateDesktopShortcut' }
    Invoke-PackageScript -ScriptName 'deploy.ps1' -ScriptArgs $deployArgs
})

$safeDisableBtn.Add_Click({
    Invoke-PackageScript -ScriptName 'startup-manager.ps1' -ScriptArgs @('-Action', 'safe-disable', '-ReleaseRoot', $packageRootPath, '-UserProfileRoot', $profileRootPath, '-LocalAppDataRoot', $localAppDataPath)
})

$uninstallKeepBtn.Add_Click({
    $ans = [System.Windows.Forms.MessageBox]::Show((T 'confirm_keep'), (T 'confirm_title'), 'YesNo', 'Question')
    if ($ans -ne [System.Windows.Forms.DialogResult]::Yes) { return }
    Invoke-PackageScript -ScriptName 'uninstall-release.ps1' -ScriptArgs @('-UserProfileRoot', $profileRootPath, '-LocalAppDataRoot', $localAppDataPath)
})

$uninstallFullBtn.Add_Click({
    $ans = [System.Windows.Forms.MessageBox]::Show((T 'confirm_full'), (T 'confirm_title'), 'YesNo', 'Warning')
    if ($ans -ne [System.Windows.Forms.DialogResult]::Yes) { return }
    Invoke-PackageScript -ScriptName 'uninstall-release.ps1' -ScriptArgs @('-RemoveData', '-UserProfileRoot', $profileRootPath, '-LocalAppDataRoot', $localAppDataPath)
})

$form.Add_Shown({
    Hide-InstallerConsoleWindow
    Apply-InstallerLanguage
    if ($SmokeTest) {
        New-Item -ItemType Directory -Force -Path $localAppDataPath | Out-Null
        [System.IO.File]::WriteAllText((Join-Path $localAppDataPath 'installer-gui-smoke.ok'), 'passed', [System.Text.UTF8Encoding]::new($false))
        $form.Close()
    }
})

[void]$form.ShowDialog()
