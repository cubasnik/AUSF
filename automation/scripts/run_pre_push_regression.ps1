param(
    [string]$Python = "python"
)

$ErrorActionPreference = "Stop"

$scriptDir = Split-Path -Parent $MyInvocation.MyCommand.Path
$repoRoot = Resolve-Path (Join-Path $scriptDir "..\..")
$runner = Join-Path $repoRoot "automation\scripts\run_fast_validation.ps1"

Write-Host "Running pre-push regression suite from $repoRoot"

Push-Location $repoRoot
try {
    & $runner -Python $Python
    if ($LASTEXITCODE -ne 0) {
        throw "pre-push regression suite failed with exit code $LASTEXITCODE"
    }
}
finally {
    Pop-Location
}