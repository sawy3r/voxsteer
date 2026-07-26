//go:build desktop && windows

package mousebutton

import (
	"fmt"
	"runtime"
	"sync"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

const (
	whMouseLL     = 14
	wmQuit        = 0x0012
	wmMButtonDown = 0x0207
	wmMButtonUp   = 0x0208
	wmXButtonDown = 0x020B
	wmXButtonUp   = 0x020C
	hcAction      = 0
)

var (
	user32             = windows.NewLazySystemDLL("user32.dll")
	kernel32           = windows.NewLazySystemDLL("kernel32.dll")
	procSetWindowsHook = user32.NewProc("SetWindowsHookExW")
	procCallNextHook   = user32.NewProc("CallNextHookEx")
	procUnhookHook     = user32.NewProc("UnhookWindowsHookEx")
	procGetMessage     = user32.NewProc("GetMessageW")
	procPostThreadMsg  = user32.NewProc("PostThreadMessageW")
	procGetAsyncKey    = user32.NewProc("GetAsyncKeyState")
	procGetThreadID    = kernel32.NewProc("GetCurrentThreadId")
)

type point struct {
	X int32
	Y int32
}

type message struct {
	Window  uintptr
	Message uint32
	WParam  uintptr
	LParam  uintptr
	Time    uint32
	Point   point
	Private uint32
}

func Listen(button int) (<-chan struct{}, func(), error) {
	if button < 3 || button > 5 {
		return nil, nil, fmt.Errorf("unsupported mouse button %d", button)
	}

	events := make(chan struct{}, 1)
	ready := make(chan error, 1)
	var stopOnce sync.Once
	var threadID uint32

	go func() {
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()
		id, _, _ := procGetThreadID.Call()
		threadID = uint32(id)
		targetPressed := false

		callback := syscall.NewCallback(func(code int, wParam, lParam uintptr) uintptr {
			if code == hcAction {
				matched, pressed := matchEvent(button, uint32(wParam), targetPressed)
				targetPressed = pressed
				if matched && isButtonDown(uint32(wParam)) {
					select {
					case events <- struct{}{}:
					default:
					}
				}
				if matched {
					return 1
				}
			}
			next, _, _ := procCallNextHook.Call(0, uintptr(code), wParam, lParam)
			return next
		})
		hook, _, callErr := procSetWindowsHook.Call(
			whMouseLL,
			callback,
			0,
			0,
		)
		if hook == 0 {
			ready <- fmt.Errorf("install Windows mouse hook: %w", callErr)
			close(events)
			return
		}
		ready <- nil

		var msg message
		for {
			result, _, _ := procGetMessage.Call(
				uintptr(unsafe.Pointer(&msg)),
				0,
				0,
				0,
			)
			if int32(result) <= 0 {
				break
			}
		}
		_, _, _ = procUnhookHook.Call(hook)
		close(events)
		runtime.KeepAlive(callback)
	}()

	if err := <-ready; err != nil {
		return nil, nil, err
	}
	stop := func() {
		stopOnce.Do(func() {
			_, _, _ = procPostThreadMsg.Call(uintptr(threadID), wmQuit, 0, 0)
		})
	}
	return events, stop, nil
}

func matchEvent(button int, message uint32, targetPressed bool) (matched, pressed bool) {
	switch button {
	case 3:
		return message == wmMButtonDown || message == wmMButtonUp, message == wmMButtonDown
	case 4, 5:
		virtualKey := uintptr(button + 1) // Mouse 4/5 map to VK_XBUTTON1/2 ($05/$06).
		switch message {
		case wmXButtonDown:
			if keyIsDown(virtualKey) {
				return true, true
			}
		case wmXButtonUp:
			if targetPressed && !keyIsDown(virtualKey) {
				return true, false
			}
		}
	}
	return false, targetPressed
}

func isButtonDown(message uint32) bool {
	return message == wmMButtonDown || message == wmXButtonDown
}

func keyIsDown(virtualKey uintptr) bool {
	state, _, _ := procGetAsyncKey.Call(virtualKey)
	return int16(state) < 0
}
