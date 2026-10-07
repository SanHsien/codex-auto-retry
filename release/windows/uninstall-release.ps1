[CmdletBinding()]
param(
    [string]$UserProfileRoot = $env:USERPROFILE,
    [string]$LocalAppDataRoot = $env:LOCALAPPDATA,
    [string]$CodexCliPath = '',
    [switch]$RemoveData,
    [switch]$DryRun,
    [switch]$SkipCodexCheck
)

$ErrorActionPreference = 'Stop'
Set-StrictMode -Version 2
. (Join-Path $PSScriptRoot 'common.ps1')

function Write-Step {
    param([string]$Message)
    Write-Host ('[Codex Auto Retry] ' + $Message)
}

function Test-PluginInstalled {
    param([string]$Cli, [string]$PluginId)

    $result = Invoke-CodexCli -Path $Cli -Arguments @('plugin', 'list', '--json')
    if ($result.ExitCode -ne 0) {
        # An unrelated marketplace with a missing source fails the full listing.
        $marketplaceName = ($PluginId -split '@', 2)[1]
        $result = Invoke-CodexCli -Path $Cli -Arguments @('plugin', 'list', '--marketplace', $marketplaceName, '--json')
    }
    if ($result.ExitCode -ne 0) { throw "Codex 無法讀取已安裝的外掛清單，請在終端機執行 codex plugin list 查看原因。 / Codex could not read the installed plugin list. Run 'codex plugin list' in a terminal to see the cause." }
    try { $document = $result.Output | ConvertFrom-Json } catch { throw 'Codex 回傳的外掛清單格式錯誤。 / Codex returned an invalid plugin list.' }
    return $null -ne (@($document.installed) | Where-Object { $_.pluginId -eq $PluginId } | Select-Object -First 1)
}

function Remove-MarketplaceEntry {
    param([string]$Path)

    $document = Read-JsonDocument -Path $Path
    if ($null -eq $document -or $null -eq $document.PSObject.Properties['plugins']) { return $false }
    $kept = @($document.plugins | Where-Object { $null -eq $_ -or [string]$_.name -ne 'codex-auto-retry' })
    if ($kept.Count -eq @($document.plugins).Count) { return $false }
    $document.plugins = $kept
    Write-JsonAtomic -Path $Path -Value $document
    return $true
}

if ($env:OS -ne 'Windows_NT') { throw '這個解除安裝程式只支援 Windows。 / This uninstaller supports Windows only.' }

$profileRootPath = Get-FullPath $UserProfileRoot
$localAppDataPath = Get-FullPath $LocalAppDataRoot
$pluginParent = Resolve-SafeChildPath -BasePath $profileRootPath -ChildPath 'plugins'
$pluginTarget = Resolve-SafeChildPath -BasePath $pluginParent -ChildPath 'codex-auto-retry'
$marketplacePath = Resolve-SafeChildPath -BasePath $profileRootPath -ChildPath '.agents\plugins\marketplace.json'
$runtimePath = Resolve-SafeChildPath -BasePath $localAppDataPath -ChildPath 'CodexAutoRetry'
$startupApprovalScript = Join-Path $pluginTarget 'scripts\startup-approval.ps1'
if (-not (Test-Path -LiteralPath $startupApprovalScript -PathType Leaf)) {
    $startupApprovalScript = Join-Path $PSScriptRoot 'payload\codex-auto-retry\scripts\startup-approval.ps1'
}
if (Test-Path -LiteralPath $startupApprovalScript -PathType Leaf) { . $startupApprovalScript }

$startupApprovedRunSubKey = 'Software\Microsoft\Windows\CurrentVersion\Explorer\StartupApproved\Run'

function Test-ReleaseStartupApprovalPresent {
    param([string]$RunName = 'CodexAutoRetry')

    $key = [Microsoft.Win32.Registry]::CurrentUser.OpenSubKey($startupApprovedRunSubKey, $false)
    if ($null -eq $key) { return $false }
    try {
        return $null -ne $key.GetValue($RunName, $null, [Microsoft.Win32.RegistryValueOptions]::DoNotExpandEnvironmentNames)
    }
    finally { $key.Close() }
}

function Remove-ReleaseStartupApproval {
    param([string]$RunName = 'CodexAutoRetry')

    # Always use the release script's own Registry API path. This keeps
    # uninstall compatible with old or partially extracted payloads whose
    # helper script is absent, and avoids silently orphaning the marker.
    $key = [Microsoft.Win32.Registry]::CurrentUser.OpenSubKey($startupApprovedRunSubKey, $true)
    if ($null -ne $key) {
        try { $key.DeleteValue($RunName, $false) }
        finally { $key.Close() }
    }
    if (Test-ReleaseStartupApprovalPresent -RunName $RunName) {
        throw "解除安裝後 $RunName 的開機啟動核准仍然存在。 / The startup approval for $RunName is still present after uninstall."
    }
}

$marketplace = Read-JsonDocument -Path $marketplacePath
$marketplaceName = if ($null -eq $marketplace) { 'personal' } else { Get-MarketplaceName -Document $marketplace }
if ($marketplaceName -notmatch '^[A-Za-z0-9._-]+$') {
    throw "個人外掛清單的名稱不受支援：$marketplaceName / The personal marketplace has an unsupported name: $marketplaceName"
}
$pluginId = 'codex-auto-retry@' + $marketplaceName

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
    return [string]::Equals($executable, (Join-Path $runtimePath 'codex-auto-retry.exe'), [System.StringComparison]::OrdinalIgnoreCase)
}

$startupProperty = Get-ItemProperty -Path 'HKCU:\Software\Microsoft\Windows\CurrentVersion\Run' -Name 'CodexAutoRetry' -ErrorAction SilentlyContinue
$startupValue = if ($null -eq $startupProperty) { '' } else { [string]$startupProperty.CodexAutoRetry }
if (-not [string]::IsNullOrWhiteSpace([string]$startupValue) -and -not (Test-OwnedStartupValue ([string]$startupValue))) {
    throw '目前使用者的開機啟動項目屬於其他程式，沒有移除。 / The current-user startup entry belongs to another command and was not removed.'
}

$cli = $null
if (-not $SkipCodexCheck) {
    Write-Step '正在尋找 Codex App 命令列工具… / Locating Codex App command line support...'
    $cli = Find-CodexCli -PreferredPath $CodexCliPath -LocalAppDataRoot $localAppDataPath
}

if ($DryRun) {
    Write-Step '試跑完成，沒有變更任何檔案或設定。 / Dry run completed. No files or settings were changed.'
    [pscustomobject]@{
        Ready = $true
        PluginId = $pluginId
        PluginPath = $pluginTarget
        RuntimePath = $runtimePath
        DataWillBeRemoved = [bool]$RemoveData
        CodexCli = $cli
    }
    return
}

$oldUserProfile = $env:USERPROFILE
$oldHome = $env:HOME
$oldLocalAppData = $env:LOCALAPPDATA
try {
    $env:USERPROFILE = $profileRootPath
    $env:HOME = $profileRootPath
    $env:LOCALAPPDATA = $localAppDataPath

    Write-Step '正在停止背景服務並移除開機啟動… / Stopping the background watchdog and removing startup...'
    $runtimeUninstaller = Join-Path $pluginTarget 'scripts\uninstall.ps1'
    if (-not (Test-Path -LiteralPath $runtimeUninstaller -PathType Leaf)) {
        $runtimeUninstaller = Join-Path $PSScriptRoot 'payload\codex-auto-retry\scripts\uninstall.ps1'
    }
    if (Test-Path -LiteralPath $runtimeUninstaller -PathType Leaf) {
        $arguments = @('-NoLogo', '-NoProfile', '-NonInteractive', '-ExecutionPolicy', 'Bypass', '-File', $runtimeUninstaller)
        if (-not $RemoveData) { $arguments += '-KeepData' }
        $output = (& powershell.exe @arguments 2>&1 | Out-String)
        if ($LASTEXITCODE -ne 0) { throw "背景服務解除安裝失敗，結束狀態 $LASTEXITCODE。 / The watchdog uninstaller failed with exit code $LASTEXITCODE." }
    }
    else {
        if ([string]::IsNullOrWhiteSpace([string]$startupValue) -or (Test-OwnedStartupValue ([string]$startupValue))) {
            Remove-ItemProperty -Path 'HKCU:\Software\Microsoft\Windows\CurrentVersion\Run' -Name 'CodexAutoRetry' -ErrorAction SilentlyContinue
        }
        if ($RemoveData -and (Test-Path -LiteralPath $runtimePath -PathType Container)) {
            $runtimeItem = Get-Item -LiteralPath $runtimePath -Force
            if (($runtimeItem.Attributes -band [System.IO.FileAttributes]::ReparsePoint) -ne 0) {
                throw "執行資料夾是連結，拒絕移除：$runtimePath / Refusing to remove a linked runtime directory: $runtimePath"
            }
            Remove-Item -LiteralPath $runtimePath -Recurse -Force
        }
    }

    Remove-ReleaseStartupApproval -RunName 'CodexAutoRetry'

    if (-not $SkipCodexCheck) {
        Write-Step '正在從 Codex 移除外掛… / Removing the plugin from Codex...'
        if (Test-PluginInstalled -Cli $cli -PluginId $pluginId) {
            $removeResult = Invoke-CodexCli -Path $cli -Arguments @('plugin', 'remove', $pluginId, '--json')
            if ($removeResult.ExitCode -ne 0) {
                throw "Codex 外掛移除失敗，結束狀態 $($removeResult.ExitCode)。 / Codex plugin removal failed with exit code $($removeResult.ExitCode)."
            }
        }
    }

    Write-Step '正在移除個人外掛清單項目與外掛檔案… / Removing the personal marketplace entry and plugin files...'
    [void](Remove-MarketplaceEntry -Path $marketplacePath)
    if (Test-Path -LiteralPath $pluginTarget -PathType Container) {
        $pluginItem = Get-Item -LiteralPath $pluginTarget -Force
        if (($pluginItem.Attributes -band [System.IO.FileAttributes]::ReparsePoint) -ne 0) {
            throw "外掛資料夾是連結，拒絕移除：$pluginTarget / Refusing to remove a linked plugin directory: $pluginTarget"
        }
        $manifest = Read-JsonDocument -Path (Join-Path $pluginTarget '.codex-plugin\plugin.json')
        if ($null -eq $manifest -or [string]$manifest.name -ne 'codex-auto-retry') {
            throw "目標外掛不是 Codex Auto Retry：$pluginTarget / The plugin target is not Codex Auto Retry: $pluginTarget"
        }
        Remove-Item -LiteralPath $pluginTarget -Recurse -Force
    }

    $runProperty = Get-ItemProperty -Path 'HKCU:\Software\Microsoft\Windows\CurrentVersion\Run' -Name 'CodexAutoRetry' -ErrorAction SilentlyContinue
    $runValue = if ($null -eq $runProperty) { $null } else { $runProperty.CodexAutoRetry }
    if (-not [string]::IsNullOrWhiteSpace([string]$runValue)) {
        throw '解除安裝後開機啟動項目仍然存在。 / The startup entry is still present after uninstall.'
    }
    if (Test-ReleaseStartupApprovalPresent -RunName 'CodexAutoRetry') {
        throw '解除安裝後 StartupApproved 值仍然存在。 / The StartupApproved value is still present after uninstall.'
    }
    if (-not $SkipCodexCheck -and (Test-PluginInstalled -Cli $cli -PluginId $pluginId)) {
        throw 'Codex 仍回報外掛已安裝。 / Codex still reports the plugin as installed.'
    }

    Write-Step '解除安裝完成。 / Uninstall completed successfully.'
    if ($RemoveData) {
        Write-Host '已移除執行設定、狀態與日誌。 / Runtime settings, state, and logs were removed.'
    }
    else {
        Write-Host "重試設定與狀態保留在：$runtimePath / Retry settings and state were preserved in: $runtimePath"
    }
    Write-Host '請開新的 Codex 任務重新整理外掛清單。 / Open a new Codex task to refresh the plugin list.'
}
finally {
    $env:USERPROFILE = $oldUserProfile
    $env:HOME = $oldHome
    $env:LOCALAPPDATA = $oldLocalAppData
}
