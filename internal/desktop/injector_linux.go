//go:build desktop && linux

package desktop

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"
	"strings"
	"time"
)

// Injector types text with an external tool, mirroring how the local engine
// shells out to whisper-cli: the compositor-specific machinery stays outside
// the Go module.
//
//   - Wayland: wtype speaks the virtual-keyboard protocol (COSMIC, wlroots,
//     KDE). ydotool works on any compositor through uinput but needs its
//     daemon. As a last resort xdotool reaches XWayland clients only.
//   - X11: xdotool uses XTEST and handles layouts and unicode.
type Injector struct{}

const typeTimeout = 60 * time.Second

func (Injector) Type(text string) error {
	if text == "" {
		return nil
	}
	// Let focus return to the application that owned it before the shortcut.
	time.Sleep(120 * time.Millisecond)

	if waylandSession() {
		if path, err := exec.LookPath("wtype"); err == nil {
			if err := runTyper(path, text, true); err == nil {
				return nil
			} else {
				log.Printf("wtype failed, trying the next injector: %v", err)
			}
		}
		if path, err := exec.LookPath("ydotool"); err == nil {
			if err := runTyper(path, text, false, "type", "--file"); err == nil {
				return nil
			} else {
				log.Printf("ydotool failed, trying the next injector: %v", err)
			}
		}
		if os.Getenv("DISPLAY") != "" {
			if path, err := exec.LookPath("xdotool"); err == nil {
				log.Print("falling back to xdotool: text reaches XWayland applications only")
				return runTyper(path, text, false, "type", "--clearmodifiers", "--file")
			}
		}
		return fmt.Errorf("no Wayland typing tool found: install wtype (recommended) or ydotool")
	}

	if path, err := exec.LookPath("xdotool"); err == nil {
		return runTyper(path, text, false, "type", "--clearmodifiers", "--file")
	}
	return fmt.Errorf("no X11 typing tool found: install xdotool")
}

func waylandSession() bool {
	return os.Getenv("WAYLAND_DISPLAY") != ""
}

// runTyper feeds the text through stdin (wtype -) or a private temporary
// file so the transcript never appears in the process argument list.
func runTyper(binary, text string, viaStdin bool, args ...string) error {
	ctx, cancel := context.WithTimeout(context.Background(), typeTimeout)
	defer cancel()

	var cmd *exec.Cmd
	if viaStdin {
		cmd = exec.CommandContext(ctx, binary, "-")
		cmd.Stdin = strings.NewReader(text)
	} else {
		file, err := os.CreateTemp("", "voxsteer-*.txt")
		if err != nil {
			return fmt.Errorf("stage transcript: %w", err)
		}
		defer os.Remove(file.Name())
		if _, err := file.WriteString(text); err != nil {
			file.Close()
			return fmt.Errorf("stage transcript: %w", err)
		}
		file.Close()
		cmd = exec.CommandContext(ctx, binary, append(args, file.Name())...)
	}

	output, err := cmd.CombinedOutput()
	if err != nil {
		detail := strings.TrimSpace(string(output))
		if detail != "" {
			return fmt.Errorf("%s: %w: %s", binary, err, detail)
		}
		return fmt.Errorf("%s: %w", binary, err)
	}
	return nil
}

func InjectionRequirements() string {
	if waylandSession() {
		return "Wayland session: typing uses wtype (or ydotool); install one of them"
	}
	return "X11 session: typing uses xdotool; install it if missing"
}
