param(
    [Parameter(ValueFromRemainingArguments = $true)]
    [string[]]$Arguments
)

$ErrorActionPreference = "Stop"

$repoRoot = Split-Path -Parent $MyInvocation.MyCommand.Path
$makeCandidates = @("make", "gmake", "mingw32-make")

$resolvedMake = $null
foreach ($candidate in $makeCandidates) {
    $command = Get-Command $candidate -ErrorAction SilentlyContinue
    if ($null -ne $command) {
        $resolvedMake = $command.Source
        break
    }
}

if ($null -eq $resolvedMake) {
    throw "No GNU Make-compatible executable found in PATH. Install make/gmake or MSYS2 mingw32-make, then rerun .\make.ps1 <target>."
}

Push-Location $repoRoot
try {
    Write-Host "==> $resolvedMake $($Arguments -join ' ')"
    & $resolvedMake @Arguments
    if ($LASTEXITCODE -ne 0) {
        throw "make failed with exit code $LASTEXITCODE"
    }
}
finally {
    Pop-Location
}