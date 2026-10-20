//go:build linux

package wayland

import (
	"encoding/binary"
	"errors"
	"net"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"golang.org/x/sys/unix"
)

// controlKeys are physical positions an application could act on instead of
// typing the character the keymap assigns.
var controlKeys = map[uint32]string{
	1: "Escape", 14: "Backspace", 29: "Ctrl", 42: "Shift", 54: "Shift",
	56: "Alt", 58: "CapsLock", 100: "AltGr", 111: "Delete", 125: "Super",
}

func TestPrintableKeysAvoidControlPositions(t *testing.T) {
	seen := map[uint32]bool{}
	for _, code := range printableKeys {
		if name, ok := controlKeys[code]; ok {
			t.Errorf("printable key %d is the physical %s key", code, name)
		}
		for _, fixed := range fixedKeys {
			if code == fixed.code {
				t.Errorf("printable key %d collides with fixed key %s", code, fixed.keysym)
			}
		}
		if seen[code] {
			t.Errorf("printable key %d listed twice", code)
		}
		seen[code] = true
	}
	if len(printableKeys) != 48 {
		t.Fatalf("len(printableKeys) = %d, want 48", len(printableKeys))
	}
}

// The transcript that exposed the bug: under wtype the full stop was the
// 14th distinct character and so landed on physical Backspace.
func TestTypeKeepsTheFourteenthCharacterOffBackspace(t *testing.T) {
	text := "It mostly looks right. My questions are like, what are the dots?"
	got := typeThroughFakeCompositor(t, text, true)
	if got.text != text {
		t.Fatalf("typed %q, want %q", got.text, text)
	}
	for _, code := range got.codes {
		if name, ok := controlKeys[code]; ok {
			t.Fatalf("typed a character on the physical %s key (%d)", name, code)
		}
	}
	if got.keymaps != 1 {
		t.Fatalf("uploaded %d keymaps, want 1", got.keymaps)
	}
}

func TestTypeSwitchesKeymapsBeyondFortyEightCharacters(t *testing.T) {
	text := "abcdefghijklmnopqrstuvwxyz ABCDEFGHIJKLMNOPQRSTUVWXYZ\n0123456789\t.,;:!?'\"-–é… back to a"
	got := typeThroughFakeCompositor(t, text, true)
	if got.text != text {
		t.Fatalf("typed %q, want %q", got.text, text)
	}
	if got.keymaps < 3 {
		t.Fatalf("uploaded %d keymaps, want a switch to the second group and back", got.keymaps)
	}
}

func TestTypeSkipsUntypeableCharacters(t *testing.T) {
	got := typeThroughFakeCompositor(t, "one\rtwo\x00", true)
	if got.text != "onetwo" {
		t.Fatalf("typed %q, want %q", got.text, "onetwo")
	}
}

func TestTypeWithoutVirtualKeyboardIsUnavailable(t *testing.T) {
	startFakeCompositor(t, false)
	if err := Type("hello"); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("Type error = %v, want ErrUnavailable", err)
	}
	if err := Available(); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("Available error = %v, want ErrUnavailable", err)
	}
}

func TestTypeWithoutCompositorIsUnavailable(t *testing.T) {
	t.Setenv("XDG_RUNTIME_DIR", shortSocketDir(t))
	t.Setenv("WAYLAND_DISPLAY", "wayland-test")
	if err := Type("hello"); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("Type error = %v, want ErrUnavailable", err)
	}
}

type typed struct {
	text    string
	codes   []uint32
	keymaps int
}

func typeThroughFakeCompositor(t *testing.T, text string, offerKeyboard bool) typed {
	t.Helper()
	result := startFakeCompositor(t, offerKeyboard)
	if err := Available(); err != nil {
		t.Fatalf("Available: %v", err)
	}
	if err := Type(text); err != nil {
		t.Fatalf("Type: %v", err)
	}
	return <-result
}

// shortSocketDir keeps the socket path inside the unix path limit.
func shortSocketDir(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("/tmp", "vox")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	return dir
}

// startFakeCompositor serves just enough of the protocol for Type and
// decodes key presses through the keymaps it receives, as a client would.
// It reports what the last connection typed once that client hangs up.
func startFakeCompositor(t *testing.T, offerKeyboard bool) <-chan typed {
	t.Helper()
	dir := shortSocketDir(t)
	t.Setenv("XDG_RUNTIME_DIR", dir)
	t.Setenv("WAYLAND_DISPLAY", "wayland-test")
	listener, err := net.ListenUnix("unix", &net.UnixAddr{Name: filepath.Join(dir, "wayland-test"), Net: "unix"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = listener.Close() })

	results := make(chan typed, 1)
	go func() {
		for {
			client, err := listener.AcceptUnix()
			if err != nil {
				return
			}
			got, err := serveClient(client, offerKeyboard)
			_ = client.Close()
			if err != nil {
				t.Errorf("fake compositor: %v", err)
			}
			if got.keymaps > 0 {
				results <- got
			}
		}
	}()
	return results
}

var (
	keycodePattern = regexp.MustCompile(`<K(\d+)> = (\d+);`)
	symbolPattern  = regexp.MustCompile(`key <K(\d+)> \{\[ (\S+) \]\};`)
)

func serveClient(client *net.UnixConn, offerKeyboard bool) (typed, error) {
	var got typed
	var text strings.Builder
	interfaces := map[uint32]string{1: "wl_display"}
	var registry uint32
	symbols := map[uint32]rune{}
	reply := func(object uint32, opcode uint16, body message) error {
		var event message
		event.uint(object)
		event.uint(uint32(8+len(body))<<16 | uint32(opcode))
		_, err := client.Write(append(event, body...))
		return err
	}

	var inbound []byte
	var fds []int
	for {
		buffer := make([]byte, 4096)
		oob := make([]byte, unix.CmsgSpace(4*4))
		n, oobn, _, _, err := client.ReadMsgUnix(buffer, oob)
		if err != nil || n == 0 {
			got.text = text.String()
			return got, nil
		}
		inbound = append(inbound, buffer[:n]...)
		if oobn > 0 {
			messages, err := unix.ParseSocketControlMessage(oob[:oobn])
			if err != nil {
				return got, err
			}
			for _, m := range messages {
				received, err := unix.ParseUnixRights(&m)
				if err != nil {
					return got, err
				}
				fds = append(fds, received...)
			}
		}
		for len(inbound) >= 8 {
			size := int(binary.NativeEndian.Uint32(inbound[4:]) >> 16)
			if len(inbound) < size {
				break
			}
			object := binary.NativeEndian.Uint32(inbound)
			opcode := uint16(binary.NativeEndian.Uint32(inbound[4:]))
			args := eventArgs(inbound[8:size])
			inbound = inbound[size:]

			switch iface := interfaces[object]; {
			case iface == "wl_display" && opcode == 0: // sync
				callback := args.uint()
				var done message
				done.uint(0)
				if err := reply(callback, 0, done); err != nil {
					return got, err
				}
			case iface == "wl_display" && opcode == 1: // get_registry
				registry = args.uint()
				interfaces[registry] = "wl_registry"
				globals := []string{"wl_compositor", seatInterface}
				if offerKeyboard {
					globals = append(globals, managerInterface)
				}
				for i, name := range globals {
					var global message
					global.uint(uint32(i + 1))
					global.string(name)
					global.uint(1)
					if err := reply(registry, 0, global); err != nil {
						return got, err
					}
				}
			case iface == "wl_registry" && opcode == 0: // bind
				args.uint()
				name := args.string()
				args.uint()
				interfaces[args.uint()] = name
			case iface == managerInterface && opcode == 0: // create_virtual_keyboard
				args.uint()
				interfaces[args.uint()] = "zwp_virtual_keyboard_v1"
			case iface == "zwp_virtual_keyboard_v1" && opcode == 0: // keymap
				if len(fds) == 0 {
					return got, errors.New("keymap request without a file descriptor")
				}
				args.uint()
				size := int(args.uint())
				fd := fds[0]
				fds = fds[1:]
				data := make([]byte, size)
				_, err := unix.Pread(fd, data, 0)
				unix.Close(fd)
				if err != nil {
					return got, err
				}
				symbols, err = parseKeymap(strings.TrimRight(string(data), "\x00"))
				if err != nil {
					return got, err
				}
				got.keymaps++
			case iface == "zwp_virtual_keyboard_v1" && opcode == 1: // key
				args.uint()
				code, state := args.uint(), args.uint()
				if state != keyPressed {
					continue
				}
				r, ok := symbols[code]
				if !ok {
					return got, errors.New("pressed key " + strconv.Itoa(int(code)) + " with no symbol in the keymap")
				}
				got.codes = append(got.codes, code)
				text.WriteRune(r)
			}
		}
	}
}

// parseKeymap maps evdev codes to the characters a client would type.
func parseKeymap(keymap string) (map[uint32]rune, error) {
	declared := map[string]bool{}
	for _, match := range keycodePattern.FindAllStringSubmatch(keymap, -1) {
		if match[1] != match[2] {
			return nil, errors.New("keycode name and value differ: " + match[0])
		}
		declared[match[1]] = true
	}
	symbols := map[uint32]rune{}
	for _, match := range symbolPattern.FindAllStringSubmatch(keymap, -1) {
		if !declared[match[1]] {
			return nil, errors.New("symbol for undeclared keycode " + match[1])
		}
		xkb, _ := strconv.Atoi(match[1])
		var r rune
		switch name := match[2]; name {
		case "space":
			r = ' '
		case "Return":
			r = '\n'
		case "Tab":
			r = '\t'
		default:
			value, err := strconv.ParseUint(strings.TrimPrefix(name, "U"), 16, 32)
			if err != nil || !strings.HasPrefix(name, "U") {
				return nil, errors.New("unexpected keysym " + name)
			}
			r = rune(value)
		}
		symbols[uint32(xkb-8)] = r
	}
	return symbols, nil
}
