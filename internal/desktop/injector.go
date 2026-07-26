//go:build desktop && !windows

package desktop

import (
	"fmt"
	"runtime"
	"time"

	"github.com/go-vgo/robotgo"
)

type Injector struct{}

func (Injector) Type(text string) error {
	// Let focus return to the application that owned it before the hotkey.
	time.Sleep(80 * time.Millisecond)
	robotgo.Type(text)
	if runtime.GOOS == "linux" {
		// RobotGo cannot inject into native Wayland clients without compositor support.
		return nil
	}
	return nil
}

func InjectionRequirements() string {
	switch runtime.GOOS {
	case "darwin":
		return "Accessibility and Input Monitoring permissions are required"
	case "linux":
		return "X11/XWayland is required; native Wayland blocks global input injection"
	default:
		return fmt.Sprintf("keyboard injection enabled for %s", runtime.GOOS)
	}
}
