//go:build desktop && darwin

package desktop

import (
	"time"

	"github.com/go-vgo/robotgo"
)

type Injector struct{}

func (Injector) Type(text string) error {
	// Let focus return to the application that owned it before the hotkey.
	time.Sleep(80 * time.Millisecond)
	robotgo.Type(text)
	return nil
}

func InjectionRequirements() string {
	return "Accessibility and Input Monitoring permissions are required"
}
