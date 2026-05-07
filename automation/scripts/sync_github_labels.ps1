param(
    [string]$Repository = "cubasnik/AUSF",
    [string]$LabelsFile = ".github/labels.json"
)

$ErrorActionPreference = "Stop"

function Require-Command {
    param([string]$Name)

    $command = Get-Command $Name -ErrorAction SilentlyContinue
    if ($null -eq $command) {
        throw "Required command '$Name' was not found in PATH."
    }
}

Require-Command "gh"

$repoRoot = Split-Path -Parent (Split-Path -Parent $PSScriptRoot)
$labelsPath = Join-Path $repoRoot $LabelsFile

if (-not (Test-Path $labelsPath)) {
    throw "Labels file not found: $labelsPath"
}

$labels = Get-Content $labelsPath -Raw | ConvertFrom-Json

foreach ($label in $labels) {
    $name = [string]$label.name
    $color = [string]$label.color
    $description = [string]$label.description

    Write-Host "==> syncing label '$name'"

    $payload = @{
        new_name = $name
        color = $color
        description = $description
    } | ConvertTo-Json -Compress

    & gh api "repos/$Repository/labels/$name" --method PATCH --input - 2>$null <<< $payload
    if ($LASTEXITCODE -eq 0) {
        continue
    }

    $createPayload = @{
        name = $name
        color = $color
        description = $description
    } | ConvertTo-Json -Compress

    & gh api "repos/$Repository/labels" --method POST --input - <<< $createPayload
    if ($LASTEXITCODE -ne 0) {
        throw "Failed to create or update label '$name'."
    }
}

Write-Host "Labels synced successfully for $Repository"
