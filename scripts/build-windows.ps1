[CmdletBinding()]
param(
    [ValidateSet("all", "amd64", "arm64")]
    [string]$Architecture = "all"
)

$ErrorActionPreference = "Stop"
$projectRoot = Split-Path -Parent $PSScriptRoot
Set-Location $projectRoot

go test ./...

$targets = if ($Architecture -eq "all") { @("amd64", "arm64") } else { @($Architecture) }
New-Item -ItemType Directory -Force -Path "bin" | Out-Null

foreach ($target in $targets) {
    $env:CGO_ENABLED = "0"
    $env:GOOS = "windows"
    $env:GOARCH = $target
    $output = "bin/voxsteer-windows-$target.exe"
    go build `
        -buildvcs=false `
        -tags desktop `
        -trimpath `
        -ldflags "-H=windowsgui -s -w" `
        -o $output `
        ./cmd/voxsteer
    Write-Host "Built $output"
}

Copy-Item "config.example.json" "bin/config.example.json" -Force
