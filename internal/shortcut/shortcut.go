package shortcut

import (
	"fmt"
	"strings"
)

const (
	Default  = "ctrl+alt+space"
	Disabled = "disabled"
)

type Spec struct {
	Ctrl    bool
	Alt     bool
	Shift   bool
	Super   bool
	Key     string
	Display string
}

func Parse(value string) (Spec, error) {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" {
		value = Default
	}
	if value == Disabled {
		return Spec{Display: "Mouse / tray only"}, nil
	}

	var spec Spec
	for _, part := range strings.Split(value, "+") {
		switch strings.TrimSpace(part) {
		case "ctrl", "control":
			if spec.Ctrl {
				return Spec{}, fmt.Errorf("shortcut repeats Ctrl")
			}
			spec.Ctrl = true
		case "alt":
			if spec.Alt {
				return Spec{}, fmt.Errorf("shortcut repeats Alt")
			}
			spec.Alt = true
		case "shift":
			if spec.Shift {
				return Spec{}, fmt.Errorf("shortcut repeats Shift")
			}
			spec.Shift = true
		case "win", "super", "command", "cmd":
			if spec.Super {
				return Spec{}, fmt.Errorf("shortcut repeats the system modifier")
			}
			spec.Super = true
		case "space":
			if spec.Key != "" {
				return Spec{}, fmt.Errorf("shortcut must contain exactly one key")
			}
			spec.Key = "SPACE"
		default:
			key := strings.ToUpper(strings.TrimSpace(part))
			if !supportedKey(key) {
				return Spec{}, fmt.Errorf("unsupported shortcut key %q", part)
			}
			if spec.Key != "" {
				return Spec{}, fmt.Errorf("shortcut must contain exactly one key")
			}
			spec.Key = key
		}
	}
	if spec.Key == "" {
		return Spec{}, fmt.Errorf("shortcut is missing a key")
	}
	if !spec.Ctrl && !spec.Alt && !spec.Shift && !spec.Super &&
		!strings.HasPrefix(spec.Key, "F") {
		return Spec{}, fmt.Errorf("shortcut needs a modifier")
	}
	spec.Display = display(spec)
	return spec, nil
}

func supportedKey(key string) bool {
	if len(key) == 1 && key[0] >= 'A' && key[0] <= 'Z' {
		return true
	}
	switch key {
	case "F1", "F2", "F3", "F4", "F5", "F6",
		"F7", "F8", "F9", "F10", "F11", "F12":
		return true
	default:
		return false
	}
}

func display(spec Spec) string {
	parts := make([]string, 0, 5)
	if spec.Ctrl {
		parts = append(parts, "Ctrl")
	}
	if spec.Alt {
		parts = append(parts, "Alt")
	}
	if spec.Shift {
		parts = append(parts, "Shift")
	}
	if spec.Super {
		parts = append(parts, "Win/Cmd")
	}
	if spec.Key == "SPACE" {
		parts = append(parts, "Space")
	} else {
		parts = append(parts, spec.Key)
	}
	return strings.Join(parts, "+")
}
