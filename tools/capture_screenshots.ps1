<#
重拍 assets/ 的介面截圖（設定視窗中英文、內嵌管理面板）。

全程使用暫存資料夾的假資料，不讀寫真實的 Codex Auto Retry 設定，也不啟動背景服務。
面板用 panel.html 內建的 ?preview 假資料，以無頭 Edge 截圖；需要先跑過 scripts\build.ps1。

    pwsh -NoProfile -File tools\capture_screenshots.ps1
    pwsh -NoProfile -File tools\capture_screenshots.ps1 -OutputDirectory $env:TEMP\shots
#>
[CmdletBinding()]
param(
    [string]$OutputDirectory = (Join-Path (Split-Path -Parent $PSScriptRoot) 'assets')
)

$ErrorActionPreference = 'Stop'
$repoRoot = Split-Path -Parent $PSScriptRoot
$outputPath = [IO.Path]::GetFullPath($OutputDirectory)
New-Item -ItemType Directory -Force -Path $outputPath | Out-Null

# The settings window is Windows Forms and must run in Windows PowerShell 5.1,
# the same host the tray uses. Each language gets its own fixture so the
# fallback prompt matches the language shown.
$settingsCapture = @'
param([string]$RepoRoot, [string]$Language, [string]$Prompt, [string]$Output)
$ErrorActionPreference = 'Stop'
Add-Type -AssemblyName System.Windows.Forms, System.Drawing
$dataDir = Join-Path ([IO.Path]::GetTempPath()) ('codex-auto-retry-shot-' + [guid]::NewGuid().ToString('N'))
New-Item -ItemType Directory -Path $dataDir | Out-Null
try {
    $utf8 = [Text.UTF8Encoding]::new($false)
    $config = @{
        config_version = 4; retry_prompt = $Prompt; max_recovery_attempts = 15
        max_consecutive_retries = 5; auth_max_attempts = 6; memory_limit_mb = 1024
        initial_delay_seconds = 5; max_delay_seconds = 300; delay_increment_seconds = 2
        delay_strategy = 'exponential'; show_notifications = $true; shared_app_server_enabled = $false
        shared_app_server_requested = $false; shared_app_server_port = 49622
        include_default_home = $false; include_cockpit_homes = $false; session_roots = @()
    }
    [IO.File]::WriteAllText((Join-Path $dataDir 'config.json'), ($config | ConvertTo-Json), $utf8)
    [IO.File]::WriteAllText((Join-Path $dataDir 'control.json'), '{"paused":false}', $utf8)
    $now = [DateTimeOffset]::UtcNow
    $state = @{ threads = @{ '019fa94e-0103-7183-b405-36bd307b6dca' = @{ stopped = @{
        class = 'rate_limit'; attempts = 15; consecutive_retries = 5
        max_attempts = 15; max_consecutive_retries = 5; reason = 'recovery_attempt_limit'
        failed_at = $now.AddMinutes(-20).ToString('o'); stopped_at = $now.AddMinutes(-2).ToString('o')
    } } } }
    [IO.File]::WriteAllText((Join-Path $dataDir 'state.json'), ($state | ConvertTo-Json -Depth 6), $utf8)
    # The window shows "running" only for a live process whose path matches
    # -Executable. This host process stands in for the watchdog.
    $selfPath = (Get-Process -Id $PID).Path
    $status = @{ running = $true; pid = $PID; last_scan_at = $now.ToString('o'); controller_state = '' }
    [IO.File]::WriteAllText((Join-Path $dataDir 'status.json'), ($status | ConvertTo-Json), $utf8)

    $source = [IO.File]::ReadAllText((Join-Path $RepoRoot 'scripts\source\ui\settings.ps1'))
    $loop = '[void]$form.ShowDialog()'
    if (-not $source.Contains($loop)) { throw 'Settings form entry point changed.' }
    . ([scriptblock]::Create($source.Replace($loop, ''))) -DataDir $dataDir -Executable $selfPath -SmokeTest
    $form.Show()
    [System.Windows.Forms.Application]::DoEvents()
    $timer.Stop()
    $script:currentLanguage = $Language
    Apply-Language
    Update-RuntimeView
    [System.Windows.Forms.Application]::DoEvents()
    $bitmap = [System.Drawing.Bitmap]::new($form.Width, $form.Height)
    try {
        $form.DrawToBitmap($bitmap, [System.Drawing.Rectangle]::new(0, 0, $form.Width, $form.Height))
        $bitmap.Save($Output, [System.Drawing.Imaging.ImageFormat]::Png)
    } finally { $bitmap.Dispose() }
    $form.Close()
} finally {
    Remove-Item -LiteralPath $dataDir -Recurse -Force -ErrorAction SilentlyContinue
}
'@
$captureScript = Join-Path ([IO.Path]::GetTempPath()) ('codex-auto-retry-capture-' + [guid]::NewGuid().ToString('N') + '.ps1')
[IO.File]::WriteAllText($captureScript, $settingsCapture, [Text.UTF8Encoding]::new($true))
try {
    $prompt = -join [char[]](0x7e7c, 0x7e8c)
    foreach ($shot in @(@{ Language = 'zh'; Prompt = $prompt; File = 'settings_zh.png' }, @{ Language = 'en'; Prompt = 'Continue'; File = 'settings_en.png' })) {
        $target = Join-Path $outputPath $shot.File
        & powershell.exe -NoLogo -NoProfile -ExecutionPolicy Bypass -File $captureScript `
            -RepoRoot $repoRoot -Language $shot.Language -Prompt $shot.Prompt -Output $target
        if ($LASTEXITCODE -ne 0) { throw "Settings capture failed: $($shot.File)" }
        Write-Host "Saved $target"
    }
}
finally {
    Remove-Item -LiteralPath $captureScript -Force -ErrorAction SilentlyContinue
}

$edge = @(
    "${env:ProgramFiles(x86)}\Microsoft\Edge\Application\msedge.exe",
    "$env:ProgramFiles\Microsoft\Edge\Application\msedge.exe"
) | Where-Object { Test-Path -LiteralPath $_ } | Select-Object -First 1
if (-not $edge) { throw 'Microsoft Edge is required for the panel screenshot.' }
$panel = Join-Path $repoRoot 'scripts\source\ui\dist\panel.html'
if (-not (Test-Path -LiteralPath $panel)) { throw 'Build the panel first: scripts\build.ps1' }
$panelUrl = ([Uri]$panel).AbsoluteUri + '?preview'
$panelTarget = Join-Path $outputPath 'panel.png'
$profileDir = Join-Path ([IO.Path]::GetTempPath()) ('codex-auto-retry-edge-' + [guid]::NewGuid().ToString('N'))
try {
    & $edge --headless=new --disable-gpu --hide-scrollbars --no-first-run `
        "--user-data-dir=$profileDir" --window-size=1024,1100 --virtual-time-budget=3000 `
        "--screenshot=$panelTarget" $panelUrl 2>&1 | Out-Null
    if (-not (Test-Path -LiteralPath $panelTarget)) { throw 'Panel screenshot was not written.' }
    Write-Host "Saved $panelTarget"
}
finally {
    Remove-Item -LiteralPath $profileDir -Recurse -Force -ErrorAction SilentlyContinue
}
