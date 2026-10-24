[CmdletBinding()]
param(
    [ValidateSet('gui', 'status', 'enable', 'disable', 'start', 'stop', 'open-settings', 'open-folder', 'open-log', 'launch-codex', 'safe-disable', 'uninstall')]
    [string]$Action = 'gui',
    [switch]$RemoveData,
    [switch]$NoPrompt,
    [string]$UserProfileRoot = $env:USERPROFILE,
    [string]$LocalAppDataRoot = $env:LOCALAPPDATA,
    [string]$RunName = 'CodexAutoRetry',
    [string]$ReleaseRoot = ''
)

$actualRoot = if (-not [string]::IsNullOrWhiteSpace($ReleaseRoot)) { [System.IO.Path]::GetFullPath($ReleaseRoot) } else { $PSScriptRoot }
$manager = Join-Path $actualRoot 'payload\codex-auto-retry\scripts\startup-manager.ps1'
if (-not (Test-Path -LiteralPath $manager -PathType Leaf)) {
    # Also support launching the copy bundled inside the plugin payload.
    $manager = Join-Path $actualRoot '..\..\scripts\startup-manager.ps1'
}
if (-not (Test-Path -LiteralPath $manager -PathType Leaf)) {
    $manager = Join-Path $actualRoot 'scripts\startup-manager.ps1'
}
if (-not (Test-Path -LiteralPath $manager -PathType Leaf)) { throw "安裝包缺少啟動管理員：$manager / The packaged startup manager is missing: $manager" }
$arguments = @(
    '-NoLogo', '-NoProfile', '-NonInteractive', '-ExecutionPolicy', 'Bypass',
    '-File', $manager, '-Action', $Action, '-ReleaseRoot', $actualRoot,
    '-UserProfileRoot', $UserProfileRoot, '-LocalAppDataRoot', $LocalAppDataRoot,
    '-RunName', $RunName
)
if ($RemoveData) { $arguments += '-RemoveData' }
if ($NoPrompt) { $arguments += '-NoPrompt' }
& powershell.exe @arguments
exit $LASTEXITCODE
