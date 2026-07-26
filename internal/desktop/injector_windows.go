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

var (
	user32Injector   = syscall.NewLazyDLL("user32.dll")
	sendInput        = user32Injector.NewProc("SendInput")
	getAsyncKeyState = user32Injector.NewProc("GetAsyncKeyState")
)

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
	waitForShortcutRelease()

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

func waitForShortcutRelease() {
	const (
		vkShift    = 0x10
		vkControl  = 0x11
		vkAlt      = 0x12
		vkLeftWin  = 0x5B
		vkRightWin = 0x5C
	)
	modifiers := [...]uintptr{vkShift, vkControl, vkAlt, vkLeftWin, vkRightWin}
	deadline := time.Now().Add(1500 * time.Millisecond)
	for time.Now().Before(deadline) {
		pressed := false
		for _, key := range modifiers {
			state, _, _ := getAsyncKeyState.Call(key)
			if int16(state) < 0 {
				pressed = true
				break
			}
		}
		if !pressed {
			time.Sleep(20 * time.Millisecond)
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func InjectionRequirements() string {
	return "native SendInput enabled (elevated apps require VoxSteer to run elevated too)"
}
