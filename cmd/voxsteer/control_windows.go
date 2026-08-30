//go:build desktop && windows

package main

import "fmt"

func startControl(func()) (func(), error) {
	return func() {}, nil
}

func sendToggle() error {
	return fmt.Errorf("the toggle command is not supported on Windows; use the configured shortcut or the tray menu")
}
