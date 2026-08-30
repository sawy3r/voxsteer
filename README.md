# VoxSteer

**Your voice at the controls.**

A cross-platform Go dictation and computer-control foundation: press the
configured shortcut (default `Ctrl+Alt+Space`), speak, press it
again, and the transcription is typed into the focused application without
using the clipboard. VoxSteer is cloud-first, with optional local Whisper
transcription for private and offline use.

For managed Macs that do not allow a new application to receive native
microphone or input-control permission, VoxSteer also ships a browser mode. The
browser records audio using its existing permission, VoxSteer transcribes it on
the loopback interface, and the user explicitly copies the result for manual
paste.

## Architecture

The orchestration, WAV encoding, cloud client, and local engine adapter are pure
Go and independently tested. Native desktop adapters are enabled with the
`desktop` build tag:

```text
captured shortcut / mouse button → microphone → cloud or whisper.cpp → native typing
                                       │                                  ↓
                              live recording HUD                    focused field
```

Windows uses WASAPI and `SendInput` directly and builds without CGO. macOS uses
miniaudio and RobotGo behind the same interfaces. Linux uses miniaudio for
capture, reads the kernel input devices (evdev) for the activation shortcut so
it works on Wayland and X11 alike, and types through an external tool —
`wtype` or `ydotool` on Wayland, `xdotool` on X11 — following the same
philosophy as the whisper-cli adapter: compositor-specific machinery stays
outside the Go module.

Browser mode is a separate CGO-free executable. It never opens an audio device,
monitors input, or injects keys:

```text
browser microphone → 16 kHz mono WAV → loopback-only Go service → cloud or whisper.cpp
                                                              ↓
                                                    review and explicit copy
```

The local adapter executes a `whisper-cli` built from the official whisper.cpp
project. This keeps CUDA/Vulkan/Metal build choices outside the Go module and
lets one application binary switch engines at runtime. On Windows the child
process runs without opening a console window.

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

## Browser mode for managed macOS

Browser mode avoids native macOS Microphone, Accessibility, and Input
Monitoring permission. It still requires the selected browser to have
microphone access and the user to allow the `127.0.0.1` site when prompted.

Build and launch the Universal 2 application:

```sh
VERSION=0.3.0 BUILD_NUMBER=1 make build-macos-browser
open "dist/macos-browser/VoxSteer Browser.app"
```

The page opens Settings when transcription is not configured. Cloud API keys
are sent only to the loopback service, written to the private per-user
`config.json`, and never returned to the page. Local mode accepts an existing
`whisper-cli` and model path.

The browser page provides recording duration, a live input meter, cancellation,
an editable transcript, and an explicit Copy action. It deliberately does not
overwrite the clipboard automatically. Paste into the destination with
`Cmd+V`.

The service binds only `127.0.0.1:47823`. Every API request requires an
ephemeral token delivered in the URL fragment, an exact loopback Host, a
same-origin request, and a loopback peer. Audio is bounded to ten minutes and
only canonical 16 kHz mono PCM WAV is accepted. The token, API key, audio, and
transcript are not logged.

For a command-line developer build:

```sh
make build-browser
./bin/voxsteer-browser
```

An ad-hoc local build exercises the complete app layout but is not suitable for
external distribution. Release builds use a Developer ID identity:

```sh
VERSION=0.3.0 BUILD_NUMBER=1 \
MACOS_SIGN_IDENTITY="Developer ID Application: Example (TEAMID)" \
bash scripts/build-macos-browser.sh
```

`scripts/notarize-macos-browser.sh` notarizes and staples both the application
and DMG using an App Store Connect API key or a configured notarytool keychain
profile. Tagged GitHub releases require the signing and notarization secrets
declared in `.github/workflows/release.yml`; manual workflow runs can produce an
explicitly ad-hoc preview.

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

The Windows installer is cloud-first. Its setup wizard asks for the activation
method, language, transcription engine, and any cloud credentials, then creates
the user's `config.json` automatically. Users never need to edit JSON. Click
**Capture** in the wizard and press any supported combination of
Ctrl/Alt/Shift/Win plus a letter, number, Space, or F1–F12. Alternatively,
press the middle mouse button, Mouse 4, or Mouse 5 to use that button directly.
The default is `Ctrl+Alt+Space`, and tray-only activation remains available.
On an upgrade, select **Reconfigure VoxSteer** to run these pages again;
otherwise the installer preserves the working configuration. Local
transcription is an unchecked optional task:

- x64 downloads and verifies the `large-v3-turbo-q5_0` model and prebuilt CUDA
  runtime only when Local is selected.
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

### Recording feedback

On Windows, starting dictation displays a compact VoxSteer widget over the top
of the active window. Its bars follow the live microphone level and its label
changes while transcription is being processed. The widget does not take focus,
so typing still returns to the original application. Click **pin** on the right
while it is visible to keep it on top between recordings; click **PIN** again
to return to automatic hiding. Drag anywhere outside the pin control to move
the widget. VoxSteer remembers that position for the rest of the session, and
the activation shortcut always raises it while listening or transcribing.

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

## Linux

Both Wayland (COSMIC, GNOME, KDE, wlroots) and X11 sessions are supported. The
Linux binary is nearly pure Go — the only C in the build is the bundled
miniaudio, so `gcc` is the only build prerequisite. The tray icon is published
over D-Bus StatusNotifierItem (built into COSMIC and KDE; GNOME needs the
AppIndicator extension).

```sh
make build-linux        # requires only Go and gcc
make install-linux      # installs to ~/.local/bin plus an autostart entry
```

### Typing tool

Transcripts are typed by an external tool, chosen at runtime:

| Session | Tool | Notes |
| --- | --- | --- |
| Wayland | `wtype` (recommended) | Virtual-keyboard protocol: COSMIC, KDE, wlroots |
| Wayland | `ydotool` | Any compositor via uinput; needs `ydotoold` running |
| X11 | `xdotool` | XTEST; also the XWayland fallback |

```sh
sudo apt install wtype      # Wayland
sudo apt install xdotool    # X11
```

### Activation

Wayland compositors do not allow global hotkey grabs, and X11 grabs never fire
while a native Wayland window has focus, so VoxSteer offers two mechanisms
that work identically on Wayland and X11:

1. **Desktop keyboard shortcut (recommended).** VoxSteer listens on a control
   socket, and `voxsteer toggle` starts or stops dictation in the running
   instance. Bind it as a custom shortcut in the desktop settings (COSMIC:
   Settings → Input Devices → Keyboard → Custom Shortcuts; GNOME: Settings →
   Keyboard → Custom Shortcuts). No special permissions are needed.
2. **Built-in listener.** With read access to `/dev/input`, VoxSteer matches
   the configured shortcut — including `mouse3`–`mouse5` — directly on the
   kernel input devices. Grant access once with
   `sudo usermod -aG input $USER`, then log out and back in. Mouse-button
   activation on Wayland requires this option.

The tray menu's **Start / stop dictation** always works regardless.

### Recording widget on Linux

The recording widget is an X11 window, so it appears on X11 sessions directly
and on Wayland desktops through XWayland. It shows the same live waveform and
status as on Windows; pinning and dragging are not yet implemented. Without
any X server (a Wayland session with XWayland disabled) VoxSteer runs without
the widget and logs why.

## Configuration

The installer creates the configuration automatically under the per-user
`VoxSteer/config.json` directory returned by the operating system. Portable and
developer builds can copy `config.example.json` to `config.json` beside the
executable. Relative Whisper paths are resolved from that file. Existing
`VTT_*` environment variables override file values.

## Native prerequisites

- Linux: GCC for the bundled miniaudio (`sudo apt install build-essential`).
  No X11 or GTK development headers are needed. At runtime, install a typing
  tool (`wtype` on Wayland, `xdotool` on X11) and see the Linux section for
  activation options.
- macOS: Xcode command-line tools. Grant Accessibility, Input Monitoring, and
  microphone permissions for native desktop mode. Browser mode requires none
  of these permissions for the VoxSteer process.
- Windows: no C compiler is needed for the Go application. Building local
  whisper.cpp requires Visual Studio C++ Build Tools; the RTX 3090 build also
  requires the NVIDIA CUDA Toolkit. Grant microphone access under
  Settings → Privacy & security → Microphone.

The desktop application is not a dependency-free pure-Go executable: audio,
global hotkeys, tray integration, and keyboard injection all touch native OS
APIs. Selecting the cloud engine removes whisper.cpp/CUDA from the runtime, but
does not remove those desktop requirements.

## Verification

VoxSteer writes operational diagnostics to
`%AppData%\VoxSteer\voxsteer.log` on Windows and
`~/.config/VoxSteer/voxsteer.log` on Linux. The log records captured audio
duration, transcription success, and actionable errors, but never transcript
text. Use **Open diagnostic log** in the VoxSteer tray menu rather than
navigating AppData manually. A transcription failure also appears briefly in
the recording widget.

Browser mode writes diagnostics to
`~/Library/Application Support/VoxSteer/voxsteer-browser.log` on macOS. Use
**Open diagnostic log** in the browser page. It records engine, duration, and
errors, but never audio, API keys, session tokens, or transcript text.

```sh
go test ./...
go test -race ./...
go test -tags desktop ./...
```

## Third-party assets

The recording widget (Windows and Linux) bundles
[Inter](https://github.com/rsms/inter) Regular 4.1 by Rasmus Andersson and the
Inter Project Authors under the SIL Open Font License 1.1. The font and its
licence are kept together in `internal/hud/assets`.
