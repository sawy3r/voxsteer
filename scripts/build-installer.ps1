[CmdletBinding()]
param(
    [ValidateSet("amd64", "arm64")]
    [string]$Architecture = "amd64"
)

$ErrorActionPreference = "Stop"
$projectRoot = Split-Path -Parent $PSScriptRoot
Set-Location $projectRoot

# Build the application and the optional local runtime. The model itself is
# downloaded only when the user selects local transcription in Setup.
& (Join-Path $PSScriptRoot "setup-whisper-windows.ps1") `
    -Architecture $Architecture `
    -Destination "dist" `
    -SkipModel

$iscc = Get-Command "ISCC.exe" -ErrorAction SilentlyContinue
if ($iscc) {
    $isccPath = $iscc.Source
} else {
    $candidates = @(
        "${env:ProgramFiles}\Inno Setup 7\ISCC.exe",
        "${env:ProgramFiles(x86)}\Inno Setup 7\ISCC.exe"
    )
    $isccPath = $candidates | Where-Object { Test-Path $_ } | Select-Object -First 1
}
if (-not $isccPath) {
    throw "Inno Setup 7 is required. Install it from https://jrsoftware.org/isinfo.php"
}

& $isccPath `
    "/DArchitecture=$Architecture" `
    (Join-Path $projectRoot "installer/voxsteer.iss")

Write-Host "Installer created under dist/installers"
