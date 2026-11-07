//go:build desktop && darwin

package desktop

import (
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/sawy3r/voxsteer/internal/app"
)

type Injector struct{}

func (Injector) Type(text string) error {
	// Let focus return to the application that owned it before the hotkey.
	time.Sleep(80 * time.Millisecond)
	if reason := typingBlocked(); reason != "" {
		return copyInstead(text, reason)
	}
	if err := typeText(text); err != nil {
		return copyInstead(text, err.Error())
	}
	return nil
}

// copyInstead saves a transcript macOS would otherwise discard: posting
// keystrokes reports no failure, and the transcript is deliberately never
// logged, so it could not be recovered afterwards.
func copyInstead(text, reason string) error {
	if err := writeClipboard(strings.TrimSpace(text)); err != nil {
		return fmt.Errorf("%s and the transcript could not be copied: %w", reason, err)
	}
	log.Printf("typing unavailable (%s): transcript copied to the clipboard", reason)
	return &app.Notice{Message: "Copied: press ⌘V to paste"}
}

func InjectionRequirements() string {
	return "Microphone and Accessibility permissions are required"
}
