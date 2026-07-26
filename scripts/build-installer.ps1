[CmdletBinding()]
param(
    [ValidateSet("amd64", "arm64")]
    [string]$Architecture = "amd64",
    [string]$Version = $(if ($env:VOXSTEER_VERSION) { $env:VOXSTEER_VERSION } else { "0.1.0" })
)

$ErrorActionPreference = "Stop"
$projectRoot = Split-Path -Parent $PSScriptRoot
Set-Location $projectRoot

if ($Architecture -eq "arm64") {
    # There is no upstream Windows ARM64 whisper.cpp bundle, so build the small
    # native runtime. The model is still downloaded only when Local is selected.
    & (Join-Path $PSScriptRoot "setup-whisper-windows.ps1") `
        -Architecture $Architecture `
        -Destination "dist" `
        -SkipModel
} else {
    # The x64 CUDA runtime is downloaded and verified by Setup only when Local
    # is selected, keeping the normal cloud installer small.
    go test ./...
    $bundle = Join-Path $projectRoot "dist/windows-amd64"
    New-Item -ItemType Directory -Force -Path $bundle | Out-Null
    $env:CGO_ENABLED = "0"
    $env:GOOS = "windows"
    $env:GOARCH = "amd64"
    go build `
        -buildvcs=false `
        -tags desktop `
        -trimpath `
        -ldflags "-H=windowsgui -s -w" `
        -o (Join-Path $bundle "voxsteer.exe") `
        ./cmd/voxsteer
}

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
    "/DAppVersion=$Version" `
    (Join-Path $projectRoot "installer/voxsteer.iss")

Write-Host "Installer created under dist/installers"
