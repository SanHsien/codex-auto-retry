[CmdletBinding()]
param()

Set-StrictMode -Version Latest
$ErrorActionPreference = "Stop"

$repoRoot = Split-Path -Parent $PSScriptRoot
Set-Location -LiteralPath $repoRoot

$venvPython = Join-Path $repoRoot ".venv\Scripts\python.exe"
if (Test-Path -LiteralPath $venvPython) {
    $pythonExe = $venvPython
} else {
    $pythonExe = (Get-Command python -ErrorAction Stop).Source
}

$env:PYTHONUTF8 = "1"
$env:PYTHONIOENCODING = "utf-8"

function Invoke-PythonStep {
    param(
        [Parameter(Mandatory)]
        [string]$Label,
        [Parameter(Mandatory)]
        [string[]]$Arguments
    )

    Write-Host "==> $Label"
    & $script:pythonExe @Arguments
    if ($LASTEXITCODE -ne 0) {
        throw "$Label failed with exit code $LASTEXITCODE"
    }
}

Invoke-PythonStep -Label "Compile maintained Python" -Arguments @(
    "-m", "compileall", "-q", "tools"
)
Invoke-PythonStep -Label "Ruff (E9 + F)" -Arguments @(
    "-m", "ruff", "check", "--select", "E9,F", "--target-version", "py310",
    "tools"
)
Invoke-PythonStep -Label "Pytest (fork tools)" -Arguments @(
    "-m", "pytest", "-c", "tools/pytest.ini", "tools/tests"
)
Invoke-PythonStep -Label "Check Markdown links" -Arguments @(
    "tools\check_links.py"
)

Write-Host "WINDOWS DEV CHECK GREEN"
