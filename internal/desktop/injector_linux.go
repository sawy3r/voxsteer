//go:build desktop && linux

package desktop

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/sawy3r/voxsteer/internal/wayland"
)

// Injector types text into the focused window.
//
//   - Wayland: VoxSteer speaks the virtual-keyboard protocol itself (COSMIC,
//     wlroots, KDE). wtype is not used: it can put punctuation on the
//     physical Backspace key, which Chrome and terminals then act on.
//     Compositors without the protocol, such as GNOME, fall back to ydotool,
//     which works through uinput but needs its daemon, and as a last resort
//     to xdotool, which reaches XWayland clients only.
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
		err := wayland.Type(text)
		if !errors.Is(err, wayland.ErrUnavailable) {
			return err
		}
		log.Printf("trying an external typing tool: %v", err)
		if path, err := exec.LookPath("ydotool"); err == nil {
			if err := runTyper(path, text, "type", "--file"); err == nil {
				return nil
			} else {
				log.Printf("ydotool failed, trying the next injector: %v", err)
			}
		}
		if os.Getenv("DISPLAY") != "" {
			if path, err := exec.LookPath("xdotool"); err == nil {
				log.Print("falling back to xdotool: text reaches XWayland applications only")
				return runTyper(path, text, "type", "--clearmodifiers", "--file")
			}
		}
		return fmt.Errorf("the compositor has no virtual-keyboard protocol and no typing tool was found: install ydotool")
	}

	if path, err := exec.LookPath("xdotool"); err == nil {
		return runTyper(path, text, "type", "--clearmodifiers", "--file")
	}
	return fmt.Errorf("no X11 typing tool found: install xdotool")
}

func waylandSession() bool {
	return os.Getenv("WAYLAND_DISPLAY") != ""
}

// runTyper feeds the text through a private temporary file so the
// transcript never appears in the process argument list.
func runTyper(binary, text string, args ...string) error {
	ctx, cancel := context.WithTimeout(context.Background(), typeTimeout)
	defer cancel()

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
	cmd := exec.CommandContext(ctx, binary, append(args, file.Name())...)

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

// InjectionRequirements reports which typing tool is actually available so
// the startup log states whether dictation can type at all, not just which
// tools would have been acceptable.
func InjectionRequirements() string {
	if waylandSession() {
		err := wayland.Available()
		if err == nil {
			return "Wayland session: typing uses the compositor's virtual-keyboard protocol"
		}
		if path, err := exec.LookPath("ydotool"); err == nil {
			return "Wayland session: typing uses ydotool (" + path + ") because " + err.Error() + "; its ydotoold daemon must be running"
		}
		if os.Getenv("DISPLAY") != "" {
			if path, err := exec.LookPath("xdotool"); err == nil {
				return "Wayland session: only xdotool (" + path + ") is available because " + err.Error() + ", so text reaches XWayland applications only; install ydotool for native Wayland windows"
			}
		}
		return "WARNING: " + err.Error() + " and no typing tool was found, so transcriptions cannot be typed; install ydotool"
	}
	if path, err := exec.LookPath("xdotool"); err == nil {
		return "X11 session: typing uses xdotool (" + path + ")"
	}
	return "WARNING: xdotool was not found, so transcriptions cannot be typed; install xdotool"
}
