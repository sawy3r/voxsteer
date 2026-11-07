//go:build desktop && !linux

package main

import (
	"fmt"
	"log"
	"sync"

	"github.com/sawy3r/voxsteer/internal/app"
	"github.com/sawy3r/voxsteer/internal/desktop"
	"github.com/sawy3r/voxsteer/internal/doubletap"
	"github.com/sawy3r/voxsteer/internal/mousebutton"
	"github.com/sawy3r/voxsteer/internal/shortcut"
	"golang.design/x/hotkey"
)

// configureActivation registers the shortcut, waiting first if the operating
// system has not yet allowed global input. macOS refuses the hotkey until
// Accessibility is granted, and the user may do that long after launch.
func configureActivation(spec shortcut.Spec, controller *app.Controller) func() {
	if spec.Disabled() || desktop.InputAccessGranted() {
		return registerActivation(spec, controller)
	}
	log.Printf("%s will activate once Accessibility access is granted", spec.Display)

	var mu sync.Mutex
	unregister := func() {}
	cancelled := make(chan struct{})
	go func() {
		if !desktop.WaitForInputAccess(cancelled) {
			return
		}
		registered := registerActivation(spec, controller)
		mu.Lock()
		defer mu.Unlock()
		select {
		case <-cancelled:
			registered()
		default:
			unregister = registered
			log.Printf("%s is active", spec.Display)
		}
	}()

	var once sync.Once
	return func() {
		once.Do(func() { close(cancelled) })
		mu.Lock()
		defer mu.Unlock()
		unregister()
	}
}

func registerActivation(spec shortcut.Spec, controller *app.Controller) func() {
	if spec.DoubleTapLeftCtrl {
		events, stop, err := doubletap.Listen()
		return forwardActivation(spec, controller, events, stop, err)
	}
	if spec.MouseButton != 0 {
		events, stop, err := mousebutton.Listen(spec.MouseButton)
		return forwardActivation(spec, controller, events, stop, err)
	}

	hk, err := configuredHotkey(spec)
	if err != nil {
		log.Printf("configure %s: %v", spec.Display, err)
		return func() {}
	}
	if hk == nil {
		return func() {}
	}
	if err := hk.Register(); err != nil {
		log.Printf("register %s: %v; the tray menu remains available", spec.Display, err)
		return func() {}
	}
	go func() {
		for range hk.Keydown() {
			if err := controller.Toggle(); err != nil {
				log.Print(err)
			}
		}
	}()
	return func() {
		_ = hk.Unregister()
	}
}

// forwardActivation toggles dictation for each event from a listener.
func forwardActivation(
	spec shortcut.Spec,
	controller *app.Controller,
	events <-chan struct{},
	stop func(),
	listenErr error,
) func() {
	if listenErr != nil {
		log.Printf("register %s: %v; the tray menu remains available", spec.Display, listenErr)
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

func configuredHotkey(spec shortcut.Spec) (*hotkey.Hotkey, error) {
	if spec.Key == "" || spec.MouseButton != 0 {
		return nil, nil
	}
	modifiers := make([]hotkey.Modifier, 0, 4)
	if spec.Ctrl {
		modifiers = append(modifiers, hotkey.ModCtrl)
	}
	if spec.Alt {
		modifiers = append(modifiers, platformAltModifier())
	}
	if spec.Shift {
		modifiers = append(modifiers, hotkey.ModShift)
	}
	if spec.Super {
		modifiers = append(modifiers, platformSuperModifier())
	}
	key, err := hotkeyKey(spec.Key)
	if err != nil {
		return nil, err
	}
	return hotkey.New(modifiers, key), nil
}

func hotkeyKey(key string) (hotkey.Key, error) {
	keys := map[string]hotkey.Key{
		"SPACE": hotkey.KeySpace,
		"A":     hotkey.KeyA, "B": hotkey.KeyB, "C": hotkey.KeyC,
		"D": hotkey.KeyD, "E": hotkey.KeyE, "F": hotkey.KeyF,
		"G": hotkey.KeyG, "H": hotkey.KeyH, "I": hotkey.KeyI,
		"J": hotkey.KeyJ, "K": hotkey.KeyK, "L": hotkey.KeyL,
		"M": hotkey.KeyM, "N": hotkey.KeyN, "O": hotkey.KeyO,
		"P": hotkey.KeyP, "Q": hotkey.KeyQ, "R": hotkey.KeyR,
		"S": hotkey.KeyS, "T": hotkey.KeyT, "U": hotkey.KeyU,
		"V": hotkey.KeyV, "W": hotkey.KeyW, "X": hotkey.KeyX,
		"Y": hotkey.KeyY, "Z": hotkey.KeyZ,
		"0": hotkey.Key0, "1": hotkey.Key1, "2": hotkey.Key2,
		"3": hotkey.Key3, "4": hotkey.Key4, "5": hotkey.Key5,
		"6": hotkey.Key6, "7": hotkey.Key7, "8": hotkey.Key8,
		"9":  hotkey.Key9,
		"F1": hotkey.KeyF1, "F2": hotkey.KeyF2, "F3": hotkey.KeyF3,
		"F4": hotkey.KeyF4, "F5": hotkey.KeyF5, "F6": hotkey.KeyF6,
		"F7": hotkey.KeyF7, "F8": hotkey.KeyF8, "F9": hotkey.KeyF9,
		"F10": hotkey.KeyF10, "F11": hotkey.KeyF11, "F12": hotkey.KeyF12,
	}
	value, ok := keys[key]
	if !ok {
		return 0, fmt.Errorf("unsupported shortcut key %q", key)
	}
	return value, nil
}
