function Test-IsDefaultInstallLocation {
    param([Parameter(Mandatory = $true)][string]$InstallDir)
    if ([string]::IsNullOrWhiteSpace($env:LOCALAPPDATA)) { return $false }
    try {
        $expected = [System.IO.Path]::GetFullPath((Join-Path $env:LOCALAPPDATA 'CodexAutoRetry')).TrimEnd('\')
        $actual = [System.IO.Path]::GetFullPath($InstallDir).TrimEnd('\')
        return [string]::Equals($expected, $actual, [System.StringComparison]::OrdinalIgnoreCase)
    }
    catch {
        return $false
    }
}

function Get-CodexAutoRetryShortcutPaths {
    param(
        [Parameter(Mandatory = $true)][string]$InstallDir,
        [string]$UserProfileRoot = $env:USERPROFILE
    )
    $isDefault = Test-IsDefaultInstallLocation -InstallDir $InstallDir
    $programsRoot = if ($isDefault) {
        [Environment]::GetFolderPath('Programs')
    }
    else {
        Join-Path $UserProfileRoot 'Start Menu\Programs'
    }
    $desktopRoot = if ($isDefault) {
        [Environment]::GetFolderPath('Desktop')
    }
    else {
        Join-Path $UserProfileRoot 'Desktop'
    }
    $startMenuDir = Join-Path $programsRoot 'Codex Auto Retry'
    return [pscustomobject][ordered]@{
        IsDefaultLocation   = $isDefault
        StartMenuDirectory  = $startMenuDir
        ManagerShortcut     = Join-Path $startMenuDir 'Codex Auto Retry 啟動管理員.lnk'
        SettingsShortcut    = Join-Path $startMenuDir 'Codex Auto Retry 參數設定.lnk'
        DataFolderShortcut  = Join-Path $startMenuDir '開啟設定與日誌資料夾.lnk'
        DesktopShortcut     = Join-Path $desktopRoot 'Codex Auto Retry.lnk'
        UninstallRegKeyPath = 'HKCU:\Software\Microsoft\Windows\CurrentVersion\Uninstall\CodexAutoRetry'
    }
}

function New-CodexAutoRetryShortcutFile {
    param(
        [Parameter(Mandatory = $true)][string]$ShortcutPath,
        [Parameter(Mandatory = $true)][string]$TargetPath,
        [string]$Arguments = '',
        [string]$WorkingDirectory = '',
        [string]$IconLocation = '',
        [string]$Description = ''
    )
    $parent = Split-Path -Parent $ShortcutPath
    if (-not [string]::IsNullOrWhiteSpace($parent)) {
        New-Item -ItemType Directory -Force -Path $parent | Out-Null
    }
    $wsh = New-Object -ComObject WScript.Shell
    try {
        $shortcut = $wsh.CreateShortcut($ShortcutPath)
        $shortcut.TargetPath = $TargetPath
        if (-not [string]::IsNullOrWhiteSpace($Arguments)) {
            $shortcut.Arguments = $Arguments
        }
        if (-not [string]::IsNullOrWhiteSpace($WorkingDirectory)) {
            $shortcut.WorkingDirectory = $WorkingDirectory
        }
        if (-not [string]::IsNullOrWhiteSpace($IconLocation)) {
            $shortcut.IconLocation = $IconLocation
        }
        if (-not [string]::IsNullOrWhiteSpace($Description)) {
            $shortcut.Description = $Description
        }
        $shortcut.Save()
    }
    finally {
        [void][System.Runtime.InteropServices.Marshal]::ReleaseComObject($wsh)
    }
}

function Copy-CodexAutoRetrySupportFiles {
    param(
        [Parameter(Mandatory = $true)][string]$ScriptsRoot,
        [Parameter(Mandatory = $true)][string]$InstallDir,
        [string]$ReleaseRoot = ''
    )
    New-Item -ItemType Directory -Force -Path $InstallDir | Out-Null
    foreach ($scriptName in @(
        'startup-manager.ps1',
        'safe-disable.ps1',
        'uninstall.ps1',
        'environment.ps1',
        'path-safety.ps1',
        'startup-approval.ps1',
        'shared-server-status.ps1',
        'shell-integration.ps1'
    )) {
        $src = Join-Path $ScriptsRoot $scriptName
        if (Test-Path -LiteralPath $src -PathType Leaf) {
            $dst = Join-Path $InstallDir $scriptName
            if (-not [string]::Equals([System.IO.Path]::GetFullPath($src), [System.IO.Path]::GetFullPath($dst), [System.StringComparison]::OrdinalIgnoreCase)) {
                Copy-Item -LiteralPath $src -Destination $dst -Force
            }
        }
    }
    foreach ($iconCandidate in @(
        (Join-Path $ScriptsRoot 'source\ui\app-icon.ico'),
        (Join-Path $ScriptsRoot 'app-icon.ico'),
        $(if ($ReleaseRoot) { Join-Path $ReleaseRoot 'app-icon.ico' } else { '' })
    )) {
        if (-not [string]::IsNullOrWhiteSpace($iconCandidate) -and (Test-Path -LiteralPath $iconCandidate -PathType Leaf)) {
            $dstIcon = Join-Path $InstallDir 'app-icon.ico'
            if (-not [string]::Equals([System.IO.Path]::GetFullPath($iconCandidate), [System.IO.Path]::GetFullPath($dstIcon), [System.StringComparison]::OrdinalIgnoreCase)) {
                Copy-Item -LiteralPath $iconCandidate -Destination $dstIcon -Force
            }
            break
        }
    }
    if (-not [string]::IsNullOrWhiteSpace($ReleaseRoot)) {
        foreach ($relFile in @('uninstall-release.ps1', 'common.ps1')) {
            $srcRel = Join-Path $ReleaseRoot $relFile
            if (Test-Path -LiteralPath $srcRel -PathType Leaf) {
                $dstRel = Join-Path $InstallDir $relFile
                if (-not [string]::Equals([System.IO.Path]::GetFullPath($srcRel), [System.IO.Path]::GetFullPath($dstRel), [System.StringComparison]::OrdinalIgnoreCase)) {
                    Copy-Item -LiteralPath $srcRel -Destination $dstRel -Force
                }
            }
        }
    }
}

function Register-CodexAutoRetryShellIntegration {
    param(
        [Parameter(Mandatory = $true)][string]$InstallDir,
        [string]$UserProfileRoot = $env:USERPROFILE,
        [string]$Version = '1.2.0',
        [switch]$CreateStartMenuShortcuts = $true,
        [switch]$CreateDesktopShortcut,
        [switch]$RegisterUninstallEntry = $true
    )
    $paths = Get-CodexAutoRetryShortcutPaths -InstallDir $InstallDir -UserProfileRoot $UserProfileRoot
    $watchdogPath = Join-Path $InstallDir 'codex-auto-retry.exe'
    $settingsPath = Join-Path $InstallDir 'settings.ps1'
    $managerPath = Join-Path $InstallDir 'startup-manager.ps1'
    $iconPath = Join-Path $InstallDir 'app-icon.ico'
    $iconLocation = if (Test-Path -LiteralPath $iconPath -PathType Leaf) {
        $iconPath
    }
    elseif (Test-Path -LiteralPath $watchdogPath -PathType Leaf) {
        "$watchdogPath,0"
    }
    else {
        ''
    }
    $psExe = Join-Path $env:WINDIR 'System32\WindowsPowerShell\v1.0\powershell.exe'
    if (-not (Test-Path -LiteralPath $psExe -PathType Leaf)) { $psExe = 'powershell.exe' }

    if ($CreateStartMenuShortcuts -and -not [string]::IsNullOrWhiteSpace($paths.StartMenuDirectory)) {
        try {
            New-CodexAutoRetryShortcutFile `
                -ShortcutPath $paths.ManagerShortcut `
                -TargetPath $psExe `
                -Arguments ('-NoLogo -NoProfile -ExecutionPolicy Bypass -WindowStyle Hidden -File "{0}"' -f $managerPath) `
                -WorkingDirectory $InstallDir `
                -IconLocation $iconLocation `
                -Description 'Codex Auto Retry 啟動管理員 / Codex Auto Retry Startup Manager'

            New-CodexAutoRetryShortcutFile `
                -ShortcutPath $paths.SettingsShortcut `
                -TargetPath $psExe `
                -Arguments ('-NoLogo -NoProfile -ExecutionPolicy Bypass -WindowStyle Hidden -STA -File "{0}" -DataDir "{1}" -Executable "{2}"' -f $settingsPath, $InstallDir, $watchdogPath) `
                -WorkingDirectory $InstallDir `
                -IconLocation $iconLocation `
                -Description 'Codex Auto Retry 參數設定 / Codex Auto Retry Settings'

            New-CodexAutoRetryShortcutFile `
                -ShortcutPath $paths.DataFolderShortcut `
                -TargetPath 'explorer.exe' `
                -Arguments ('"{0}"' -f $InstallDir) `
                -WorkingDirectory $InstallDir `
                -IconLocation $iconLocation `
                -Description '開啟 Codex Auto Retry 設定與日誌資料夾 / Open Codex Auto Retry Config & Log Folder'
        }
        catch {
            Write-Warning "無法建立開始功能表捷徑：$($_.Exception.Message) / Could not create Start Menu shortcuts: $($_.Exception.Message)"
        }
    }

    if ($CreateDesktopShortcut -and -not [string]::IsNullOrWhiteSpace($paths.DesktopShortcut)) {
        try {
            New-CodexAutoRetryShortcutFile `
                -ShortcutPath $paths.DesktopShortcut `
                -TargetPath $psExe `
                -Arguments ('-NoLogo -NoProfile -ExecutionPolicy Bypass -WindowStyle Hidden -STA -File "{0}" -DataDir "{1}" -Executable "{2}"' -f $settingsPath, $InstallDir, $watchdogPath) `
                -WorkingDirectory $InstallDir `
                -IconLocation $iconLocation `
                -Description 'Codex Auto Retry 參數設定與佇列監控 / Codex Auto Retry Settings & Queue Monitor'
        }
        catch {
            Write-Warning "無法建立桌面捷徑：$($_.Exception.Message) / Could not create Desktop shortcut: $($_.Exception.Message)"
        }
    }

    if ($RegisterUninstallEntry -and $paths.IsDefaultLocation) {
        try {
            New-Item -Path $paths.UninstallRegKeyPath -Force | Out-Null
            $uninstallScript = Join-Path $InstallDir 'uninstall-release.ps1'
            $uninstallCmd = if (Test-Path -LiteralPath $uninstallScript -PathType Leaf) {
                '"{0}" -NoLogo -NoProfile -ExecutionPolicy Bypass -File "{1}"' -f $psExe, $uninstallScript
            }
            else {
                '"{0}" -NoLogo -NoProfile -ExecutionPolicy Bypass -File "{1}" -KeepData' -f $psExe, (Join-Path $InstallDir 'uninstall.ps1')
            }
            $modifyCmd = '"{0}" -NoLogo -NoProfile -ExecutionPolicy Bypass -WindowStyle Hidden -File "{1}"' -f $psExe, $managerPath
            Set-ItemProperty -Path $paths.UninstallRegKeyPath -Name 'DisplayName' -Value 'Codex Auto Retry' -Type String
            Set-ItemProperty -Path $paths.UninstallRegKeyPath -Name 'DisplayVersion' -Value $Version -Type String
            Set-ItemProperty -Path $paths.UninstallRegKeyPath -Name 'Publisher' -Value 'SanHsien' -Type String
            Set-ItemProperty -Path $paths.UninstallRegKeyPath -Name 'DisplayIcon' -Value $iconLocation -Type String
            Set-ItemProperty -Path $paths.UninstallRegKeyPath -Name 'InstallLocation' -Value $InstallDir -Type String
            Set-ItemProperty -Path $paths.UninstallRegKeyPath -Name 'InstallDate' -Value ((Get-Date).ToString('yyyyMMdd')) -Type String
            Set-ItemProperty -Path $paths.UninstallRegKeyPath -Name 'UninstallString' -Value $uninstallCmd -Type String
            Set-ItemProperty -Path $paths.UninstallRegKeyPath -Name 'QuietUninstallString' -Value ($uninstallCmd + ' -SkipCodexCheck') -Type String
            Set-ItemProperty -Path $paths.UninstallRegKeyPath -Name 'ModifyPath' -Value $modifyCmd -Type String
            Set-ItemProperty -Path $paths.UninstallRegKeyPath -Name 'NoRepair' -Value 1 -Type DWord
            Set-ItemProperty -Path $paths.UninstallRegKeyPath -Name 'NoModify' -Value 0 -Type DWord
            Set-ItemProperty -Path $paths.UninstallRegKeyPath -Name 'EstimatedSize' -Value 12288 -Type DWord
        }
        catch {
            Write-Warning "無法寫入 Windows 已安裝應用程式登錄項目：$($_.Exception.Message) / Could not write Windows Installed Apps registry entry: $($_.Exception.Message)"
        }
    }

    return [pscustomobject][ordered]@{
        StartMenuDirectory = $paths.StartMenuDirectory
        StartMenuCreated   = Test-Path -LiteralPath $paths.ManagerShortcut -PathType Leaf
        DesktopShortcut    = $paths.DesktopShortcut
        DesktopCreated     = Test-Path -LiteralPath $paths.DesktopShortcut -PathType Leaf
        UninstallEntry     = if ($paths.IsDefaultLocation) { $paths.UninstallRegKeyPath } else { $null }
    }
}

function Unregister-CodexAutoRetryShellIntegration {
    param(
        [Parameter(Mandatory = $true)][string]$InstallDir,
        [string]$UserProfileRoot = $env:USERPROFILE
    )
    $paths = Get-CodexAutoRetryShortcutPaths -InstallDir $InstallDir -UserProfileRoot $UserProfileRoot
    if (-not [string]::IsNullOrWhiteSpace($paths.StartMenuDirectory) -and (Test-Path -LiteralPath $paths.StartMenuDirectory -PathType Container)) {
        Remove-Item -LiteralPath $paths.StartMenuDirectory -Recurse -Force -ErrorAction SilentlyContinue
    }
    if (-not [string]::IsNullOrWhiteSpace($paths.DesktopShortcut) -and (Test-Path -LiteralPath $paths.DesktopShortcut -PathType Leaf)) {
        Remove-Item -LiteralPath $paths.DesktopShortcut -Force -ErrorAction SilentlyContinue
    }
    if ($paths.IsDefaultLocation -and (Test-Path -LiteralPath $paths.UninstallRegKeyPath)) {
        Remove-Item -LiteralPath $paths.UninstallRegKeyPath -Recurse -Force -ErrorAction SilentlyContinue
    }
}
