param(
    [string]$Python = "python"
)

$ErrorActionPreference = "Stop"

$scriptDir = Split-Path -Parent $MyInvocation.MyCommand.Path
$repoRoot = Resolve-Path (Join-Path $scriptDir "..\..")
$runner = Join-Path $repoRoot "automation\scripts\run_full_validation.py"

Write-Host "Running fast validation from $repoRoot"

Push-Location $repoRoot
try {
    & $Python $runner
    if ($LASTEXITCODE -ne 0) {
        throw "fast validation failed with exit code $LASTEXITCODE"
    }
}
finally {
    Pop-Location
}