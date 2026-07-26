//go:build desktop && windows

package hud

import (
	"fmt"
	"runtime"
	"sync"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

const (
	wmDestroy        = 0x0002
	wmPaint          = 0x000F
	wmClose          = 0x0010
	wmEraseBkgnd     = 0x0014
	wmMouseActivate  = 0x0021
	wmNCHitTest      = 0x0084
	wmTimer          = 0x0113
	wmLButtonUp      = 0x0202
	wmAppStatus      = 0x8001
	wmAppLevel       = 0x8002
	wmAppError       = 0x8003
	wsPopup          = 0x80000000
	wsExToolWindow   = 0x00000080
	wsExNoActivate   = 0x08000000
	htClient         = 1
	htCaption        = 2
	maNoActivate     = 3
	swHide           = 0
	swShowNoActivate = 4
	swpNoSize        = 0x0001
	swpNoMove        = 0x0002
	swpNoActivate    = 0x0010
	swpShowWindow    = 0x0040
	colorWindow      = 5
	transparentMode  = 1
	defaultGUIFont   = 17
	idcArrow         = 32512
	hudWidth         = 268
	hudHeight        = 68
	statusIdle       = 0
	statusRecording  = 1
	statusProcessing = 2
	statusError      = 3
	errorTimerID     = 1
	pinHitWidth      = 48
	hwndTopmost      = ^uintptr(0)
	hwndNotTopmost   = ^uintptr(1)
)

var (
	user32              = windows.NewLazySystemDLL("user32.dll")
	gdi32               = windows.NewLazySystemDLL("gdi32.dll")
	kernel32            = windows.NewLazySystemDLL("kernel32.dll")
	procRegisterClass   = user32.NewProc("RegisterClassExW")
	procCreateWindow    = user32.NewProc("CreateWindowExW")
	procDefWindowProc   = user32.NewProc("DefWindowProcW")
	procDestroyWindow   = user32.NewProc("DestroyWindow")
	procGetMessage      = user32.NewProc("GetMessageW")
	procTranslateMsg    = user32.NewProc("TranslateMessage")
	procDispatchMsg     = user32.NewProc("DispatchMessageW")
	procPostMessage     = user32.NewProc("PostMessageW")
	procPostQuit        = user32.NewProc("PostQuitMessage")
	procShowWindow      = user32.NewProc("ShowWindow")
	procSetWindowPos    = user32.NewProc("SetWindowPos")
	procGetForeground   = user32.NewProc("GetForegroundWindow")
	procGetWindowRect   = user32.NewProc("GetWindowRect")
	procInvalidateRect  = user32.NewProc("InvalidateRect")
	procBeginPaint      = user32.NewProc("BeginPaint")
	procEndPaint        = user32.NewProc("EndPaint")
	procFillRect        = user32.NewProc("FillRect")
	procLoadCursor      = user32.NewProc("LoadCursorW")
	procSetWindowRgn    = user32.NewProc("SetWindowRgn")
	procSetTimer        = user32.NewProc("SetTimer")
	procKillTimer       = user32.NewProc("KillTimer")
	procGetModuleHandle = kernel32.NewProc("GetModuleHandleW")
	procCreateBrush     = gdi32.NewProc("CreateSolidBrush")
	procDeleteObject    = gdi32.NewProc("DeleteObject")
	procCreateRoundRgn  = gdi32.NewProc("CreateRoundRectRgn")
	procSetBkMode       = gdi32.NewProc("SetBkMode")
	procSetTextColor    = gdi32.NewProc("SetTextColor")
	procGetStockObject  = gdi32.NewProc("GetStockObject")
	procSelectObject    = gdi32.NewProc("SelectObject")
	procTextOut         = gdi32.NewProc("TextOutW")
	windowStates        sync.Map
	windowCallback      = syscall.NewCallback(windowProc)
)

type Overlay struct {
	window uintptr
}

type windowState struct {
	status     int
	pinned     bool
	positioned bool
	history    [12]float32
}

type point struct {
	X int32
	Y int32
}

type rect struct {
	Left   int32
	Top    int32
	Right  int32
	Bottom int32
}

type windowClass struct {
	Size        uint32
	Style       uint32
	WindowProc  uintptr
	ClassExtra  int32
	WindowExtra int32
	Instance    uintptr
	Icon        uintptr
	Cursor      uintptr
	Background  uintptr
	MenuName    *uint16
	ClassName   *uint16
	IconSmall   uintptr
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

type paint struct {
	Context   uintptr
	Erase     int32
	PaintRect rect
	Restore   int32
	IncUpdate int32
	Reserved  [32]byte
}

func New() (*Overlay, error) {
	ready := make(chan struct {
		window uintptr
		err    error
	}, 1)
	go runWindow(ready)
	result := <-ready
	if result.err != nil {
		return nil, result.err
	}
	return &Overlay{window: result.window}, nil
}

func (o *Overlay) SetStatus(status string) {
	if o == nil || o.window == 0 {
		return
	}
	value := statusIdle
	switch status {
	case "recording":
		value = statusRecording
	case "transcribing":
		value = statusProcessing
	}
	_, _, _ = procPostMessage.Call(o.window, wmAppStatus, uintptr(value), 0)
}

func (o *Overlay) SetLevel(level float32) {
	if o == nil || o.window == 0 {
		return
	}
	if level < 0 {
		level = 0
	}
	if level > 1 {
		level = 1
	}
	_, _, _ = procPostMessage.Call(o.window, wmAppLevel, uintptr(level*1000), 0)
}

func (o *Overlay) SetError() {
	if o == nil || o.window == 0 {
		return
	}
	_, _, _ = procPostMessage.Call(o.window, wmAppError, 0, 0)
}

func (o *Overlay) Close() {
	if o != nil && o.window != 0 {
		_, _, _ = procPostMessage.Call(o.window, wmClose, 0, 0)
		o.window = 0
	}
}

func runWindow(ready chan<- struct {
	window uintptr
	err    error
}) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	className, _ := syscall.UTF16PtrFromString("VoxSteerRecordingHUD")
	instance, _, _ := procGetModuleHandle.Call(0)
	cursor, _, _ := procLoadCursor.Call(0, idcArrow)
	class := windowClass{
		Size:       uint32(unsafe.Sizeof(windowClass{})),
		WindowProc: windowCallback,
		Instance:   instance,
		Cursor:     cursor,
		Background: colorWindow + 1,
		ClassName:  className,
	}
	atom, _, registerErr := procRegisterClass.Call(uintptr(unsafe.Pointer(&class)))
	if atom == 0 {
		ready <- struct {
			window uintptr
			err    error
		}{err: fmt.Errorf("register recording HUD: %w", registerErr)}
		return
	}

	title, _ := syscall.UTF16PtrFromString("VoxSteer")
	window, _, createErr := procCreateWindow.Call(
		wsExToolWindow|wsExNoActivate,
		uintptr(unsafe.Pointer(className)),
		uintptr(unsafe.Pointer(title)),
		wsPopup,
		0,
		0,
		hudWidth,
		hudHeight,
		0,
		0,
		instance,
		0,
	)
	if window == 0 {
		ready <- struct {
			window uintptr
			err    error
		}{err: fmt.Errorf("create recording HUD: %w", createErr)}
		return
	}
	windowStates.Store(window, &windowState{})
	region, _, _ := procCreateRoundRgn.Call(0, 0, hudWidth+1, hudHeight+1, 18, 18)
	_, _, _ = procSetWindowRgn.Call(window, region, 1)
	ready <- struct {
		window uintptr
		err    error
	}{window: window}

	var msg message
	for {
		result, _, _ := procGetMessage.Call(uintptr(unsafe.Pointer(&msg)), 0, 0, 0)
		if int32(result) <= 0 {
			break
		}
		_, _, _ = procTranslateMsg.Call(uintptr(unsafe.Pointer(&msg)))
		_, _, _ = procDispatchMsg.Call(uintptr(unsafe.Pointer(&msg)))
	}
}

func windowProc(window uintptr, message uint32, wParam, lParam uintptr) uintptr {
	value, ok := windowStates.Load(window)
	if !ok && message != wmDestroy {
		result, _, _ := procDefWindowProc.Call(window, uintptr(message), wParam, lParam)
		return result
	}
	state, _ := value.(*windowState)

	switch message {
	case wmAppStatus:
		_, _, _ = procKillTimer.Call(window, errorTimerID)
		state.status = int(wParam)
		if state.status == statusRecording {
			state.history = [12]float32{}
			positionAndShow(window, state)
		} else if state.status == statusIdle && !state.pinned {
			applyPinState(window, false)
			_, _, _ = procShowWindow.Call(window, swHide)
		}
		invalidate(window)
		return 0
	case wmAppError:
		state.status = statusError
		positionAndShow(window, state)
		_, _, _ = procSetTimer.Call(window, errorTimerID, 5000, 0)
		invalidate(window)
		return 0
	case wmAppLevel:
		level := float32(wParam) / 1000
		copy(state.history[:len(state.history)-1], state.history[1:])
		state.history[len(state.history)-1] = level
		invalidate(window)
		return 0
	case wmLButtonUp:
		x := int(int16(lParam & 0xffff))
		if x >= hudWidth-pinHitWidth {
			state.pinned = !state.pinned
			applyPinState(window, state.pinned || state.status != statusIdle)
			if !state.pinned && state.status == statusIdle {
				_, _, _ = procShowWindow.Call(window, swHide)
			}
			invalidate(window)
		}
		return 0
	case wmNCHitTest:
		screenX := int32(int16(lParam & 0xffff))
		var bounds rect
		ok, _, _ := procGetWindowRect.Call(window, uintptr(unsafe.Pointer(&bounds)))
		if ok != 0 && screenX >= bounds.Right-pinHitWidth {
			return htClient
		}
		return htCaption
	case wmMouseActivate:
		return maNoActivate
	case wmTimer:
		if wParam == errorTimerID {
			_, _, _ = procKillTimer.Call(window, errorTimerID)
			state.status = statusIdle
			if state.pinned {
				invalidate(window)
			} else {
				applyPinState(window, false)
				_, _, _ = procShowWindow.Call(window, swHide)
			}
		}
		return 0
	case wmPaint:
		paintWindow(window, state)
		return 0
	case wmEraseBkgnd:
		return 1
	case wmClose:
		_, _, _ = procDestroyWindow.Call(window)
		return 0
	case wmDestroy:
		windowStates.Delete(window)
		_, _, _ = procPostQuit.Call(0)
		return 0
	}

	result, _, _ := procDefWindowProc.Call(window, uintptr(message), wParam, lParam)
	return result
}

func positionAndShow(window uintptr, state *windowState) {
	x, y := int32(0), int32(0)
	flags := uintptr(swpNoActivate | swpShowWindow)
	if state.positioned {
		flags |= swpNoMove
	} else {
		foreground, _, _ := procGetForeground.Call()
		var target rect
		if foreground != 0 {
			ok, _, _ := procGetWindowRect.Call(foreground, uintptr(unsafe.Pointer(&target)))
			if ok == 0 {
				target = rect{Left: 0, Top: 0, Right: 1280, Bottom: 720}
			}
		} else {
			target = rect{Left: 0, Top: 0, Right: 1280, Bottom: 720}
		}
		x = target.Left + (target.Right-target.Left-hudWidth)/2
		y = target.Top + 28
		state.positioned = true
	}
	_, _, _ = procSetWindowPos.Call(
		window,
		hwndTopmost,
		uintptr(x),
		uintptr(y),
		hudWidth,
		hudHeight,
		swpNoActivate|swpShowWindow,
	)
	_, _, _ = procShowWindow.Call(window, swShowNoActivate)
}

func applyPinState(window uintptr, pinned bool) {
	insertAfter := hwndNotTopmost
	if pinned {
		insertAfter = hwndTopmost
	}
	_, _, _ = procSetWindowPos.Call(
		window,
		insertAfter,
		0,
		0,
		0,
		0,
		swpNoMove|swpNoSize|swpNoActivate,
	)
}

func invalidate(window uintptr) {
	_, _, _ = procInvalidateRect.Call(window, 0, 0)
}

func paintWindow(window uintptr, state *windowState) {
	var painting paint
	context, _, _ := procBeginPaint.Call(window, uintptr(unsafe.Pointer(&painting)))
	if context == 0 {
		return
	}
	defer procEndPaint.Call(window, uintptr(unsafe.Pointer(&painting)))

	background, _, _ := procCreateBrush.Call(rgb(13, 22, 38))
	full := rect{Right: hudWidth, Bottom: hudHeight}
	_, _, _ = procFillRect.Call(context, uintptr(unsafe.Pointer(&full)), background)
	_, _, _ = procDeleteObject.Call(background)

	font, _, _ := procGetStockObject.Call(defaultGUIFont)
	oldFont, _, _ := procSelectObject.Call(context, font)
	defer procSelectObject.Call(context, oldFont)
	_, _, _ = procSetBkMode.Call(context, transparentMode)

	label := "VoxSteer ready"
	labelColor := rgb(185, 198, 218)
	switch state.status {
	case statusRecording:
		label = "Listening"
		labelColor = rgb(42, 226, 231)
	case statusProcessing:
		label = "Transcribing..."
		labelColor = rgb(116, 205, 255)
	case statusError:
		label = "Error - see log"
		labelColor = rgb(251, 146, 60)
	}
	_, _, _ = procSetTextColor.Call(context, labelColor)
	textOut(context, 16, 26, label)

	waveBrush, _, _ := procCreateBrush.Call(rgb(34, 211, 238))
	defer procDeleteObject.Call(waveBrush)
	for i, value := range state.history {
		if state.status == statusProcessing {
			value = float32((i%4)+2) / 6
		} else if state.status == statusError {
			value = 0.18
		} else if state.status == statusIdle {
			value = 0.12
		}
		height := int32(4 + value*40)
		bar := rect{
			Left:   int32(100 + i*9),
			Top:    (hudHeight - height) / 2,
			Right:  int32(105 + i*9),
			Bottom: (hudHeight + height) / 2,
		}
		_, _, _ = procFillRect.Call(context, uintptr(unsafe.Pointer(&bar)), waveBrush)
	}

	if state.pinned {
		_, _, _ = procSetTextColor.Call(context, rgb(34, 211, 238))
		textOut(context, hudWidth-38, 26, "PIN")
	} else {
		_, _, _ = procSetTextColor.Call(context, rgb(100, 116, 139))
		textOut(context, hudWidth-38, 26, "pin")
	}
}

func textOut(context uintptr, x, y int, text string) {
	encoded, _ := syscall.UTF16FromString(text)
	_, _, _ = procTextOut.Call(
		context,
		uintptr(x),
		uintptr(y),
		uintptr(unsafe.Pointer(&encoded[0])),
		uintptr(len(encoded)-1),
	)
}

func rgb(red, green, blue byte) uintptr {
	return uintptr(red) | uintptr(green)<<8 | uintptr(blue)<<16
}
