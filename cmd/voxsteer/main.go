//go:build desktop

package main

import (
	"fmt"
	"log"

	"github.com/getlantern/systray"
	"github.com/sawy3r/voxsteer/internal/app"
	"github.com/sawy3r/voxsteer/internal/config"
	"github.com/sawy3r/voxsteer/internal/desktop"
	"github.com/sawy3r/voxsteer/internal/shortcut"
	"github.com/sawy3r/voxsteer/internal/transcriber"
	"golang.design/x/hotkey"
)

func main() {
	if err := run(); err != nil {
		desktop.ShowStartupError(err)
	}
}

func run() error {
	cfg, configPath, err := config.Load()
	if err != nil {
		return err
	}
	engine, err := configuredEngine(cfg)
	if err != nil {
		if configPath == "" {
			return fmt.Errorf("%w; create config.json beside the executable or set VTT_CONFIG", err)
		}
		return fmt.Errorf("%w (configuration: %s)", err, configPath)
	}
	activation, err := shortcut.Parse(cfg.Shortcut)
	if err != nil {
		return fmt.Errorf("invalid activation shortcut: %w", err)
	}

	systray.Run(func() {
		systray.SetTitle("VoxSteer")
		systray.SetTooltip("VoxSteer — " + activation.Display)
		statusItem := systray.AddMenuItem("Status: idle", "Current dictation status")
		statusItem.Disable()
		controller := app.New(
			desktop.Recorder{},
			engine,
			desktop.Injector{},
			func(status app.Status) {
				log.Printf("status: %s", status)
				statusItem.SetTitle("Status: " + string(status))
			},
			func(err error) { log.Printf("dictation failed: %v", err) },
		)
		toggleItem := systray.AddMenuItem("Start / stop dictation", activation.Display)
		systray.AddSeparator()
		quitItem := systray.AddMenuItem("Quit", "Quit VoxSteer")

		hk, hotkeyErr := configuredHotkey(activation)
		if hotkeyErr != nil {
			log.Printf("configure %s: %v", activation.Display, hotkeyErr)
		} else if hk != nil {
			if err := hk.Register(); err != nil {
				log.Printf("register %s: %v; the tray menu remains available", activation.Display, err)
				hk = nil
			} else {
				go func() {
					for range hk.Keydown() {
						if err := controller.Toggle(); err != nil {
							log.Print(err)
						}
					}
				}()
			}
		}
		go func() {
			for {
				select {
				case <-toggleItem.ClickedCh:
					if err := controller.Toggle(); err != nil {
						log.Print(err)
					}
				case <-quitItem.ClickedCh:
					if hk != nil {
						_ = hk.Unregister()
					}
					systray.Quit()
					return
				}
			}
		}()
		log.Printf("ready: %s", desktop.InjectionRequirements())
	}, func() {})
	return nil
}

func configuredHotkey(spec shortcut.Spec) (*hotkey.Hotkey, error) {
	if spec.Key == "" {
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

func configuredEngine(cfg config.Config) (transcriber.Transcriber, error) {
	switch cfg.Engine {
	case "cloud":
		if cfg.Cloud.Endpoint == "" {
			return nil, fmt.Errorf("VTT_CLOUD_ENDPOINT is required for the cloud engine")
		}
		return &transcriber.Cloud{
			Endpoint: cfg.Cloud.Endpoint,
			APIKey:   cfg.Cloud.APIKey,
			Model:    cfg.Cloud.Model,
			Language: cfg.Language,
		}, nil
	case "local":
		if cfg.Local.Model == "" {
			return nil, fmt.Errorf("VTT_WHISPER_MODEL is required for the local engine")
		}
		return &transcriber.LocalWhisper{
			Binary:   cfg.Local.Binary,
			Model:    cfg.Local.Model,
			Language: cfg.Language,
		}, nil
	default:
		return nil, fmt.Errorf("VTT_ENGINE must be cloud or local")
	}
}
