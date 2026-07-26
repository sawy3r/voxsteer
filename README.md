# VoxSteer

**Your voice at the controls.**

A cross-platform Go dictation and computer-control foundation: press
`Ctrl+Shift+S`, speak, press it
again, and the transcription is typed into the focused application without
using the clipboard. VoxSteer is cloud-first, with optional local Whisper
transcription for private and offline use.

## Architecture

The orchestration, WAV encoding, cloud client, and local engine adapter are pure
Go and independently tested. Native desktop adapters are enabled with the
`desktop` build tag:

```text
Ctrl+Shift+S → microphone (16 kHz mono) → cloud or whisper.cpp → native typing
                      ↑                                      ↓
                   tray status                         focused field
```

Windows uses WASAPI and `SendInput` directly and builds without CGO. macOS and
Linux use miniaudio and RobotGo behind the same interfaces.

The local adapter executes a `whisper-cli` built from the official whisper.cpp
project. This keeps CUDA/Vulkan/Metal build choices outside the Go module and
lets one application binary switch engines at runtime.

## Run with a cloud transcription endpoint

The endpoint must accept the OpenAI-compatible `multipart/form-data` contract
(`file`, `model`, and optional `language`) and return `{"text":"..."}`.

```sh
export VTT_ENGINE=cloud
export VTT_CLOUD_ENDPOINT=https://your-provider.example/v1/audio/transcriptions
export VTT_CLOUD_API_KEY=...
export VTT_CLOUD_MODEL=whisper-1
make build-desktop
./bin/voxsteer
```

## Run with local whisper.cpp

Build whisper.cpp with the acceleration backend appropriate to the machine. For
an NVIDIA GPU:

```sh
git clone https://github.com/ggml-org/whisper.cpp
cmake -S whisper.cpp -B whisper.cpp/build -DGGML_CUDA=1
cmake --build whisper.cpp/build --config Release -j

export VTT_ENGINE=local
export VTT_WHISPER_BINARY="$PWD/whisper.cpp/build/bin/whisper-cli"
export VTT_WHISPER_MODEL=/path/to/ggml-base.en.bin
make build-desktop
./bin/voxsteer
```

Optional: set `VTT_LANGUAGE` to an ISO language code.

## Windows

Both Windows targets are supported:

| Target | Audio and typing | Recommended transcription |
| --- | --- | --- |
| `windows/amd64` | Native WASAPI + `SendInput` | CUDA whisper.cpp on the RTX 3090 |
| `windows/arm64` | Native WASAPI + `SendInput` | Cloud endpoint or native ARM64 CPU whisper.cpp |

Build both application binaries from any Go development machine:

```powershell
.\scripts\build-windows.ps1
```

### Installer

The Windows installer is cloud-first. By default it installs only the small
core application and asks for an OpenAI-compatible endpoint, API key, model, and
language. Local transcription is an unchecked optional task:

- x64 downloads the verified `large-v3-turbo-q5_0` model and installs the
  prebuilt CUDA runtime.
- ARM64 downloads the verified `base.en` model and installs the native CPU
  runtime.

Build an architecture-specific installer on Windows:

```powershell
.\scripts\build-installer.ps1 -Architecture amd64
.\scripts\build-installer.ps1 -Architecture arm64
```

This requires Inno Setup 7 plus the local-runtime prerequisites described
below. The resulting installers are written to `dist\installers`. Model files
are downloaded and SHA-256 verified only when the user selects local
transcription, so cloud users do not download them.

For a complete local bundle, run one of:

```powershell
# RTX 3090: CUDA build targeting NVIDIA Ampere sm_86
.\scripts\setup-whisper-windows.ps1 -Architecture amd64

# Snapdragon: native ARM64 CPU build
.\scripts\setup-whisper-windows.ps1 -Architecture arm64
```

The setup script produces `dist\windows-<architecture>\voxsteer.exe`,
`config.json`, the matching `whisper-cli.exe`, and a verified model. The amd64
bundle uses `large-v3-turbo-q5_0`; ARM64 uses the smaller `base.en` model.

The RTX desktop may run Llama and Whisper concurrently, but they share VRAM.
If Llama already occupies most of the 3090, reduce its GPU layers/context or use
the quantized Whisper model supplied by the script.

`SendInput` follows Windows integrity boundaries: a normal application cannot
type into an elevated Administrator window. Run both applications at the same
integrity level.

## Configuration

Copy `config.example.json` to `config.json` beside the executable. Relative
Whisper paths are resolved from that file. The app also checks the per-user
`VoxSteer/config.json` directory returned by the operating system. Existing
`VTT_*` environment variables override file values.

## Native prerequisites

- Linux: GCC plus AppIndicator, GTK 3, and XTest development libraries required
  by the tray, hotkey, and RobotGo packages. On Debian/Ubuntu:
  `sudo apt install build-essential libayatana-appindicator3-dev libgtk-3-dev
  libx11-dev libxtst-dev`. X11/XWayland is required for global injection; native
  Wayland deliberately prevents arbitrary applications from typing into other
  clients.
- macOS: Xcode command-line tools. Grant Accessibility, Input Monitoring, and
  microphone permissions.
- Windows: no C compiler is needed for the Go application. Building local
  whisper.cpp requires Visual Studio C++ Build Tools; the RTX 3090 build also
  requires the NVIDIA CUDA Toolkit. Grant microphone access under
  Settings → Privacy & security → Microphone.

The desktop application is not a dependency-free pure-Go executable: audio,
global hotkeys, tray integration, and keyboard injection all touch native OS
APIs. Selecting the cloud engine removes whisper.cpp/CUDA from the runtime, but
does not remove those desktop requirements.

## Verification

```sh
go test ./...
go test -race ./...
```
