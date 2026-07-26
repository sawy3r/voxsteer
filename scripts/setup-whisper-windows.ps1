[CmdletBinding()]
param(
    [ValidateSet("amd64", "arm64")]
    [string]$Architecture = "amd64",
    [string]$Destination = "dist",
    [switch]$SkipModel
)

$ErrorActionPreference = "Stop"
$projectRoot = Split-Path -Parent $PSScriptRoot
Set-Location $projectRoot

foreach ($command in @("git", "cmake", "go")) {
    if (-not (Get-Command $command -ErrorAction SilentlyContinue)) {
        throw "$command is required and was not found on PATH"
    }
}
if ($Architecture -eq "amd64" -and -not (Get-Command "nvcc" -ErrorAction SilentlyContinue)) {
    throw "The NVIDIA CUDA Toolkit is required for the RTX 3090 build (nvcc not found)"
}

$source = Join-Path $projectRoot ".build/whisper.cpp"
$build = Join-Path $projectRoot ".build/whisper-$Architecture"
$bundle = Join-Path $projectRoot "$Destination/windows-$Architecture"
$whisperBundle = Join-Path $bundle "whisper/Release"
$modelBundle = Join-Path $bundle "models"

if (-not (Test-Path $source)) {
    New-Item -ItemType Directory -Force -Path (Split-Path -Parent $source) | Out-Null
    git clone --depth 1 --branch v1.9.1 https://github.com/ggml-org/whisper.cpp.git $source
}

if ($Architecture -eq "amd64") {
    $modelName = "large-v3-turbo-q5_0"
    $modelSha1 = "e050f7970618a659205450ad97eb95a18d69c9ee"
    $cmakeArchitecture = "x64"
    $backendArgs = @("-DGGML_CUDA=1", "-DCMAKE_CUDA_ARCHITECTURES=86")
} else {
    # CUDA is unavailable on Snapdragon. This produces a native ARM64 CPU build.
    $modelName = "base.en"
    $modelSha1 = "137c40403d78fd54d454da0f9bd998f78703390c"
    $cmakeArchitecture = "ARM64"
    $backendArgs = @()
}

$configureArgs = @(
    "-S", $source,
    "-B", $build,
    "-A", $cmakeArchitecture,
    "-DBUILD_SHARED_LIBS=OFF",
    "-DWHISPER_BUILD_TESTS=OFF"
) + $backendArgs
cmake @configureArgs
cmake --build $build --config Release --parallel

New-Item -ItemType Directory -Force -Path $whisperBundle, $modelBundle | Out-Null
$whisperExe = Join-Path $build "bin/Release/whisper-cli.exe"
if (-not (Test-Path $whisperExe)) {
    throw "whisper-cli.exe was not produced at $whisperExe"
}
Copy-Item $whisperExe (Join-Path $whisperBundle "whisper-cli.exe") -Force

$modelFile = "ggml-$modelName.bin"
$modelPath = Join-Path $modelBundle $modelFile
if (-not $SkipModel) {
    if (-not (Test-Path $modelPath)) {
        $modelUrl = "https://huggingface.co/ggerganov/whisper.cpp/resolve/main/$modelFile"
        Invoke-WebRequest -Uri $modelUrl -OutFile $modelPath
    }
    $actualSha1 = (Get-FileHash -Algorithm SHA1 $modelPath).Hash.ToLowerInvariant()
    if ($actualSha1 -ne $modelSha1) {
        throw "Model checksum mismatch: got $actualSha1, expected $modelSha1"
    }
}

$env:CGO_ENABLED = "0"
$env:GOOS = "windows"
$env:GOARCH = $Architecture
go build `
    -buildvcs=false `
    -tags desktop `
    -trimpath `
    -ldflags "-H=windowsgui -s -w" `
    -o (Join-Path $bundle "voxsteer.exe") `
    ./cmd/voxsteer

$config = @{
    engine = "local"
    language = "en"
    shortcut = "ctrl+alt+space"
    local = @{
        binary = "whisper/Release/whisper-cli.exe"
        model = "models/$modelFile"
    }
    cloud = @{
        endpoint = ""
        api_key = ""
        model = "whisper-1"
    }
}
$configJson = $config | ConvertTo-Json -Depth 4
$utf8WithoutBom = New-Object System.Text.UTF8Encoding($false)
[System.IO.File]::WriteAllText((Join-Path $bundle "config.json"), $configJson, $utf8WithoutBom)
if ($SkipModel) {
    Write-Host "Created Windows $Architecture installer staging bundle at $bundle (model downloads during installation)"
} else {
    Write-Host "Created portable Windows $Architecture bundle at $bundle"
}
