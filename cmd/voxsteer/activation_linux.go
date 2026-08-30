//go:build desktop && linux

package main

import (
	"log"
	"os"

	"github.com/sawy3r/voxsteer/internal/activation"
	"github.com/sawy3r/voxsteer/internal/app"
	"github.com/sawy3r/voxsteer/internal/shortcut"
)

// configureActivation listens for the shortcut on the kernel input devices.
// evdev is the only mechanism that works on both Wayland and X11: X11 key
// grabs never fire while a native Wayland window has focus, and Wayland
// compositors do not offer global grabs at all.
func configureActivation(spec shortcut.Spec, controller *app.Controller) func() {
	if spec.Key == "" && spec.MouseButton == 0 {
		return func() {}
	}
	events, stop, err := activation.Listen(spec)
	if err != nil {
		executable, _ := os.Executable()
		log.Printf("register %s: %v", spec.Display, err)
		log.Printf("either add your user to the input group (sudo usermod -aG input $USER, then log out and back in) "+
			"or bind a system keyboard shortcut to %q in your desktop settings; the tray menu remains available",
			executable+" toggle")
		return func() {}
	}
	go func() {
		for range events {
			if err := controller.Toggle(); err != nil {
				log.Print(err)
			}
		}
	}()
	return stop
}
