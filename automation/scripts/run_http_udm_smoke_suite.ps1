param(
    [string]$Python = "python",
    [switch]$SkipBuild
)

$ErrorActionPreference = "Stop"

$scriptDir = Split-Path -Parent $MyInvocation.MyCommand.Path
$repoRoot = Resolve-Path (Join-Path $scriptDir "..\..")
$runner = Join-Path $repoRoot "automation\scripts\run_http_udm_smoke_suite.py"

Write-Host "Running HTTP UDM smoke suite from $repoRoot"

$runnerArgs = @($runner)
if ($SkipBuild) { $runnerArgs += "--skip-build" }

Push-Location $repoRoot
try {
    & $Python @runnerArgs
    if ($LASTEXITCODE -ne 0) {
        throw "HTTP UDM smoke suite failed with exit code $LASTEXITCODE"
    }
}
finally {
    Pop-Location
}
