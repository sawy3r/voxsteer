//go:build desktop && windows && (amd64 || arm64)

package desktop

import (
	"fmt"
	"syscall"
	"time"
	"unicode/utf16"
	"unsafe"
)

const (
	inputKeyboard    = 1
	keyeventfKeyUp   = 0x0002
	keyeventfUnicode = 0x0004
)

var sendInput = syscall.NewLazyDLL("user32.dll").NewProc("SendInput")

type keyboardInput struct {
	virtualKey uint16
	scanCode   uint16
	flags      uint32
	time       uint32
	extraInfo  uintptr
}

// input64 matches the 40-byte Win32 INPUT layout on amd64 and arm64.
type input64 struct {
	inputType uint32
	_         uint32
	keyboard  keyboardInput
	_         [8]byte
}

type Injector struct{}

// Type injects UTF-16 keyboard events into the foreground application without
// reading or changing the clipboard.
func (Injector) Type(text string) error {
	time.Sleep(80 * time.Millisecond)

	codeUnits := utf16.Encode([]rune(text))
	inputs := make([]input64, 0, len(codeUnits)*2)
	for _, codeUnit := range codeUnits {
		inputs = append(
			inputs,
			input64{
				inputType: inputKeyboard,
				keyboard: keyboardInput{
					scanCode: codeUnit,
					flags:    keyeventfUnicode,
				},
			},
			input64{
				inputType: inputKeyboard,
				keyboard: keyboardInput{
					scanCode: codeUnit,
					flags:    keyeventfUnicode | keyeventfKeyUp,
				},
			},
		)
	}
	if len(inputs) == 0 {
		return nil
	}

	sent, _, callErr := sendInput.Call(
		uintptr(len(inputs)),
		uintptr(unsafe.Pointer(&inputs[0])),
		unsafe.Sizeof(inputs[0]),
	)
	if sent != uintptr(len(inputs)) {
		if callErr != syscall.Errno(0) {
			return fmt.Errorf("SendInput typed %d of %d events: %w", sent, len(inputs), callErr)
		}
		return fmt.Errorf("SendInput typed %d of %d events", sent, len(inputs))
	}
	return nil
}

func InjectionRequirements() string {
	return "native SendInput enabled (elevated apps require VoxSteer to run elevated too)"
}
