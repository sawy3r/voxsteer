//go:build desktop

package main

import (
	"fmt"
	"log"

	"github.com/getlantern/systray"
	"github.com/sawy3r/voxsteer/internal/app"
	"github.com/sawy3r/voxsteer/internal/config"
	"github.com/sawy3r/voxsteer/internal/desktop"
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

	systray.Run(func() {
		systray.SetTitle("VoxSteer")
		systray.SetTooltip("VoxSteer — Ctrl+Shift+S")
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
		toggleItem := systray.AddMenuItem("Start / stop dictation", "Ctrl+Shift+S")
		systray.AddSeparator()
		quitItem := systray.AddMenuItem("Quit", "Quit VoxSteer")

		hk := hotkey.New([]hotkey.Modifier{hotkey.ModCtrl, hotkey.ModShift}, hotkey.KeyS)
		if err := hk.Register(); err != nil {
			log.Printf("register Ctrl+Shift+S: %v", err)
		} else {
			go func() {
				for range hk.Keydown() {
					if err := controller.Toggle(); err != nil {
						log.Print(err)
					}
				}
			}()
		}
		go func() {
			for {
				select {
				case <-toggleItem.ClickedCh:
					if err := controller.Toggle(); err != nil {
						log.Print(err)
					}
				case <-quitItem.ClickedCh:
					_ = hk.Unregister()
					systray.Quit()
					return
				}
			}
		}()
		log.Printf("ready: %s", desktop.InjectionRequirements())
	}, func() {})
	return nil
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
