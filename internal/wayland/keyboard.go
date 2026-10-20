//go:build linux

// Package wayland types text through the compositor's virtual-keyboard
// protocol (zwp_virtual_keyboard_v1), speaking the Wayland wire protocol
// directly so VoxSteer needs neither libwayland nor an external typing tool.
//
// It replaces wtype, which hands each distinct character the next keycode
// starting at evdev 1. Whichever character comes 14th lands on the physical
// Backspace position, and Chromium, Electron and terminals that identify keys
// by position then delete the previous character instead of typing it. Here
// literal characters only ever occupy the printable block of a PC keyboard,
// while space, Return and Tab sit on their real keys.
package wayland

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"
	"unicode"

	"golang.org/x/sys/unix"
)

// ErrUnavailable is wrapped by every error that happens before the first key
// is sent, so callers can safely fall back to another way of typing.
var ErrUnavailable = errors.New("wayland virtual keyboard unavailable")

const (
	seatInterface    = "wl_seat"
	managerInterface = "zwp_virtual_keyboard_manager_v1"

	keymapFormatXKBV1 = 1
	keyReleased       = 0
	keyPressed        = 1
)

// printableKeys are the evdev codes of the printable block of a PC keyboard:
// the digit row from 1 to =, the Q, A and Z rows, and the ISO key beside left
// Shift. An application that looks at the physical key sees an ordinary
// character key whatever symbol the keymap assigns to it.
var printableKeys = func() []uint32 {
	var keys []uint32
	for _, span := range [][2]uint32{{2, 13}, {16, 27}, {30, 41}, {43, 53}, {86, 86}} {
		for code := span[0]; code <= span[1]; code++ {
			keys = append(keys, code)
		}
	}
	return keys
}()

// fixedKey places whitespace on its real key so it behaves as the physical
// key does in every application.
type fixedKey struct {
	code   uint32
	keysym string
}

var fixedKeys = map[rune]fixedKey{
	' ':  {57, "space"},
	'\n': {28, "Return"},
	'\t': {15, "Tab"},
}

// stroke is one key press: group selects which keymap must be loaded, or -1
// for a fixed key that every keymap carries.
type stroke struct {
	group int
	code  uint32
}

// plan assigns each distinct character a printable key in order of first
// appearance. Text with more distinct characters than there are printable
// keys is split into groups that each get their own keymap.
func plan(text string) (groups [][]rune, strokes []stroke) {
	slots := map[rune]int{}
	for _, r := range text {
		if fixed, ok := fixedKeys[r]; ok {
			strokes = append(strokes, stroke{group: -1, code: fixed.code})
			continue
		}
		if r == unicode.ReplacementChar || unicode.IsControl(r) {
			continue
		}
		slot, ok := slots[r]
		if !ok {
			slot = len(slots)
			slots[r] = slot
			if slot%len(printableKeys) == 0 {
				groups = append(groups, nil)
			}
			groups[len(groups)-1] = append(groups[len(groups)-1], r)
		}
		strokes = append(strokes, stroke{
			group: slot / len(printableKeys),
			code:  printableKeys[slot%len(printableKeys)],
		})
	}
	return groups, strokes
}

// keymap builds an XKB keymap holding the fixed keys and one group of
// characters. Characters are named by their Unicode keysym, which xkbcommon
// resolves to the Latin-1 keysym below U+0100.
func keymap(group []rune) string {
	var b strings.Builder
	b.WriteString("xkb_keymap {\nxkb_keycodes \"(unnamed)\" {\nminimum = 8;\nmaximum = 255;\n")
	symbols := map[uint32]string{}
	for _, fixed := range fixedKeys {
		symbols[fixed.code] = fixed.keysym
	}
	for i, r := range group {
		symbols[printableKeys[i]] = fmt.Sprintf("U%04X", r)
	}
	codes := make([]uint32, 0, len(symbols))
	for code := range symbols {
		codes = append(codes, code)
	}
	slices.Sort(codes)
	for _, code := range codes {
		fmt.Fprintf(&b, "<K%d> = %d;\n", code+8, code+8)
	}
	b.WriteString("};\nxkb_types \"(unnamed)\" { include \"complete\" };\n")
	b.WriteString("xkb_compatibility \"(unnamed)\" { include \"complete\" };\n")
	b.WriteString("xkb_symbols \"(unnamed)\" {\n")
	for _, code := range codes {
		fmt.Fprintf(&b, "key <K%d> {[ %s ]};\n", code+8, symbols[code])
	}
	b.WriteString("};\n};\n")
	return b.String()
}

// Available reports whether the compositor offers a seat and the
// virtual-keyboard protocol, without creating a keyboard.
func Available() error {
	c, err := dial()
	if err != nil {
		return fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	defer c.close()
	if _, _, err := c.bindGlobals(); err != nil {
		return fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	return nil
}

// Type types text into the focused window. Characters that cannot be typed,
// such as control characters other than newline and tab, are skipped.
func Type(text string) error {
	groups, strokes := plan(text)
	if len(strokes) == 0 {
		return nil
	}
	c, err := dial()
	if err != nil {
		return fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	defer c.close()

	seat, manager, err := c.bindGlobals()
	if err != nil {
		return fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	keyboard := c.newID()
	var create message
	create.uint(seat)
	create.uint(keyboard)
	if err := c.send(manager, 0, create, -1); err != nil {
		return fmt.Errorf("%w: create virtual keyboard: %v", ErrUnavailable, err)
	}

	start := time.Now()
	loaded := -1
	for i, s := range strokes {
		group := s.group
		if group < 0 {
			group = max(loaded, 0)
		}
		if group != loaded {
			var chars []rune
			if group < len(groups) {
				chars = groups[group]
			}
			if err := uploadKeymap(c, keyboard, keymap(chars)); err != nil {
				return startedError(i, fmt.Errorf("upload keymap: %w", err))
			}
			loaded = group
		}
		// Waiting for the compositor after each press and release, as wtype
		// does, keeps clients from receiving a flood they may drop.
		for _, state := range []uint32{keyPressed, keyReleased} {
			var key message
			key.uint(uint32(time.Since(start).Milliseconds()))
			key.uint(s.code)
			key.uint(state)
			if err := c.send(keyboard, 1, key, -1); err != nil {
				return startedError(i, err)
			}
			if err := c.roundtrip(); err != nil {
				return startedError(i, err)
			}
		}
	}
	if err := c.send(keyboard, 3, message{}, -1); err != nil {
		return err
	}
	return c.roundtrip()
}

// startedError marks a failure as unavailable while no key has been typed
// yet, so a fallback cannot type the text twice.
func startedError(typed int, err error) error {
	if typed == 0 {
		return fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	return fmt.Errorf("virtual keyboard failed after %d keys: %w", typed, err)
}

func uploadKeymap(c *conn, keyboard uint32, text string) error {
	fd, err := unix.MemfdCreate("voxsteer-keymap", unix.MFD_CLOEXEC)
	if err != nil {
		return err
	}
	defer unix.Close(fd)
	// The compositor maps the keymap as a NUL-terminated string.
	data := append([]byte(text), 0)
	for written := 0; written < len(data); {
		n, err := unix.Write(fd, data[written:])
		if err != nil {
			return err
		}
		written += n
	}
	var request message
	request.uint(keymapFormatXKBV1)
	request.uint(uint32(len(data)))
	if err := c.send(keyboard, 0, request, fd); err != nil {
		return err
	}
	// The protocol error for a rejected keyboard or keymap arrives here,
	// before any key is pressed.
	return c.roundtrip()
}
