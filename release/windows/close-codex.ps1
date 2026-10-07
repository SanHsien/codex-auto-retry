function Get-InstallerDesktopState {
    try {
        $main = @(Get-CimInstance Win32_Process -OperationTimeoutSec 5 -ErrorAction Stop | Where-Object {
            ($_.Name -eq 'ChatGPT.exe' -or ($_.Name -eq 'Codex.exe' -and
                $_.ExecutablePath -match '\\app\\Codex\.exe$')) -and
            (-not $_.CommandLine -or $_.CommandLine -notmatch '(?:^|\s)--type=')
        })
        if ($main.Count -gt 0) { return 'running' }
        return 'closed'
    }
    catch {
        # Unknown process state is not permission to replace a live runtime.
        return 'unknown'
    }
}

function Show-CodexCloseNotice {
    param([string]$State, [int]$TimeoutSeconds)
    # The escaped JSON keeps the Chinese prompt valid in Windows PowerShell 5.1.
    $text = @'
{
  "running": "\u5075\u6e2c\u5230 Codex \u4ecd\u5728\u57f7\u884c\u3002\n\n\u8acb\u5148\u5132\u5b58\u5de5\u4f5c\uff0c\u7136\u5f8c\u5b8c\u5168\u7d50\u675f Codex\uff08\u5305\u62ec\u7cfb\u7d71\u5323\uff09\u3002\n\u7d50\u675f\u5f8c\u6309\u4e00\u4e0b\u300c\u91cd\u8a66\u300d\uff0c\u5b89\u88dd\u7a0b\u5f0f\u6703\u91cd\u65b0\u6aa2\u67e5\u4e26\u7e7c\u7e8c\u3002\n\n\u4e0d\u6703\u5f37\u5236\u95dc\u9589 Codex\u3002\u6309\u4e00\u4e0b\u300c\u53d6\u6d88\u300d\u53ef\u5b89\u5168\u7d50\u675f\u5b89\u88dd\u3002\n\nCodex is still running. Save your work and fully exit Codex, including its tray icon, then choose Retry to check again and continue. Codex is never force-closed; Cancel exits the installation safely.",
  "unknown": "\u7121\u6cd5\u78ba\u8a8d Codex \u662f\u5426\u5df2\u7d50\u675f\uff08\u884c\u7a0b\u67e5\u8a62\u5931\u6557\uff09\u3002\n\n\u70ba\u907f\u514d\u4e2d\u65b7\u4efb\u52d9\uff0c\u66ab\u4e0d\u66f4\u65b0\u3002\u8acb\u78ba\u8a8d\u5df2\u7d50\u675f Codex \u5f8c\u6309\u4e00\u4e0b\u300c\u91cd\u8a66\u300d\uff0c\u6216\u6309\u4e00\u4e0b\u300c\u53d6\u6d88\u300d\u7d50\u675f\u5b89\u88dd\u3002\n\nCannot confirm whether Codex has exited (process query failed). To avoid interrupting tasks, nothing is updated yet. After exiting Codex choose Retry, or choose Cancel to end the installation.",
  "timeout": "\n\n\u7b49\u5f85\u903e\u6642\u5c07\u81ea\u52d5\u53d6\u6d88\u672c\u6b21\u5b89\u88dd\u3002\nWaiting too long cancels this installation automatically."
}
'@ | ConvertFrom-Json
    $message = if ($State -eq 'running') { $text.running } else { $text.unknown }
    $shell = $null
    try {
        $shell = New-Object -ComObject WScript.Shell
        # Retry/Cancel, warning icon. 4 = Retry, 2 = Cancel, -1 = timeout.
        return [int]$shell.Popup(($message + $text.timeout), $TimeoutSeconds, 'Codex Auto Retry', 53)
    }
    catch {
        Write-Host '請完全關閉 Codex 後重新執行安裝程式，沒有關閉任何程式。 / Close Codex completely and rerun this installer. No process was closed.'
        return 2
    }
    finally {
        if ($shell -and [Runtime.InteropServices.Marshal]::IsComObject($shell)) {
            [void][Runtime.InteropServices.Marshal]::FinalReleaseComObject($shell)
        }
    }
}

function Wait-CodexInstallerExit {
    param(
        [ValidateRange(1, 600)][int]$TimeoutSeconds = 300,
        [ValidateRange(1, 50)][int]$MaxPrompts = 20
    )
    $watch = [Diagnostics.Stopwatch]::StartNew()
    try {
        for ($attempt = 0; $attempt -le $MaxPrompts; $attempt++) {
            $state = Get-InstallerDesktopState
            if ($state -eq 'closed') { return $true }
            $remaining = [int][Math]::Floor($TimeoutSeconds - $watch.Elapsed.TotalSeconds)
            if ($remaining -le 0 -or $attempt -eq $MaxPrompts) { return $false }
            Write-Host '[Codex Auto Retry] 等待 Codex 關閉。請先儲存工作並結束 Codex，再選「重試」；選「取消」不會變更任何安裝。 / Waiting for Codex to close. Save your work, exit Codex, then choose Retry; Cancel leaves the installation unchanged.'
            if ((Show-CodexCloseNotice -State $state -TimeoutSeconds $remaining) -ne 4) { return $false }
        }
        return $false
    }
    finally { $watch.Stop() }
}
