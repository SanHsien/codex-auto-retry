[CmdletBinding()]
param(
    [ValidateSet('gui', 'status', 'enable', 'disable', 'start', 'stop', 'launch-codex', 'safe-disable', 'uninstall')]
    [string]$Action = 'gui',
    [switch]$RemoveData,
    [switch]$NoPrompt,
    [string]$UserProfileRoot = $env:USERPROFILE,
    [string]$LocalAppDataRoot = $env:LOCALAPPDATA,
    [string]$RunName = 'CodexAutoRetry',
    [string]$ReleaseRoot = '',
    [ValidateSet('', 'zh', 'en')]
    [string]$Language = ''
)

$ErrorActionPreference = 'Stop'
Set-StrictMode -Version 2
if ($env:OS -ne 'Windows_NT') { throw 'The startup manager supports Windows only.' }

$profileRoot = [System.IO.Path]::GetFullPath($UserProfileRoot)
$localAppDataRoot = [System.IO.Path]::GetFullPath($LocalAppDataRoot)
$installDir = Join-Path $localAppDataRoot 'CodexAutoRetry'
$watchdog = Join-Path $installDir 'codex-auto-retry.exe'
$pluginTarget = Join-Path $profileRoot 'plugins\codex-auto-retry'
$statusPath = Join-Path $installDir 'status.json'
$configPath = Join-Path $installDir 'config.json'
$sharedStatePath = Join-Path $installDir 'shared-server.json'
$runSubKey = 'Software\Microsoft\Windows\CurrentVersion\Run'
. (Join-Path $PSScriptRoot 'startup-approval.ps1')
. (Join-Path $PSScriptRoot 'shared-server-status.ps1')

function Open-RunKey {
    param([bool]$Writable)
    return [Microsoft.Win32.Registry]::CurrentUser.OpenSubKey($runSubKey, $Writable)
}

function Get-RunValue {
    $key = Open-RunKey -Writable $false
    if ($null -eq $key) { return '' }
    try {
        $value = $key.GetValue($RunName, $null, [Microsoft.Win32.RegistryValueOptions]::DoNotExpandEnvironmentNames)
        if ($null -eq $value) { return '' }
        return [string]$value
    }
    finally {
        $key.Close()
    }
}

function Test-OwnedStartupValue {
    param([AllowNull()][string]$Value)
    if ([string]::IsNullOrWhiteSpace($Value)) { return $false }
    $trimmed = $Value.Trim()
    if ($trimmed.StartsWith('"')) {
        $closingQuote = $trimmed.IndexOf('"', 1)
        if ($closingQuote -le 1) { return $false }
        $executable = $trimmed.Substring(1, $closingQuote - 1)
    }
    else {
        $executable = ($trimmed -split '[\s\t]', 2)[0]
    }
    return [string]::Equals($executable, $watchdog, [System.StringComparison]::OrdinalIgnoreCase)
}

function Restore-ManagedStartupValue {
    param([AllowNull()][string]$Value)
    $key = Open-RunKey -Writable $true
    if ($null -eq $key) {
        if ([string]::IsNullOrWhiteSpace($Value)) { return }
        throw 'The current-user startup registry key could not be opened while restoring the previous value.'
    }
    try {
        if ([string]::IsNullOrWhiteSpace($Value)) {
            $key.DeleteValue($RunName, $false)
        }
        else {
            $key.SetValue($RunName, $Value, [Microsoft.Win32.RegistryValueKind]::String)
        }
    }
    finally {
        $key.Close()
    }
}

function Set-ManagedStartup {
    if (-not (Test-Path -LiteralPath $watchdog -PathType Leaf)) {
        throw "The watchdog executable is missing: $watchdog"
    }
    $existing = Get-RunValue
    if (-not [string]::IsNullOrWhiteSpace($existing) -and -not (Test-OwnedStartupValue $existing)) {
        throw "The startup entry $RunName belongs to another command and was not changed."
    }
    $oldApproval = Get-CodexAutoRetryStartupApproval -RunName $RunName
    $desiredValue = ('"{0}" supervise' -f $watchdog)
    [byte[]]$expectedApprovalBytes = @(Get-CodexAutoRetryStartupApprovalEnabledBytes -ExistingBytes $(if ($oldApproval.Present) { [byte[]]$oldApproval.Bytes } else { $null }))
    try {
        $key = Open-RunKey -Writable $true
        if ($null -eq $key) {
            $key = [Microsoft.Win32.Registry]::CurrentUser.CreateSubKey($runSubKey, $true)
        }
        if ($null -eq $key) { throw 'The current-user startup registry key could not be opened.' }
        try {
            $key.SetValue($RunName, $desiredValue, [Microsoft.Win32.RegistryValueKind]::String)
        }
        finally {
            $key.Close()
        }
        $null = Set-CodexAutoRetryStartupApprovalEnabled -RunName $RunName
        $actual = Get-RunValue
        if (-not (Test-OwnedStartupValue $actual) -or $actual -notmatch '(?i)\bsupervise\b') {
            throw 'The startup entry could not be registered in supervised mode.'
        }
    }
    catch {
        try {
            # Restore only while the registry still contains the value this
            # operation wrote. A concurrent user or installer change is left
            # untouched and reported through the original failure.
            if ((Get-RunValue) -eq $desiredValue) {
                $currentApproval = Get-CodexAutoRetryStartupApproval -RunName $RunName
                $currentApprovalBytes = if ($currentApproval.Present) { [byte[]]$currentApproval.Bytes } else { $null }
                $approvalWasOld = Test-CodexAutoRetryStartupApprovalBytes -Left $currentApprovalBytes -Right $(if ($oldApproval.Present) { [byte[]]$oldApproval.Bytes } else { $null })
                $approvalWasWritten = Test-CodexAutoRetryStartupApprovalBytes -Left $currentApprovalBytes -Right $expectedApprovalBytes
                if ($approvalWasOld -or $approvalWasWritten) {
                    Restore-ManagedStartupValue -Value $existing
                    if ($approvalWasWritten) {
                        if ($oldApproval.Present) {
                            Restore-CodexAutoRetryStartupApproval -RunName $RunName -Bytes ([byte[]]$oldApproval.Bytes)
                        }
                        else {
                            $null = Remove-CodexAutoRetryStartupApproval -RunName $RunName
                        }
                    }
                }
            }
        }
        catch {
            # Preserve the original failure. The next status/repair pass can
            # report the remaining registry inconsistency without masking it.
        }
        throw
    }
    return $actual
}

function Remove-ManagedStartup {
    $existing = Get-RunValue
    if (-not [string]::IsNullOrWhiteSpace($existing) -and -not (Test-OwnedStartupValue $existing)) {
        throw "The startup entry $RunName belongs to another command and was not removed."
    }
    $oldApproval = Get-CodexAutoRetryStartupApproval -RunName $RunName
    $removedRun = $false
    try {
        # Re-read immediately before deletion so a concurrent installer or
        # user cannot replace the checked owned command with a foreign one.
        $current = Get-RunValue
        if ($current -ne $existing) {
            throw "The startup entry $RunName changed while it was being removed."
        }
        if (-not [string]::IsNullOrWhiteSpace($existing)) {
            $key = Open-RunKey -Writable $true
            if ($null -ne $key) {
                try { $key.DeleteValue($RunName, $false); $removedRun = $true }
                finally { $key.Close() }
            }
        }
        if (-not [string]::IsNullOrWhiteSpace((Get-RunValue))) {
            throw 'The plugin startup entry is still present after removal.'
        }
        $removedApproval = Remove-CodexAutoRetryStartupApproval -RunName $RunName
        return $removedRun -or $removedApproval
    }
    catch {
        try {
            # Roll back only an unchanged post-delete state. Never overwrite a
            # foreign command that appeared while the removal was in flight.
            $currentRunAfterFailure = Get-RunValue
            $currentApproval = Get-CodexAutoRetryStartupApproval -RunName $RunName
            $currentApprovalBytes = if ($currentApproval.Present) { [byte[]]$currentApproval.Bytes } else { $null }
            $approvalWasOld = Test-CodexAutoRetryStartupApprovalBytes -Left $currentApprovalBytes -Right $(if ($oldApproval.Present) { [byte[]]$oldApproval.Bytes } else { $null })
            $approvalWasRemoved = -not $currentApproval.Present
            $runWasRemoved = [string]::IsNullOrWhiteSpace($currentRunAfterFailure)
            $runWasUnchanged = $currentRunAfterFailure -eq $existing
            if (($runWasRemoved -or $runWasUnchanged) -and ($approvalWasOld -or $approvalWasRemoved)) {
                if ($runWasRemoved) {
                    Restore-ManagedStartupValue -Value $existing
                }
                if ($approvalWasRemoved -and $oldApproval.Present) {
                    Restore-CodexAutoRetryStartupApproval -RunName $RunName -Bytes ([byte[]]$oldApproval.Bytes)
                }
            }
        }
        catch {
            # Preserve the original failure and leave status visible for a
            # subsequent explicit repair.
        }
        throw
    }
}

function Get-ManagerProcesses {
    if (-not (Test-Path -LiteralPath $watchdog -PathType Leaf)) { return @() }
    return @(Get-CimInstance Win32_Process -ErrorAction SilentlyContinue |
        Where-Object {
            $_.ExecutablePath -and
            [string]::Equals($_.ExecutablePath, $watchdog, [System.StringComparison]::OrdinalIgnoreCase)
        })
}

function Test-HeartbeatFresh {
    param(
        [Parameter(Mandatory = $true)]$Value,
        [TimeSpan]$MaxAge = ([TimeSpan]::FromSeconds(15))
    )
    try {
        $timestamp = if ($Value -is [DateTime]) {
            [DateTimeOffset]$Value
        }
        elseif ($Value -is [DateTimeOffset]) {
            $Value
        }
        else {
            [DateTimeOffset]::Parse([string]$Value)
        }
        $age = [DateTimeOffset]::UtcNow - $timestamp.ToUniversalTime()
        return $age -ge [TimeSpan]::Zero -and $age -le $MaxAge
    }
    catch {
        return $false
    }
}

function Read-JsonOrNull {
    param([Parameter(Mandatory = $true)][string]$Path)
    if (-not (Test-Path -LiteralPath $Path -PathType Leaf)) { return $null }
    try { return (Get-Content -Raw -Encoding UTF8 -LiteralPath $Path | ConvertFrom-Json) }
    catch { return $null }
}

function Get-ManagerState {
    $status = Read-JsonOrNull -Path $statusPath
    $config = Read-JsonOrNull -Path $configPath
    $sharedState = Read-JsonOrNull -Path $sharedStatePath
    $statusReadFailed = (Test-Path -LiteralPath $statusPath -PathType Leaf) -and $null -eq $status
    $sharedStateReadFailed = (Test-Path -LiteralPath $sharedStatePath -PathType Leaf) -and $null -eq $sharedState
    $processes = @(Get-ManagerProcesses)
    $heartbeatFresh = $false
    if ($status -and $status.last_scan_at) {
        $heartbeatFresh = Test-HeartbeatFresh -Value $status.last_scan_at
    }
    $serviceRunning = $null -ne $status -and [bool]$status.running -and $heartbeatFresh -and $processes.Count -gt 0
    $startupEntry = Get-RunValue
    $startupApproval = Get-CodexAutoRetryStartupApproval -RunName $RunName
    $startupMode = if ([string]::IsNullOrWhiteSpace($startupEntry)) {
        'missing'
    }
    elseif ($startupEntry -match '(?i)\bsupervise\b') {
        'supervise'
    }
    elseif ($startupEntry -match '(?i)\brun\b') {
        'run'
    }
    else {
        'unknown'
    }
    $expectedSharedPort = if ($config -and $config.PSObject.Properties['shared_app_server_port']) { [int]$config.shared_app_server_port } else { 0 }
    $statusCompatibility = Get-CodexAutoRetryStatusCompatibility -Status $status -ReadFailed $statusReadFailed
    $sharedVerification = if ($sharedStateReadFailed) {
        [pscustomobject][ordered]@{ Status = 'unknown'; Reason = 'state_unreadable'; PID = $null; Endpoint = $null }
    } else {
        Get-CodexAutoRetrySharedServerStatus -State $sharedState -ExpectedPort $expectedSharedPort
    }
    $sharedStateStatus = [string]$sharedVerification.Status
    $endpoint = [Environment]::GetEnvironmentVariable('CODEX_APP_SERVER_WS_URL', 'User')
    $manifest = Read-JsonOrNull -Path (Join-Path $pluginTarget '.codex-plugin\plugin.json')
    return [pscustomobject][ordered]@{
        PluginInstalled = $null -ne $manifest -and [string]$manifest.name -eq 'codex-auto-retry'
        PluginVersion = if ($manifest) { [string]$manifest.version } else { $null }
        InstallDir = $installDir
        ServiceRunning = $serviceRunning
        ProcessCount = $processes.Count
        ProcessIds = @($processes | ForEach-Object { [int]$_.ProcessId })
        HeartbeatFresh = $heartbeatFresh
        RuntimeVersion = if ($status) { [string]$status.version } else { $null }
        LastScanAt = if ($status) { $status.last_scan_at } else { $null }
        StartupMode = $startupMode
        StartupEntry = if ([string]::IsNullOrWhiteSpace($startupEntry)) { $null } else { $startupEntry }
        StartupOwned = Test-OwnedStartupValue $startupEntry
        StartupApproved = $startupApproval.Status
        SharedModeEnabled = if ($config -and $config.PSObject.Properties['shared_app_server_enabled']) { [bool]$config.shared_app_server_enabled } else { $false }
        SharedModeRequested = if ($config -and $config.PSObject.Properties['shared_app_server_requested']) { [bool]$config.shared_app_server_requested } elseif ($config -and $config.PSObject.Properties['shared_app_server_enabled']) { [bool]$config.shared_app_server_enabled } else { $false }
        SharedEndpointConfigured = -not [string]::IsNullOrWhiteSpace($endpoint)
        DesktopLaunchMode = Get-CodexAutoRetryStatusProperty -Status $status -Name 'desktop_launch_mode' -Default 'legacy_unprotected'
        SafeLauncher = Join-Path $PSScriptRoot 'launch-codex.ps1'
        SharedServerState = $sharedStateStatus
        SharedServerVerification = [string]$sharedVerification.Reason
        SharedAppServerMemoryUsageMB = Get-CodexAutoRetryStatusProperty -Status $status -Name 'shared_app_server_memory_usage_mb' -Default 0
        SharedAppServerMemoryLimitMB = Get-CodexAutoRetryStatusProperty -Status $status -Name 'shared_app_server_memory_limit_mb' -Default $null
        SharedAppServerMemoryGuardTriggered = [bool](Get-CodexAutoRetryStatusProperty -Status $status -Name 'shared_app_server_memory_guard_triggered' -Default $false)
        RetrySafetyWarning = [string](Get-CodexAutoRetryStatusProperty -Status $status -Name 'retry_safety_warning' -Default '')
        StatusCompatibility = $statusCompatibility
        StatusCompatibilityMessage = Get-CodexAutoRetryStatusCompatibilityMessage -Status $statusCompatibility
        DataDirectoryExists = Test-Path -LiteralPath $installDir -PathType Container
    }
}

function Wait-ManagerServiceState {
    param([Parameter(Mandatory = $true)][bool]$Running, [int]$TimeoutSeconds = 15)
    $deadline = (Get-Date).AddSeconds($TimeoutSeconds)
    do {
        Start-Sleep -Milliseconds 250
        $state = Get-ManagerState
        if ($state.ServiceRunning -eq $Running) { return $state }
    } while ((Get-Date) -lt $deadline)
    return Get-ManagerState
}

function Start-ManagedService {
    $current = Get-ManagerState
    if ($current.ServiceRunning) { return $current }
    if (-not (Test-Path -LiteralPath $watchdog -PathType Leaf)) {
        throw "The watchdog executable is missing: $watchdog"
    }
    New-Item -ItemType Directory -Force -Path $installDir | Out-Null
    Remove-Item -LiteralPath (Join-Path $installDir 'stop.signal') -Force -ErrorAction SilentlyContinue
    Remove-Item -LiteralPath (Join-Path $installDir 'supervisor.stop') -Force -ErrorAction SilentlyContinue
    Start-Process -FilePath $watchdog -ArgumentList @('supervise') -WorkingDirectory $installDir -WindowStyle Hidden | Out-Null
    $state = Wait-ManagerServiceState -Running $true
    if (-not $state.ServiceRunning) { throw 'The watchdog did not publish a fresh heartbeat after starting.' }
    return $state
}

function Stop-ManagedService {
    if (-not (Test-Path -LiteralPath $installDir -PathType Container)) { return Get-ManagerState }
    New-Item -ItemType File -Force -Path (Join-Path $installDir 'supervisor.stop') | Out-Null
    New-Item -ItemType File -Force -Path (Join-Path $installDir 'stop.signal') | Out-Null
    $deadline = (Get-Date).AddSeconds(12)
    do {
        $processes = @(Get-ManagerProcesses)
        if ($processes.Count -eq 0) { break }
        Start-Sleep -Milliseconds 250
    } while ((Get-Date) -lt $deadline)
    $remaining = @(Get-ManagerProcesses)
    if ($remaining.Count -gt 0) {
        throw 'The watchdog did not stop gracefully. No process was force-terminated; use the safe-disable action after Codex closes.'
    }
    return Get-ManagerState
}

function Invoke-ManagedScript {
    param(
        [Parameter(Mandatory = $true)][string]$Path,
        [string[]]$Arguments = @()
    )
    if (-not (Test-Path -LiteralPath $Path -PathType Leaf)) { throw "Required maintenance script is missing: $Path" }
    $output = (& powershell.exe -NoLogo -NoProfile -NonInteractive -ExecutionPolicy Bypass -File $Path @Arguments 2>&1 | Out-String)
    if ($LASTEXITCODE -ne 0) { throw "Maintenance script failed with exit code $LASTEXITCODE.`n$($output.Trim())" }
    return $output
}

function Invoke-ManagerSafeDisable {
    $safeDisable = Join-Path $pluginTarget 'scripts\safe-disable.ps1'
    if (-not (Test-Path -LiteralPath $safeDisable -PathType Leaf) -and -not [string]::IsNullOrWhiteSpace($ReleaseRoot)) {
        $safeDisable = Join-Path $ReleaseRoot 'payload\codex-auto-retry\scripts\safe-disable.ps1'
    }
    $null = Invoke-ManagedScript -Path $safeDisable -Arguments @('-DataDir', $installDir)
    return Get-ManagerState
}

function Invoke-ManagerUninstall {
    $uninstaller = ''
    if (-not [string]::IsNullOrWhiteSpace($ReleaseRoot)) {
        $uninstaller = Join-Path $ReleaseRoot 'uninstall-release.ps1'
    }
    if (-not (Test-Path -LiteralPath $uninstaller -PathType Leaf)) {
        throw 'For a complete uninstall, run this manager from the extracted release folder so it can remove the Codex plugin registration safely.'
    }
    $arguments = @('-UserProfileRoot', $profileRoot, '-LocalAppDataRoot', $localAppDataRoot)
    if ($RemoveData) { $arguments += '-RemoveData' }
    $null = Invoke-ManagedScript -Path $uninstaller -Arguments $arguments
    return [pscustomobject][ordered]@{
        Uninstalled = $true
        DataRemoved = [bool]$RemoveData
        StartupEntry = Get-RunValue
        RuntimePathExists = Test-Path -LiteralPath $installDir -PathType Container
        PluginPathExists = Test-Path -LiteralPath $pluginTarget -PathType Container
    }
}

function Invoke-ManagerAction {
    param([Parameter(Mandatory = $true)][string]$RequestedAction)
    switch ($RequestedAction) {
        'status' { return Get-ManagerState }
        'enable' { $null = Set-ManagedStartup; return Get-ManagerState }
        'disable' { $null = Remove-ManagedStartup; return Get-ManagerState }
        'start' { return Start-ManagedService }
        'stop' { return Stop-ManagedService }
        'launch-codex' {
            $null = Invoke-ManagedScript -Path (Join-Path $PSScriptRoot 'launch-codex.ps1') -Arguments @('-DataDir', $installDir)
            return Get-ManagerState
        }
        'safe-disable' { return Invoke-ManagerSafeDisable }
        'uninstall' {
            if ($RemoveData -and -not $NoPrompt) {
                throw 'Destructive data removal requires -NoPrompt when invoked without the graphical manager.'
            }
            return Invoke-ManagerUninstall
        }
        default { throw "Unsupported manager action: $RequestedAction" }
    }
}

# Interface text. The language follows the settings window's saved choice
# (ui-language.json) unless -Language is given; Traditional Chinese is default.
$managerText = @{
    title             = @{ zh = 'Codex Auto Retry 啟動管理員'; en = 'Codex Auto Retry Startup Manager' }
    subtitle          = @{ zh = '檢視開機啟動、背景服務與共用後端狀態。所有操作只影響本外掛。'; en = 'Inspect startup, service, and shared-backend state. Actions are limited to this plugin.' }
    language          = @{ zh = 'English'; en = '中文' }
    refresh           = @{ zh = '重新整理狀態'; en = 'Refresh status' }
    'launch-codex'    = @{ zh = '安全啟動 Codex'; en = 'Launch Codex safely' }
    enable            = @{ zh = '啟用開機啟動'; en = 'Enable startup' }
    disable           = @{ zh = '停用開機啟動'; en = 'Disable startup' }
    start             = @{ zh = '啟動服務'; en = 'Start service' }
    stop              = @{ zh = '停止服務'; en = 'Stop service' }
    'safe-disable'    = @{ zh = '緊急停用共用後端'; en = 'Safe-disable shared backend' }
    uninstall         = @{ zh = '解除安裝（保留資料）'; en = 'Uninstall, keep data' }
    'uninstall-remove' = @{ zh = '解除安裝並刪除資料'; en = 'Uninstall and remove data' }
    confirmFull       = @{ zh = '這會移除外掛、執行狀態、設定與日誌，不會動到對話資料。要繼續嗎？'; en = 'This removes the plugin, runtime state, settings, and logs. Chat data is not touched. Continue?' }
    confirmFullTitle  = @{ zh = '確認完整解除安裝'; en = 'Confirm full uninstall' }
    doneFull          = @{ zh = '已完整解除安裝。'; en = 'Full uninstall completed.' }
    confirmKeep       = @{ zh = '這會移除外掛、開機啟動項目與背景服務，但保留重試設定、狀態與日誌。要繼續嗎？'; en = 'This removes the plugin, startup entry, and service, but keeps retry settings, state, and logs. Continue?' }
    confirmKeepTitle  = @{ zh = '確認解除安裝'; en = 'Confirm uninstall' }
    doneKeep          = @{ zh = '已解除安裝，執行資料已保留。'; en = 'Uninstall completed. Runtime data was kept.' }
    failedTitle       = @{ zh = '操作失敗'; en = 'Action failed' }
}
$managerStateLabels = @{
    PluginInstalled = '外掛已安裝'; PluginVersion = '外掛版本'; InstallDir = '安裝資料夾'
    ServiceRunning = '服務執行中'; ProcessCount = '行程數'; ProcessIds = '行程 ID'
    HeartbeatFresh = '心跳正常'; RuntimeVersion = '執行版本'; LastScanAt = '最近掃描'
    StartupMode = '開機啟動模式'; StartupEntry = '開機啟動指令'; StartupOwned = '啟動項目屬於本外掛'
    StartupApproved = 'Windows 開機啟動核准'; SharedModeEnabled = '共用後端已啟用'
    SharedModeRequested = '已要求共用後端'; SharedEndpointConfigured = '已設定共用端點'
    DesktopLaunchMode = 'Codex 啟動方式'; SafeLauncher = '安全啟動指令碼'
    SharedServerState = '共用後端狀態'; SharedServerVerification = '共用後端驗證'
    SharedAppServerMemoryUsageMB = '共用後端記憶體（MB）'; SharedAppServerMemoryLimitMB = '共用後端記憶體上限（MB）'
    SharedAppServerMemoryGuardTriggered = '共用後端記憶體保護已觸發'; RetrySafetyWarning = '重試安全提醒'
    StatusCompatibility = '狀態檔相容性'; StatusCompatibilityMessage = '相容性說明'
    DataDirectoryExists = '資料夾存在'
}

function Get-ManagerLanguage {
    if ($Language) { return $Language }
    $saved = Read-JsonOrNull -Path (Join-Path $installDir 'ui-language.json')
    if ($saved -and $saved.PSObject.Properties['language'] -and [string]$saved.language -eq 'en') { return 'en' }
    return 'zh'
}

function Format-ManagerState {
    param([Parameter(Mandatory = $true)]$State, [string]$Lang = 'en')
    if ($Lang -ne 'zh') { return ($State | Format-List * | Out-String -Width 120) }
    $properties = @($State.PSObject.Properties)
    $labels = @($properties | ForEach-Object { if ($managerStateLabels.ContainsKey($_.Name)) { $managerStateLabels[$_.Name] } else { $_.Name } })
    # MingLiU draws CJK characters exactly two ASCII columns wide, so padding
    # by display width keeps the colons aligned.
    $displayWidth = { param([string]$Value) $count = 0; foreach ($char in $Value.ToCharArray()) { if ([int]$char -ge 0x2E80) { $count += 2 } else { $count += 1 } }; $count }
    $width = ($labels | ForEach-Object { & $displayWidth $_ } | Measure-Object -Maximum).Maximum
    $lines = for ($index = 0; $index -lt $properties.Count; $index++) {
        $value = $properties[$index].Value
        $shown = if ($value -is [bool]) { if ($value) { '是' } else { '否' } }
            elseif ($null -eq $value) { '' }
            elseif ($value -is [array]) { $value -join ', ' }
            else { [string]$value }
        $labels[$index] + (' ' * ($width + 2 - (& $displayWidth $labels[$index]))) + ': ' + $shown
    }
    return ($lines -join [Environment]::NewLine)
}

function Hide-ManagerConsoleWindow {
    # The .cmd entry point is intentionally simple and may be hosted by
    # Windows Terminal. Hide only that console after the Forms window is ready;
    # hiding the PowerShell process itself would also hide the manager dialog.
    try {
        if (-not ('CodexAutoRetry.NativeWindow' -as [type])) {
            Add-Type @'
using System;
using System.Runtime.InteropServices;
namespace CodexAutoRetry {
    public static class NativeWindow {
        [DllImport("kernel32.dll")]
        public static extern IntPtr GetConsoleWindow();

        [DllImport("user32.dll")]
        [return: MarshalAs(UnmanagedType.Bool)]
        public static extern bool ShowWindow(IntPtr window, int command);
    }
}
'@
        }
        $console = [CodexAutoRetry.NativeWindow]::GetConsoleWindow()
        if ($console -ne [IntPtr]::Zero) {
            [void][CodexAutoRetry.NativeWindow]::ShowWindow($console, 0)
        }
    }
    catch {
        # A host without a console or without the native window API can still
        # use the graphical manager; hiding the console is only cosmetic.
    }
}

function Show-Manager {
    Add-Type -AssemblyName System.Windows.Forms
    Add-Type -AssemblyName System.Drawing

    $ui = @{ Lang = Get-ManagerLanguage }
    $text = $managerText
    $form = New-Object System.Windows.Forms.Form
    $form.Text = $text.title[$ui.Lang]
    $form.StartPosition = 'CenterScreen'
    $form.Size = New-Object System.Drawing.Size(720, 600)
    $form.MinimumSize = New-Object System.Drawing.Size(620, 540)

    $title = New-Object System.Windows.Forms.Label
    $title.Text = $text.title[$ui.Lang]
    $title.Font = New-Object System.Drawing.Font('Segoe UI', 14, [System.Drawing.FontStyle]::Bold)
    $title.AutoSize = $true
    $title.Location = New-Object System.Drawing.Point(18, 15)
    $form.Controls.Add($title)

    $subtitle = New-Object System.Windows.Forms.Label
    $subtitle.Text = $text.subtitle[$ui.Lang]
    $subtitle.AutoSize = $true
    $subtitle.ForeColor = [System.Drawing.Color]::DimGray
    $subtitle.Location = New-Object System.Drawing.Point(20, 48)
    $form.Controls.Add($subtitle)

    $languageButton = New-Object System.Windows.Forms.Button
    $languageButton.Text = $text.language[$ui.Lang]
    $languageButton.Width = 90
    $languageButton.Height = 28
    $languageButton.Anchor = 'Top,Right'
    $languageButton.Location = New-Object System.Drawing.Point(596, 14)
    $form.Controls.Add($languageButton)

    $output = New-Object System.Windows.Forms.TextBox
    $output.Multiline = $true
    $output.ReadOnly = $true
    $output.ScrollBars = 'Vertical'
    $outputFont = @{
        en = New-Object System.Drawing.Font('Consolas', 10)
        zh = New-Object System.Drawing.Font('MingLiU', 11)
    }
    $output.Font = $outputFont[$ui.Lang]
    $output.Anchor = 'Top,Bottom,Left,Right'
    $output.Location = New-Object System.Drawing.Point(18, 78)
    $output.Size = New-Object System.Drawing.Size(668, 330)
    $form.Controls.Add($output)

    $buttons = New-Object System.Windows.Forms.FlowLayoutPanel
    $buttons.FlowDirection = 'LeftToRight'
    $buttons.WrapContents = $true
    $buttons.Anchor = 'Bottom,Left,Right'
    $buttons.Location = New-Object System.Drawing.Point(18, 420)
    $buttons.Size = New-Object System.Drawing.Size(668, 124)
    $buttons.Padding = New-Object System.Windows.Forms.Padding(0)
    $form.Controls.Add($buttons)

    $refresh = New-Object System.Windows.Forms.Button
    $refresh.Text = $text.refresh[$ui.Lang]
    $refresh.Tag = 'refresh'
    $refresh.Width = 142
    $refresh.Height = 30
    $buttons.Controls.Add($refresh)

    $definitions = @(
        @('launch-codex', $false),
        @('enable', $false),
        @('disable', $false),
        @('start', $false),
        @('stop', $false),
        @('safe-disable', $true),
        @('uninstall', $true),
        @('uninstall-remove', $true)
    )

    $refreshView = {
        try {
            $output.Text = Format-ManagerState -State (Get-ManagerState) -Lang $ui.Lang
        }
        catch {
            $output.Text = $_.Exception.Message
        }
    }.GetNewClosure()

    function Add-ManagerButton {
        param([string]$ButtonAction, [bool]$Danger, [scriptblock]$RefreshView, [hashtable]$Ui, [hashtable]$Text)
        $button = New-Object System.Windows.Forms.Button
        $button.Text = $Text[$ButtonAction][$Ui.Lang]
        $button.Tag = $ButtonAction
        $button.Width = 142
        $button.Height = 30
        if ($Danger) { $button.ForeColor = [System.Drawing.Color]::DarkRed }
        $handler = {
            try {
                if ($ButtonAction -eq 'uninstall-remove') {
                    $confirm = [System.Windows.Forms.MessageBox]::Show(
                        $Text.confirmFull[$Ui.Lang],
                        $Text.confirmFullTitle[$Ui.Lang],
                        [System.Windows.Forms.MessageBoxButtons]::YesNo,
                        [System.Windows.Forms.MessageBoxIcon]::Warning
                    )
                    if ($confirm -ne [System.Windows.Forms.DialogResult]::Yes) { return }
                    $script:RemoveData = $true
                    $script:NoPrompt = $true
                    $null = Invoke-ManagerAction -RequestedAction 'uninstall'
                    [System.Windows.Forms.MessageBox]::Show($Text.doneFull[$Ui.Lang], 'Codex Auto Retry') | Out-Null
                    $form.Close()
                    return
                }
                if ($ButtonAction -eq 'uninstall') {
                    $confirm = [System.Windows.Forms.MessageBox]::Show(
                        $Text.confirmKeep[$Ui.Lang],
                        $Text.confirmKeepTitle[$Ui.Lang],
                        [System.Windows.Forms.MessageBoxButtons]::YesNo,
                        [System.Windows.Forms.MessageBoxIcon]::Question
                    )
                    if ($confirm -ne [System.Windows.Forms.DialogResult]::Yes) { return }
                    $script:RemoveData = $false
                    $script:NoPrompt = $true
                    $null = Invoke-ManagerAction -RequestedAction 'uninstall'
                    [System.Windows.Forms.MessageBox]::Show($Text.doneKeep[$Ui.Lang], 'Codex Auto Retry') | Out-Null
                    $form.Close()
                    return
                }
                $null = Invoke-ManagerAction -RequestedAction $ButtonAction
                & $RefreshView
            }
            catch {
                [System.Windows.Forms.MessageBox]::Show($_.Exception.Message, $Text.failedTitle[$Ui.Lang], [System.Windows.Forms.MessageBoxButtons]::OK, [System.Windows.Forms.MessageBoxIcon]::Error) | Out-Null
                try { & $RefreshView } catch { $output.Text = $_.Exception.Message }
            }
        }.GetNewClosure()
        $button.Add_Click($handler)
        $buttons.Controls.Add($button)
    }

    foreach ($definition in $definitions) {
        Add-ManagerButton -ButtonAction $definition[0] -Danger ([bool]$definition[1]) -RefreshView $refreshView -Ui $ui -Text $text
    }
    $refresh.Add_Click({ & $refreshView }.GetNewClosure())
    $languageButton.Add_Click({
        $ui.Lang = if ($ui.Lang -eq 'zh') { 'en' } else { 'zh' }
        $form.Text = $text.title[$ui.Lang]
        $title.Text = $text.title[$ui.Lang]
        $subtitle.Text = $text.subtitle[$ui.Lang]
        $languageButton.Text = $text.language[$ui.Lang]
        $output.Font = $outputFont[$ui.Lang]
        foreach ($control in $buttons.Controls) {
            if ($text.ContainsKey([string]$control.Tag)) { $control.Text = $text[[string]$control.Tag][$ui.Lang] }
        }
        # Share the choice with the settings window; a failed save only
        # affects the next launch, never the running manager.
        try {
            if (Test-Path -LiteralPath $installDir -PathType Container) {
                [IO.File]::WriteAllText((Join-Path $installDir 'ui-language.json'), (@{ language = $ui.Lang } | ConvertTo-Json -Compress), [Text.UTF8Encoding]::new($false))
            }
        } catch { }
        & $refreshView
    }.GetNewClosure())
    $form.Add_Shown({
        Hide-ManagerConsoleWindow
        # A hidden script host can also suppress the first ShowWindow call.
        # Explicitly show this dialog, never the console or a Codex window.
        if ('CodexAutoRetry.NativeWindow' -as [type]) {
            [void][CodexAutoRetry.NativeWindow]::ShowWindow($form.Handle, 1)
        }
        & $refreshView
        $form.Activate()
    }.GetNewClosure())
    [void]$form.ShowDialog()
}

if ($Action -eq 'gui') {
    Show-Manager
    return
}

$result = Invoke-ManagerAction -RequestedAction $Action
if ($null -ne $result) { $result | Format-List * }
