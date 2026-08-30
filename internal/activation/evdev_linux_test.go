//go:build linux

package activation

import (
	"testing"

	"github.com/sawy3r/voxsteer/internal/shortcut"
)

func TestTargetCode(t *testing.T) {
	cases := []struct {
		name string
		spec shortcut.Spec
		want uint16
	}{
		{"space", shortcut.Spec{Key: "SPACE"}, 57},
		{"letter", shortcut.Spec{Key: "Q"}, 16},
		{"digit", shortcut.Spec{Key: "0"}, 11},
		{"function", shortcut.Spec{Key: "F12"}, 88},
		{"mouse middle", shortcut.Spec{MouseButton: 3}, btnMiddle},
		{"mouse side", shortcut.Spec{MouseButton: 4}, btnSide},
		{"mouse extra", shortcut.Spec{MouseButton: 5}, btnExtra},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := targetCode(tc.spec)
			if err != nil {
				t.Fatal(err)
			}
			if got != tc.want {
				t.Fatalf("targetCode = %d, want %d", got, tc.want)
			}
		})
	}
	if _, err := targetCode(shortcut.Spec{Key: "ESC"}); err == nil {
		t.Fatal("expected an error for an unsupported key")
	}
	if _, err := targetCode(shortcut.Spec{MouseButton: 9}); err == nil {
		t.Fatal("expected an error for an unsupported mouse button")
	}
}

func TestShortcutFiresOnlyWithExactModifiers(t *testing.T) {
	spec, err := shortcut.Parse("ctrl+alt+space")
	if err != nil {
		t.Fatal(err)
	}
	l := &listener{
		spec:    spec,
		target:  57,
		events:  make(chan struct{}, 1),
		pressed: map[uint16]bool{},
	}

	l.handleKey(keyLeftCtrl, keyPressed)
	l.handleKey(keyLeftAlt, keyPressed)
	l.handleKey(57, keyPressed)
	select {
	case <-l.events:
	default:
		t.Fatal("shortcut did not fire with the exact modifiers held")
	}

	// Key repeat must not retrigger.
	l.handleKey(57, 2)
	select {
	case <-l.events:
		t.Fatal("shortcut fired on key repeat")
	default:
	}

	// An extra modifier must suppress the shortcut.
	l.handleKey(keyLeftShift, keyPressed)
	l.handleKey(57, keyPressed)
	select {
	case <-l.events:
		t.Fatal("shortcut fired with an extra modifier held")
	default:
	}

	// Releasing a required modifier must suppress the shortcut.
	l.handleKey(keyLeftShift, keyReleased)
	l.handleKey(keyLeftAlt, keyReleased)
	l.handleKey(57, keyPressed)
	select {
	case <-l.events:
		t.Fatal("shortcut fired with a required modifier released")
	default:
	}
}

func TestMouseButtonIgnoresModifiers(t *testing.T) {
	spec, err := shortcut.Parse("mouse4")
	if err != nil {
		t.Fatal(err)
	}
	l := &listener{
		spec:    spec,
		target:  btnSide,
		events:  make(chan struct{}, 1),
		pressed: map[uint16]bool{},
	}
	l.handleKey(keyLeftShift, keyPressed)
	l.handleKey(btnSide, keyPressed)
	select {
	case <-l.events:
	default:
		t.Fatal("mouse button did not fire while a modifier was held")
	}
}
