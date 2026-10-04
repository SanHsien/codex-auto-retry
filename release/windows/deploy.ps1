[CmdletBinding()]
param(
    [string]$PackageRoot = '',
    [string]$UserProfileRoot = $env:USERPROFILE,
    [string]$LocalAppDataRoot = $env:LOCALAPPDATA,
    [string]$CodexCliPath = '',
    [switch]$DryRun,
    [switch]$SkipCodexCheck,
    [switch]$SkipPluginRegistration,
    [switch]$SkipRuntimeInstall,
    [switch]$EnableSharedAppServer,
    [switch]$WaitForCodexExit
)

$ErrorActionPreference = 'Stop'
Set-StrictMode -Version 2
. (Join-Path $PSScriptRoot 'common.ps1')
. (Join-Path $PSScriptRoot 'upgrade-runtime.ps1')
. (Join-Path $PSScriptRoot 'close-codex.ps1')

function Write-Step {
    param([string]$Message)
    Write-Host ('[Codex Auto Retry] ' + $Message)
}

function Test-CodexDesktopRunning {
    return (Get-InstallerDesktopState) -ne 'closed'
}

function Test-SharedBackendInUse {
    param([string]$RuntimePath)

    $configPath = Join-Path $RuntimePath 'config.json'
    $config = $null
    if (Test-Path -LiteralPath $configPath -PathType Leaf) {
        try { $config = Get-Content -Raw -Encoding UTF8 -LiteralPath $configPath | ConvertFrom-Json } catch { }
    }
    if ($config -and [bool]$config.shared_app_server_enabled) { return $true }

    $statePath = Join-Path $RuntimePath 'shared-server.json'
    if (Test-Path -LiteralPath $statePath -PathType Leaf) {
        try {
            $state = Get-Content -Raw -Encoding UTF8 -LiteralPath $statePath | ConvertFrom-Json
            $owned = [string]$state.owner -eq 'codex-auto-retry' -and
                [string]$state.endpoint -match '^ws://127\.0\.0\.1:\d+$' -and
                [int]$state.pid -gt 0 -and
                -not [string]::IsNullOrWhiteSpace([string]$state.executable)
            if (-not $owned) { return $true }
            $process = Get-CimInstance Win32_Process -Filter ('ProcessId = ' + [int]$state.pid) -ErrorAction Stop
            if ($null -eq $process) {
                # The ownership record can outlive a process after an
                # interrupted stop. Require Desktop to be closed before the
                # installer repairs that ambiguous state.
                return $true
            }
            # A live owned app-server is in use even when the user endpoint was
            # already removed. Replacing the watchdog must not kill it while
            # Desktop may still have an inherited connection.
            return $true
        }
        catch {
            # An unreadable or ambiguous ownership record is not proof that the
            # endpoint is safe to mutate. Require Desktop to be closed so the
            # next run can repair it deliberately.
            return $true
        }
    }

    $backupPath = Join-Path $RuntimePath 'environment-backup.json'
    if (-not (Test-Path -LiteralPath $backupPath -PathType Leaf)) { return $false }
    try {
        $backup = Get-Content -Raw -Encoding UTF8 -LiteralPath $backupPath | ConvertFrom-Json
        if ([int]$backup.schema_version -ne 1 -or [string]$backup.name -ne 'CODEX_APP_SERVER_WS_URL' -or
            [string]$backup.installed_value -notmatch '^ws://127\.0\.0\.1:\d+$') { return $true }
        $endpoint = [Environment]::GetEnvironmentVariable('CODEX_APP_SERVER_WS_URL', 'User')
        return [string]::Equals($endpoint, [string]$backup.installed_value, [System.StringComparison]::OrdinalIgnoreCase)
    }
    catch {
        return $true
    }
}

function Set-ObjectProperty {
    param($Object, [string]$Name, $Value)
    if ($null -ne $Object.PSObject.Properties[$Name]) {
        $Object.$Name = $Value
    }
    else {
        $Object | Add-Member -NotePropertyName $Name -NotePropertyValue $Value
    }
}

function Read-ReleaseManifest {
    param([string]$Root)

    $path = Join-Path $Root 'release-manifest.json'
    if (-not (Test-Path -LiteralPath $path -PathType Leaf)) {
        # release\windows in the source tree is only the installer template.
        throw '缺少 release-manifest.json。這個資料夾是原始碼裡的安裝範本，不是打包好的發佈檔。請下載 Windows x64 發佈檔，或執行 scripts\build-release.ps1 後用產生的壓縮檔安裝。 / release-manifest.json is missing. This folder is the installer template from the source tree, not a packaged release. Download the Windows x64 release ZIP, or run scripts\build-release.ps1 and install from the archive it creates.'
    }
    $manifest = Read-JsonDocument -Path $path
    if ($null -eq $manifest -or $manifest.product -ne 'Codex Auto Retry' -or
        $manifest.target -ne 'windows-x64') {
        throw '這個資料夾不是有效的 Codex Auto Retry Windows x64 發佈檔。 / This folder is not a valid Codex Auto Retry Windows x64 release.'
    }
    return $manifest
}

function Test-ReleaseIntegrity {
    param([string]$Root)

    $sumsPath = Join-Path $Root 'SHA256SUMS.txt'
    if (-not (Test-Path -LiteralPath $sumsPath -PathType Leaf)) {
        throw '發佈檔缺少 SHA256SUMS.txt。 / SHA256SUMS.txt is missing from the release.'
    }
    $checked = 0
    foreach ($line in (Get-Content -LiteralPath $sumsPath -Encoding UTF8)) {
        if ([string]::IsNullOrWhiteSpace($line)) { continue }
        if ($line -notmatch '^([0-9A-Fa-f]{64})  (.+)$') {
            throw "雜湊清單有無效的一行：$line / Invalid checksum line: $line"
        }
        $expected = $matches[1].ToUpperInvariant()
        $relative = $matches[2].Replace('/', '\')
        $path = Resolve-SafeChildPath -BasePath $Root -ChildPath $relative
        if (-not (Test-Path -LiteralPath $path -PathType Leaf)) {
            throw "發佈檔缺少檔案：$relative / Release file is missing: $relative"
        }
        $actual = (Get-FileHash -LiteralPath $path -Algorithm SHA256).Hash.ToUpperInvariant()
        if ($actual -ne $expected) {
            throw "發佈檔的檔案沒有通過完整性檢查：$relative / Release file failed its integrity check: $relative"
        }
        $checked++
    }
    if ($checked -lt 8) { throw '發佈檔的雜湊清單不完整。 / The release checksum list is incomplete.' }
    return $checked
}

function Read-OrCreateMarketplace {
    param([string]$Path)

    $document = Read-JsonDocument -Path $Path
    if ($null -eq $document) {
        return [pscustomobject][ordered]@{
            name = 'personal'
            interface = [pscustomobject][ordered]@{ displayName = 'Personal' }
            plugins = @()
        }
    }
    if ($null -eq $document.PSObject.Properties['plugins']) {
        $document | Add-Member -NotePropertyName plugins -NotePropertyValue @()
    }
    if ($null -eq $document.plugins) { $document.plugins = @() }
    return $document
}

function Ensure-MarketplaceEntry {
    param($Document)

    $updated = New-Object System.Collections.Generic.List[object]
    $found = $false
    foreach ($entry in @($Document.plugins)) {
        if ($null -ne $entry -and [string]$entry.name -eq 'codex-auto-retry') {
            if ($found) { continue }
            $found = $true
            Set-ObjectProperty -Object $entry -Name 'source' -Value ([pscustomobject][ordered]@{
                source = 'local'
                path = './plugins/codex-auto-retry'
            })
            if ($null -eq $entry.PSObject.Properties['policy'] -or $null -eq $entry.policy) {
                Set-ObjectProperty -Object $entry -Name 'policy' -Value ([pscustomobject][ordered]@{
                    installation = 'AVAILABLE'
                    authentication = 'ON_INSTALL'
                })
            }
            else {
                if ($null -eq $entry.policy.PSObject.Properties['installation']) {
                    $entry.policy | Add-Member -NotePropertyName installation -NotePropertyValue 'AVAILABLE'
                }
                if ($null -eq $entry.policy.PSObject.Properties['authentication']) {
                    $entry.policy | Add-Member -NotePropertyName authentication -NotePropertyValue 'ON_INSTALL'
                }
            }
            if ($null -eq $entry.PSObject.Properties['category']) {
                $entry | Add-Member -NotePropertyName category -NotePropertyValue 'Productivity'
            }
            [void]$updated.Add($entry)
            continue
        }
        [void]$updated.Add($entry)
    }
    if (-not $found) {
        [void]$updated.Add([pscustomobject][ordered]@{
            name = 'codex-auto-retry'
            source = [pscustomobject][ordered]@{
                source = 'local'
                path = './plugins/codex-auto-retry'
            }
            policy = [pscustomobject][ordered]@{
                installation = 'AVAILABLE'
                authentication = 'ON_INSTALL'
            }
            category = 'Productivity'
        })
    }
    $Document.plugins = $updated.ToArray()
    return $Document
}

function Assert-ExistingPluginIsOurs {
    param([string]$Path)

    if (-not (Test-Path -LiteralPath $Path -PathType Container)) { return }
    $item = Get-Item -LiteralPath $Path -Force
    if (($item.Attributes -band [System.IO.FileAttributes]::ReparsePoint) -ne 0) {
        throw "外掛資料夾是連結，拒絕取代：$Path / Refusing to replace a linked plugin directory: $Path"
    }
    $manifest = Read-JsonDocument -Path (Join-Path $Path '.codex-plugin\plugin.json')
    if ($null -eq $manifest -or [string]$manifest.name -ne 'codex-auto-retry') {
        throw "目標資料夾裡不是 Codex Auto Retry：$Path / The existing target directory is not Codex Auto Retry: $Path"
    }
}

function Set-InstalledMcpLauncher {
    param([string]$PluginPath, [string]$RuntimePath)

    $configPath = Join-Path $PluginPath '.mcp.json'
    $config = Read-JsonDocument -Path $configPath
    if ($null -eq $config -or $null -eq $config.PSObject.Properties['mcpServers'] -or
        $null -eq $config.mcpServers.PSObject.Properties['codex-auto-retry']) {
        throw '外掛的 MCP 設定缺少 codex-auto-retry。 / The plugin MCP configuration is missing codex-auto-retry.'
    }

    $server = $config.mcpServers.'codex-auto-retry'
    $mcpPath = Join-Path $RuntimePath 'codex-auto-retry-mcp.exe'
    Set-ObjectProperty -Object $server -Name 'command' -Value $mcpPath
    Set-ObjectProperty -Object $server -Name 'args' -Value @('mcp')
    Write-JsonAtomic -Path $configPath -Value $config
}

function Install-Runtime {
    param([string]$PluginPath, [bool]$EnableSharedAppServer)

    $script = Join-Path $PluginPath 'scripts\install.ps1'
    $arguments = @('-NoLogo', '-NoProfile', '-NonInteractive', '-ExecutionPolicy', 'Bypass', '-File', $script)
    if ($EnableSharedAppServer) { $arguments += '-EnableSharedAppServer' }
    $result = Invoke-CodexCli -Path (Join-Path $env:WINDIR 'System32\WindowsPowerShell\v1.0\powershell.exe') -Arguments $arguments -TimeoutMilliseconds 180000
    if ($result.ExitCode -ne 0) {
        throw "背景服務安裝失敗（exit=$($result.ExitCode)，category=$(Get-ReleaseCommandFailure $result)）。 / The watchdog installer failed (exit=$($result.ExitCode), category=$(Get-ReleaseCommandFailure $result))."
    }
    return $result.Output
}

function Stop-RuntimeForUpgrade {
    param([string]$RuntimePath)

    $watchdog = Join-Path $RuntimePath 'codex-auto-retry.exe'
    $mcp = Join-Path $RuntimePath 'codex-auto-retry-mcp.exe'
    $stopSignal = Join-Path $RuntimePath 'stop.signal'
    $supervisorStop = Join-Path $RuntimePath 'supervisor.stop'
    $watchdogProcesses = @(Get-CimInstance Win32_Process -ErrorAction Stop |
        Where-Object { $_.ExecutablePath -and [string]::Equals($_.ExecutablePath, $watchdog, [System.StringComparison]::OrdinalIgnoreCase) })
    $wasRunning = $watchdogProcesses.Count -gt 0
    if ($wasRunning) {
        New-Item -ItemType Directory -Force -Path $RuntimePath | Out-Null
        New-Item -ItemType File -Force -Path $supervisorStop | Out-Null
        New-Item -ItemType File -Force -Path $stopSignal | Out-Null
        $deadline = (Get-Date).AddSeconds(12)
        do {
            Start-Sleep -Milliseconds 250
            $watchdogProcesses = @(Get-CimInstance Win32_Process -ErrorAction Stop |
                Where-Object { $_.ExecutablePath -and [string]::Equals($_.ExecutablePath, $watchdog, [System.StringComparison]::OrdinalIgnoreCase) })
        } while ($watchdogProcesses.Count -gt 0 -and (Get-Date) -lt $deadline)
        if ($watchdogProcesses.Count -gt 0) {
            throw '背景服務沒有正常停止，已取消升級，沒有取代任何檔案。 / The watchdog did not stop gracefully. Upgrade was cancelled without replacing files.'
        }
    }

    @(Get-CimInstance Win32_Process -ErrorAction Stop |
        Where-Object { $_.ExecutablePath -and [string]::Equals($_.ExecutablePath, $mcp, [System.StringComparison]::OrdinalIgnoreCase) }) |
        ForEach-Object { Stop-Process -Id $_.ProcessId -Force -ErrorAction SilentlyContinue }
    Remove-Item -LiteralPath $stopSignal -Force -ErrorAction SilentlyContinue
    Remove-Item -LiteralPath $supervisorStop -Force -ErrorAction SilentlyContinue
    return $wasRunning
}

function Test-SafeUpgradeTransactionRoot {
    param([string]$Path)
    if ([string]::IsNullOrWhiteSpace($Path)) { return $false }
    try {
        $tempRoot = (Get-FullPath ([System.IO.Path]::GetTempPath())).TrimEnd('\') + '\'
        $candidate = Get-FullPath $Path
        return $candidate.StartsWith($tempRoot, [System.StringComparison]::OrdinalIgnoreCase)
    }
    catch {
        return $false
    }
}

function Read-UpgradeJournal {
    param([string]$Path)
    if (-not (Test-Path -LiteralPath $Path -PathType Leaf)) { return $null }
    $journal = Read-JsonDocument -Path $Path
    if ($null -eq $journal -or [int]$journal.schema_version -ne 1 -or
        [string]::IsNullOrWhiteSpace([string]$journal.transaction_id) -or
        [string]::IsNullOrWhiteSpace([string]$journal.transaction_root)) {
        throw "升級紀錄檔無效，未做修改：$Path / The upgrade journal is invalid and was not modified: $Path"
    }
    if (-not (Test-SafeUpgradeTransactionRoot -Path ([string]$journal.transaction_root))) {
        throw '升級紀錄檔指向暫存交易區以外的位置。 / The upgrade journal points outside the temporary transaction area.'
    }
    return $journal
}

function Restore-IncompleteUpgrade {
    param(
        [Parameter(Mandatory = $true)]$Journal,
        [Parameter(Mandatory = $true)][string]$PluginTarget,
        [Parameter(Mandatory = $true)][string]$MarketplacePath,
        [Parameter(Mandatory = $true)][string]$JournalPath,
        [Parameter(Mandatory = $true)][string]$RuntimePath,
        [string]$Cli = '',
        [string]$PluginId = ''
    )

    $transactionRoot = Get-FullPath ([string]$Journal.transaction_root)
    if (-not (Test-SafeUpgradeTransactionRoot $transactionRoot)) { throw '交易資料夾不安全。 / Unsafe transaction directory.' }
    $pluginBackup = Get-FullPath (Join-Path $transactionRoot 'plugin-backup')
    $marketplaceBackup = Get-FullPath (Join-Path $transactionRoot 'marketplace.json')
    $pluginExisted = [bool]$Journal.plugin_existed
    $marketplaceExisted = [bool]$Journal.marketplace_existed
    if ($pluginExisted -and -not (Test-Path -LiteralPath $pluginBackup -PathType Container)) {
        throw '未完成的升級缺少外掛備份，拒絕用猜測的方式回復。 / The incomplete upgrade is missing its plugin backup; refusing a guessed rollback.'
    }
    if ($marketplaceExisted -and -not (Test-Path -LiteralPath $marketplaceBackup -PathType Leaf)) {
        throw '未完成的升級缺少外掛清單備份，拒絕用猜測的方式回復。 / The incomplete upgrade is missing its marketplace backup; refusing a guessed rollback.'
    }
    Assert-UpgradePlainPath $transactionRoot
    Assert-UpgradePlainPath $PluginTarget
    Assert-UpgradePlainPath $MarketplacePath
    if ($Journal.PSObject.Properties['runtime_snapshot'] -and $Journal.runtime_snapshot) {
        Restore-UpgradeRuntime -RuntimePath $RuntimePath -TransactionRoot $transactionRoot
    }

    if (Test-Path -LiteralPath $PluginTarget -PathType Container) {
        Assert-ExistingPluginIsOurs -Path $PluginTarget
        Remove-Item -LiteralPath $PluginTarget -Recurse -Force
    }
    if ($pluginExisted) {
        New-Item -ItemType Directory -Force -Path (Split-Path -Parent $PluginTarget) | Out-Null
        Copy-DirectoryContents -Source $pluginBackup -Destination $PluginTarget
    }
    if ($marketplaceExisted) {
        Copy-Item -LiteralPath $marketplaceBackup -Destination $MarketplacePath -Force
    }
    else {
        Remove-Item -LiteralPath $MarketplacePath -Force -ErrorAction SilentlyContinue
    }
    if ($Journal.PSObject.Properties['registration_attempted'] -and $Journal.registration_attempted) {
        if (-not $Cli -or -not $PluginId) { throw '回復外掛註冊需要已確認的 Codex 命令列工具。 / Plugin registration rollback needs a verified CLI.' }
        $action = if ($pluginExisted) { 'add' } else { 'remove' }
        $restored = Invoke-CodexCli -Path $Cli -Arguments @('plugin', $action, $PluginId, '--json')
        if ($restored.ExitCode -ne 0) {
            throw "回復外掛註冊失敗（exit=$($restored.ExitCode)，category=$(Get-ReleaseCommandFailure $restored)）。 / Plugin registration rollback failed (exit=$($restored.ExitCode), category=$(Get-ReleaseCommandFailure $restored))."
        }
        $document = Get-VerifiedPluginList -Cli $Cli -PluginId $PluginId
        $entry = @($document.installed | Where-Object pluginId -eq $PluginId)
        if ($pluginExisted) {
            $oldManifest = Read-JsonDocument (Join-Path $PluginTarget '.codex-plugin\plugin.json')
            if ($entry.Count -ne 1 -or -not $entry[0].installed -or -not $entry[0].enabled -or
                $entry[0].version -ne $oldManifest.version) { throw '回復外掛註冊後驗證失敗。 / Plugin registration rollback verification failed.' }
        } elseif (@($entry | Where-Object installed).Count -gt 0) { throw '新的外掛註冊沒有撤除。 / New plugin registration was not retired.' }
    }
    # Persist completion before removing backups so interruption during cleanup
    # cannot leave an apparently unfinished transaction with no recovery files.
    $Journal.phase = 'rolled_back'
    Write-JsonAtomic -Path $JournalPath -Value $Journal
    Remove-Item -LiteralPath $JournalPath -Force
    Remove-Item -LiteralPath $transactionRoot -Recurse -Force -ErrorAction SilentlyContinue
}

function Get-VerifiedPluginList {
    param([string]$Cli, [string]$PluginId, [switch]$AllMarketplaces)
    $marketplaceName = ($PluginId -split '@', 2)[1]
    $arguments = @('plugin', 'list', '--json')
    if (-not $AllMarketplaces) { $arguments = @('plugin', 'list', '--marketplace', $marketplaceName, '--json') }
    for ($attempt = 1; $attempt -le 2; $attempt++) {
        $listing = Invoke-CodexCli -Path $Cli -Arguments $arguments -TimeoutMilliseconds 30000
        if ($listing.ExitCode -eq 0) {
            try { $document = $listing.Output | ConvertFrom-Json }
            catch { throw 'Codex 外掛驗證失敗（category=invalid_json）。 / Codex plugin verification failed (category=invalid_json).' }
            if ($null -eq $document -or -not $document.PSObject.Properties['installed']) {
                throw 'Codex 外掛驗證失敗（category=invalid_schema）。 / Codex plugin verification failed (category=invalid_schema).'
            }
            return $document
        }
        $category = Get-ReleaseCommandFailure $listing
        if ($attempt -eq 1 -and $category -in @('timeout', 'connection_failure')) {
            Start-Sleep -Milliseconds 500
            continue
        }
        if ($AllMarketplaces) {
            # Listing every marketplace fails when any unrelated one has a
            # missing or invalid source. Only our own marketplace matters.
            return Get-VerifiedPluginList -Cli $Cli -PluginId $PluginId
        }
        throw "Codex 外掛驗證失敗（exit=$($listing.ExitCode)，category=$category），沒有記錄任何憑證或原始輸出。請在終端機執行 codex plugin list 查看原因。 / Codex plugin verification failed (exit=$($listing.ExitCode), category=$category). No credentials or raw command output were logged. Run 'codex plugin list' in a terminal to see the cause."
    }
}

function Verify-Installation {
    param(
        [string]$PluginPath,
        [string]$RuntimePath,
        [string]$Cli,
        [string]$PluginId,
        [string]$ExpectedBaseVersion,
        [bool]$VerifyPlugin,
        [bool]$VerifyRuntime,
        [bool]$ExpectedSharedAppServer
    )

    $pluginManifest = Read-JsonDocument -Path (Join-Path $PluginPath '.codex-plugin\plugin.json')
    if ($null -eq $pluginManifest -or [string]$pluginManifest.name -ne 'codex-auto-retry') {
        throw '無法確認已安裝外掛的來源。 / The installed plugin source could not be verified.'
    }

    $mcpConfig = Read-JsonDocument -Path (Join-Path $PluginPath '.mcp.json')
    $mcpServer = if ($null -eq $mcpConfig -or $null -eq $mcpConfig.PSObject.Properties['mcpServers'] -or
        $null -eq $mcpConfig.mcpServers.PSObject.Properties['codex-auto-retry']) {
        $null
    }
    else {
        $mcpConfig.mcpServers.'codex-auto-retry'
    }
    $expectedMcpPath = Join-Path $RuntimePath 'codex-auto-retry-mcp.exe'
    $mcpArgs = @()
    if ($null -ne $mcpServer -and $null -ne $mcpServer.PSObject.Properties['args']) {
        $mcpArgs = @($mcpServer.args)
    }
    if ($null -eq $mcpServer -or
        -not [string]::Equals([string]$mcpServer.command, $expectedMcpPath, [System.StringComparison]::OrdinalIgnoreCase) -or
        $mcpArgs.Count -ne 1 -or [string]$mcpArgs[0] -ne 'mcp') {
        throw '已安裝的外掛沒有使用直接啟動的背景 MCP 程式。 / The installed plugin does not use the direct background MCP launcher.'
    }

    if ($VerifyPlugin) {
        $listDocument = Get-VerifiedPluginList -Cli $Cli -PluginId $PluginId
        $matches = @($listDocument.installed | Where-Object { $_.pluginId -eq $PluginId -and $_.installed -and $_.enabled })
        if ($matches.Count -ne 1) { throw "Codex 回報的 $PluginId 啟用安裝不是剛好一個。 / Codex did not report exactly one enabled installation of $PluginId." }
        if ([string]$matches[0].version -ne [string]$pluginManifest.version) {
            throw 'Codex 外掛驗證失敗（category=version_mismatch）。 / Codex plugin verification failed (category=version_mismatch).'
        }
    }

    if ($VerifyRuntime) {
        $watchdog = Join-Path $RuntimePath 'codex-auto-retry.exe'
        $mcp = Join-Path $RuntimePath 'codex-auto-retry-mcp.exe'
        Assert-X64PeBinary -Path $watchdog
        Assert-X64PeBinary -Path $mcp

        $status = Read-JsonDocument -Path (Join-Path $RuntimePath 'status.json')
        if ($null -eq $status -or -not $status.running -or [string]$status.version -ne $ExpectedBaseVersion) {
            throw '背景服務沒有回報預期的執行心跳。 / The watchdog did not publish the expected running heartbeat.'
        }
        $process = Get-CimInstance Win32_Process -Filter ("ProcessId = " + [int]$status.pid) -ErrorAction SilentlyContinue
        if ($null -eq $process -or -not $process.ExecutablePath -or
            -not [string]::Equals($process.ExecutablePath, $watchdog, [System.StringComparison]::OrdinalIgnoreCase)) {
            throw '背景服務的心跳對不上正在執行的已安裝程式。 / The watchdog heartbeat does not match a running installed process.'
        }
        if ($null -eq $status.PSObject.Properties['desktop_launch_mode'] -or $status.desktop_launch_mode -ne 'process_scoped') {
            throw '已安裝的背景程式不支援只對單一行程的 Desktop 路由。 / The installed worker does not support process-scoped Desktop routing.'
        }
        $config = Read-JsonDocument -Path (Join-Path $RuntimePath 'config.json')
        if ($null -eq $config -or [bool]$config.shared_app_server_enabled -ne $ExpectedSharedAppServer) {
            throw '已安裝的共用後端模式和要求的設定不一致。 / The installed shared app-server mode does not match the requested setting.'
        }
        $runProperty = Get-ItemProperty -Path 'HKCU:\Software\Microsoft\Windows\CurrentVersion\Run' -Name 'CodexAutoRetry' -ErrorAction SilentlyContinue
        $runValue = if ($null -eq $runProperty) { $null } else { $runProperty.CodexAutoRetry }
        if ([string]::IsNullOrWhiteSpace([string]$runValue) -or
            $runValue -notmatch [regex]::Escape($watchdog) -or
            $runValue -notmatch '(?i)\bsupervise\b') {
            throw '目前使用者的開機啟動項目沒有以監護模式註冊。 / The current-user startup entry was not registered in supervised mode.'
        }
        $approvalScript = Join-Path $PluginPath 'scripts\startup-approval.ps1'
        if (-not (Test-Path -LiteralPath $approvalScript -PathType Leaf)) {
            throw '已安裝的外掛缺少 StartupApproved 驗證程式。 / The installed plugin is missing its StartupApproved verification helper.'
        }
        . $approvalScript
        if ((Get-CodexAutoRetryStartupApproval -RunName 'CodexAutoRetry').Status -ne 'enabled') {
            throw '目前使用者的開機啟動核准沒有啟用。 / The current-user startup approval was not enabled.'
        }
    }
}

if ($env:OS -ne 'Windows_NT') { throw '這個發佈檔只支援 Windows。 / This release supports Windows only.' }
if (-not [Environment]::Is64BitOperatingSystem) { throw '這個發佈檔需要 64 位元 Windows。 / This release requires 64-bit Windows.' }
if ([string]::IsNullOrWhiteSpace($PackageRoot)) { $PackageRoot = $PSScriptRoot }

$packageRootPath = Get-FullPath $PackageRoot
$profileRootPath = Get-FullPath $UserProfileRoot
$localAppDataPath = Get-FullPath $LocalAppDataRoot
$manifest = Read-ReleaseManifest -Root $packageRootPath
$payloadRelative = [string]$manifest.payloadPath
$payloadRoot = Resolve-SafeChildPath -BasePath $packageRootPath -ChildPath $payloadRelative.Replace('/', '\')
. (Join-Path $payloadRoot 'scripts\startup-approval.ps1')
$pluginManifestPath = Join-Path $payloadRoot '.codex-plugin\plugin.json'
$pluginManifest = Read-JsonDocument -Path $pluginManifestPath
if ($null -eq $pluginManifest -or [string]$pluginManifest.name -ne 'codex-auto-retry' -or
    [string]$pluginManifest.version -ne [string]$manifest.pluginVersion) {
    throw '安裝包內的外掛資訊與發佈資訊不一致。 / The payload plugin manifest does not match the release manifest.'
}
Assert-X64PeBinary -Path (Join-Path $payloadRoot 'scripts\bin\codex-auto-retry.exe')
Assert-X64PeBinary -Path (Join-Path $payloadRoot 'scripts\bin\codex-auto-retry-mcp.exe')

Write-Step '正在驗證發佈檔… / Verifying release files...'
$checkedFiles = Test-ReleaseIntegrity -Root $packageRootPath

$cli = $null
if (-not $SkipCodexCheck) {
    Write-Step '正在尋找 Codex App 命令列工具… / Locating Codex App command line support...'
    $cli = Find-CodexCli -PreferredPath $CodexCliPath -LocalAppDataRoot $localAppDataPath
}
elseif (-not [string]::IsNullOrWhiteSpace($CodexCliPath)) {
    $cli = Get-FullPath $CodexCliPath
}

$pluginParent = Resolve-SafeChildPath -BasePath $profileRootPath -ChildPath 'plugins'
$pluginTarget = Resolve-SafeChildPath -BasePath $pluginParent -ChildPath 'codex-auto-retry'
$marketplacePath = Resolve-SafeChildPath -BasePath $profileRootPath -ChildPath '.agents\plugins\marketplace.json'
$runtimePath = Resolve-SafeChildPath -BasePath $localAppDataPath -ChildPath 'CodexAutoRetry'
$marketplace = Ensure-MarketplaceEntry -Document (Read-OrCreateMarketplace -Path $marketplacePath)
$marketplaceName = Get-MarketplaceName -Document $marketplace
if ($marketplaceName -notmatch '^[A-Za-z0-9._-]+$') {
    throw "個人外掛清單的名稱不受支援：$marketplaceName / The personal marketplace has an unsupported name: $marketplaceName"
}
$pluginId = 'codex-auto-retry@' + $marketplaceName
$upgradeJournalPath = Join-Path $runtimePath 'upgrade-journal.json'

if ($DryRun) {
    Write-Step '試跑完成，沒有變更任何檔案或設定。 / Dry run completed. No files or settings were changed.'
    [pscustomobject]@{
        Ready = $true
        PackageVersion = [string]$manifest.packageVersion
        PluginVersion = [string]$manifest.pluginVersion
        FilesVerified = $checkedFiles
        PluginTarget = $pluginTarget
        RuntimeTarget = $runtimePath
        Marketplace = $marketplacePath
        CodexCli = $cli
    }
    return
}

# Interactive one-click installs wait before creating a lock or touching any
# installed files. Automation keeps the existing immediate, fail-closed gate.
if ($WaitForCodexExit -and -not (Wait-CodexInstallerExit)) {
    Write-Step '已取消安裝，外掛與執行環境都沒有變更。 / Installation cancelled. No plugin or runtime changes were made.'
    exit 2
}

New-Item -ItemType Directory -Force -Path $runtimePath | Out-Null
$upgradeLock = $null
try {
    $upgradeLock = [System.IO.File]::Open(
        (Join-Path $runtimePath '.upgrade.lock'),
        [System.IO.FileMode]::OpenOrCreate,
        [System.IO.FileAccess]::ReadWrite,
        [System.IO.FileShare]::None
    )
}
catch {
    throw '另一個 Codex Auto Retry 升級或修復正在進行。 / Another Codex Auto Retry upgrade or repair is already in progress.'
}

try {
    $unfinished = Read-UpgradeJournal -Path $upgradeJournalPath
    if ($unfinished) {
        $phase = [string]$unfinished.phase
        if ($phase -notin @('committed', 'rolled_back') -and (Test-CodexDesktopRunning)) {
            throw '有中斷的升級等待回復，請完全關閉 Codex 後再執行修復。 / An interrupted upgrade is waiting for recovery. Close Codex completely before running the repair again.'
        }
        if ($phase -in @('committed', 'rolled_back')) {
            Assert-UpgradePlainPath ([string]$unfinished.transaction_root)
            Remove-Item -LiteralPath ([string]$unfinished.transaction_root) -Recurse -Force -ErrorAction SilentlyContinue
            Remove-Item -LiteralPath $upgradeJournalPath -Force -ErrorAction SilentlyContinue
        }
        else {
            Write-Step "正在回復中斷的升級 $([string]$unfinished.transaction_id)… / Recovering interrupted upgrade transaction $([string]$unfinished.transaction_id)..."
            [void](Stop-RuntimeForUpgrade -RuntimePath $runtimePath)
            . (Join-Path $payloadRoot 'scripts\environment.ps1')
            Disable-CodexAutoRetryLegacyRouting -DataDir $runtimePath
            $null = Stop-CodexAutoRetrySharedServerIfUnused -DataDir $runtimePath
            Restore-IncompleteUpgrade -Journal $unfinished -PluginTarget $pluginTarget -MarketplacePath $marketplacePath -JournalPath $upgradeJournalPath -RuntimePath $runtimePath -Cli $cli -PluginId $pluginId
        }
    }

    if (Test-CodexDesktopRunning) {
        throw '安裝或升級前請完全關閉 Codex（包括使用官方後端的工作階段），外掛與執行環境都沒有變更。 / Close Codex completely before installing or upgrading, including official-backend sessions. No plugin or runtime changes were made.'
    }
}
catch {
    if ($upgradeLock) { $upgradeLock.Dispose(); $upgradeLock = $null }
    throw
}

try {
    if (-not $SkipRuntimeInstall) {
        $pathSafety = Join-Path $payloadRoot 'scripts\path-safety.ps1'
        if (-not (Test-Path -LiteralPath $pathSafety -PathType Leaf)) {
            throw '安裝包缺少執行路徑安全檢查程式。 / The payload is missing the runtime path-safety helper.'
        }
        . $pathSafety
        [void](Assert-CodexAutoRetryHostPath -Path $runtimePath)
    }
    # A listing failure must be discovered before files, registration or the worker
    # are replaced. Fresh installs have no personal marketplace to filter yet.
    if (-not $SkipPluginRegistration) {
        Write-Step '變更前先確認能列出外掛… / Checking plugin listing support before making changes...'
        $null = Get-VerifiedPluginList -Cli $cli -PluginId $pluginId -AllMarketplaces:(-not (Test-Path -LiteralPath $marketplacePath))
    }
} catch {
    if ($upgradeLock) { $upgradeLock.Dispose(); $upgradeLock = $null }
    throw
}

Get-ChildItem -LiteralPath $packageRootPath -File -Recurse -Force -ErrorAction SilentlyContinue |
    Unblock-File -ErrorAction SilentlyContinue

$transactionRoot = Join-Path ([System.IO.Path]::GetTempPath()) ('codex-auto-retry-install-' + [guid]::NewGuid().ToString('N'))
$pluginBackup = Join-Path $transactionRoot 'plugin-backup'
$marketplaceBackup = Join-Path $transactionRoot 'marketplace.json'
$pluginExisted = Test-Path -LiteralPath $pluginTarget -PathType Container
$marketplaceExisted = Test-Path -LiteralPath $marketplacePath -PathType Leaf
$success = $false
$journalCleared = $false
$journal = $null

$oldUserProfile = $env:USERPROFILE
$oldHome = $env:HOME
$oldLocalAppData = $env:LOCALAPPDATA
try {
    New-Item -ItemType Directory -Force -Path $transactionRoot | Out-Null
    Assert-ExistingPluginIsOurs -Path $pluginTarget
    if ($pluginExisted) {
        Copy-DirectoryContents -Source $pluginTarget -Destination $pluginBackup
    }
    if ($marketplaceExisted) {
        Copy-Item -LiteralPath $marketplacePath -Destination $marketplaceBackup -Force
    }
    if (-not $SkipRuntimeInstall) {
        Backup-UpgradeRuntime -RuntimePath $runtimePath -TransactionRoot $transactionRoot
    }
	$journal = [pscustomobject][ordered]@{
	    schema_version = 1
	    transaction_id = [guid]::NewGuid().ToString('N')
	    phase = 'prepared'
	    transaction_root = $transactionRoot
	    plugin_existed = $pluginExisted
	    marketplace_existed = $marketplaceExisted
	    runtime_snapshot = -not $SkipRuntimeInstall
	    registration_attempted = $false
	    created_at = [DateTime]::UtcNow.ToString('o')
	}
	Write-JsonAtomic -Path $upgradeJournalPath -Value $journal

    Write-Step '正在安裝外掛檔案… / Installing plugin files...'
    if ($pluginExisted -or -not $SkipRuntimeInstall) {
        $null = Stop-RuntimeForUpgrade -RuntimePath $runtimePath
    }
	$journal.phase = 'runtime_stopped'
	Write-JsonAtomic -Path $upgradeJournalPath -Value $journal
    New-Item -ItemType Directory -Force -Path $pluginParent | Out-Null
    if ($pluginExisted) {
        Remove-Item -LiteralPath $pluginTarget -Recurse -Force
    }
    Copy-DirectoryContents -Source $payloadRoot -Destination $pluginTarget
    $gitMetadataBackup = Join-Path $pluginBackup '.git'
    $gitMetadataTarget = Join-Path $pluginTarget '.git'
    if (Test-Path -LiteralPath $gitMetadataBackup -PathType Container) {
        Copy-DirectoryContents -Source $gitMetadataBackup -Destination $gitMetadataTarget
    }
    elseif (Test-Path -LiteralPath $gitMetadataBackup -PathType Leaf) {
        Copy-Item -LiteralPath $gitMetadataBackup -Destination $gitMetadataTarget -Force
    }
    Set-InstalledMcpLauncher -PluginPath $pluginTarget -RuntimePath $runtimePath
	$journal.phase = 'plugin_replaced'
	Write-JsonAtomic -Path $upgradeJournalPath -Value $journal
    Write-JsonAtomic -Path (Join-Path $pluginTarget '.codex-auto-retry-release.json') -Value ([pscustomobject][ordered]@{
        packageVersion = [string]$manifest.packageVersion
        pluginVersion = [string]$manifest.pluginVersion
        installedAt = [DateTime]::UtcNow.ToString('o')
    })

    Write-Step '正在註冊個人 Codex 外掛… / Registering the personal Codex plugin...'
    Write-JsonAtomic -Path $marketplacePath -Value $marketplace
	$journal.phase = 'plugin_registered'
	Write-JsonAtomic -Path $upgradeJournalPath -Value $journal

    $env:USERPROFILE = $profileRootPath
    $env:HOME = $profileRootPath
    $env:LOCALAPPDATA = $localAppDataPath

    if (-not $SkipPluginRegistration) {
        if ([string]::IsNullOrWhiteSpace([string]$cli)) { throw '註冊外掛需要 Codex 命令列工具。 / Codex CLI is required to register the plugin.' }
        $journal.registration_attempted = $true
        Write-JsonAtomic -Path $upgradeJournalPath -Value $journal
        $addResult = Invoke-CodexCli -Path $cli -Arguments @('plugin', 'add', $pluginId, '--json')
        if ($addResult.ExitCode -ne 0) {
            throw "Codex 外掛註冊失敗（exit=$($addResult.ExitCode)，category=$(Get-ReleaseCommandFailure $addResult)）。 / Codex plugin registration failed (exit=$($addResult.ExitCode), category=$(Get-ReleaseCommandFailure $addResult))."
        }
    }

    if (-not $SkipRuntimeInstall) {
        Write-Step '正在安裝並啟動背景服務… / Installing and starting the background watchdog...'
        [void](Install-Runtime -PluginPath $pluginTarget -EnableSharedAppServer:$EnableSharedAppServer)
		$journal.phase = 'runtime_installed'
		Write-JsonAtomic -Path $upgradeJournalPath -Value $journal
    }

    Write-Step '正在驗證安裝結果… / Verifying the completed installation...'
    $baseVersion = ([string]$manifest.pluginVersion -split '\+', 2)[0]
    Verify-Installation -PluginPath $pluginTarget -RuntimePath $runtimePath -Cli $cli -PluginId $pluginId -ExpectedBaseVersion $baseVersion -VerifyPlugin (-not $SkipPluginRegistration) -VerifyRuntime (-not $SkipRuntimeInstall) -ExpectedSharedAppServer:$EnableSharedAppServer
	$journal.phase = 'committed'
	Write-JsonAtomic -Path $upgradeJournalPath -Value $journal
    $success = $true

    Write-Step '安裝完成。 / Installation completed successfully.'
    if ($EnableSharedAppServer) {
        Write-Host '請重新啟動一次 Codex 以連上共用恢復服務，再開新任務載入管理面板。 / Restart Codex once so it connects to the shared recovery service, then open a new task to load the management panel.'
    }
    else {
        Write-Host '共用恢復維持關閉；開新任務即可載入管理面板。 / Shared recovery remains disabled; open a new task to load the management panel.'
    }
    [pscustomobject]@{
        Installed = $true
        Running = -not $SkipRuntimeInstall
        PackageVersion = [string]$manifest.packageVersion
        PluginVersion = [string]$manifest.pluginVersion
        PluginPath = $pluginTarget
        RuntimePath = $runtimePath
        Startup = if ($SkipRuntimeInstall) { 'not changed' } else { 'current user sign-in' }
        ExistingStatePreserved = $true
    }
}
catch {
    $failure = $_
    Write-Step '安裝失敗，正在還原先前的安裝… / Installation failed; restoring the previous installation...'
    try {
        if ($journal -and (Test-Path -LiteralPath $upgradeJournalPath)) {
            if (Test-CodexDesktopRunning) { throw '請先關閉 Codex 再完成回復。 / Close Codex before completing rollback.' }
            $null = Stop-RuntimeForUpgrade -RuntimePath $runtimePath
            # Never invoke an old installer's shared-mode publisher on rollback.
            . (Join-Path $payloadRoot 'scripts\environment.ps1')
            Disable-CodexAutoRetryLegacyRouting -DataDir $runtimePath
            $null = Stop-CodexAutoRetrySharedServerIfUnused -DataDir $runtimePath
            Restore-IncompleteUpgrade -Journal $journal -PluginTarget $pluginTarget -MarketplacePath $marketplacePath -JournalPath $upgradeJournalPath -RuntimePath $runtimePath -Cli $cli -PluginId $pluginId
            $journalCleared = $true
            Write-Step '已還原先前的檔案。自動重試已停止，設定與任務狀態都保留。 / Previous files restored. Automatic retry is stopped; settings and task state were preserved.'
        }
    }
    catch {
        Write-Warning '自動回復沒有完成，已保留備份與升級紀錄；請關閉 Codex 後重新執行安裝程式，既有的重試資料沒有刪除。 / Automatic rollback was incomplete. Backup and upgrade journal were retained; close Codex and rerun this installer. Existing retry data was not deleted.'
    }
    throw $failure
}
finally {
    $env:USERPROFILE = $oldUserProfile
    $env:HOME = $oldHome
    $env:LOCALAPPDATA = $oldLocalAppData
    if (($success -or $journalCleared) -and (Test-Path -LiteralPath $transactionRoot -PathType Container)) {
        $tempRoot = (Get-FullPath ([System.IO.Path]::GetTempPath())).TrimEnd('\') + '\'
        $transactionFull = Get-FullPath $transactionRoot
        if ($transactionFull.StartsWith($tempRoot, [System.StringComparison]::OrdinalIgnoreCase)) {
            Remove-Item -LiteralPath $transactionFull -Recurse -Force -ErrorAction SilentlyContinue
        }
    }
    if ($success) {
        Remove-Item -LiteralPath $upgradeJournalPath -Force -ErrorAction SilentlyContinue
    }
    if ($upgradeLock) {
        $upgradeLock.Dispose()
        $upgradeLock = $null
    }
    if (-not $success) {
        Write-Host '請看上方的錯誤訊息，沒有刻意刪除任何重試設定或任務狀態。 / See the error above. No retry configuration or task state was intentionally deleted.'
    }
}
