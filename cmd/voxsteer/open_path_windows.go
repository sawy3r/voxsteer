//go:build windows

package main

import (
	"fmt"
	"syscall"
	"unsafe"
)

var shellExecute = syscall.NewLazyDLL("shell32.dll").NewProc("ShellExecuteW")

func openPath(path string) error {
	operation, _ := syscall.UTF16PtrFromString("open")
	target, _ := syscall.UTF16PtrFromString(path)
	result, _, callErr := shellExecute.Call(
		0,
		uintptr(unsafe.Pointer(operation)),
		uintptr(unsafe.Pointer(target)),
		0,
		0,
		1,
	)
	if result <= 32 {
		return fmt.Errorf("ShellExecuteW failed (%d): %w", result, callErr)
	}
	return nil
}
