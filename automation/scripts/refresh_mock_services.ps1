param(
    [string[]]$Services = @("mock-amf", "mock-nrf")
)

$ErrorActionPreference = "Stop"

function Invoke-Compose {
    param(
        [Parameter(Mandatory = $true)]
        [string[]]$Args,

        [switch]$IgnoreExitCode
    )

    & docker compose @Args
    if (-not $IgnoreExitCode -and $LASTEXITCODE -ne 0) {
        throw "docker compose $($Args -join ' ') failed with exit code $LASTEXITCODE"
    }
}

if ($Services.Count -eq 0) {
    throw "Provide at least one Docker Compose service name."
}

Write-Host "Refreshing mock services: $($Services -join ', ')"

$stopArgs = @("stop", "--timeout", "5") + $Services
$rmArgs = @("rm", "-f", "-s") + $Services
$upArgs = @("up", "-d", "--force-recreate") + $Services

Invoke-Compose -Args $stopArgs -IgnoreExitCode
Invoke-Compose -Args $rmArgs -IgnoreExitCode
Invoke-Compose -Args $upArgs

Write-Host "Mock services refreshed successfully."