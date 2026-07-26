//go:build desktop && windows

package desktop

import (
	"fmt"
	"syscall"
	"unsafe"
)

var messageBox = syscall.NewLazyDLL("user32.dll").NewProc("MessageBoxW")

func ShowStartupError(err error) {
	title, _ := syscall.UTF16PtrFromString("VoxSteer")
	message, conversionErr := syscall.UTF16PtrFromString(fmt.Sprintf(
		"VoxSteer could not start:\r\n\r\n%v",
		err,
	))
	if conversionErr != nil {
		return
	}
	const mbIconError = 0x10
	_, _, _ = messageBox.Call(
		0,
		uintptr(unsafe.Pointer(message)),
		uintptr(unsafe.Pointer(title)),
		mbIconError,
	)
}
