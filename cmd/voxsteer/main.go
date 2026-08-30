//go:build desktop

package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync/atomic"

	"fyne.io/systray"
	"github.com/sawy3r/voxsteer/internal/app"
	"github.com/sawy3r/voxsteer/internal/config"
	"github.com/sawy3r/voxsteer/internal/desktop"
	"github.com/sawy3r/voxsteer/internal/hud"
	"github.com/sawy3r/voxsteer/internal/shortcut"
)

func main() {
	if len(os.Args) > 1 && os.Args[1] == "toggle" {
		if err := sendToggle(); err != nil {
			fmt.Fprintf(os.Stderr, "voxsteer toggle: %v\n", err)
			os.Exit(1)
		}
		return
	}

	logPath, closeLog, logErr := startLogging()
	if logErr == nil {
		defer closeLog()
		log.Printf("VoxSteer diagnostic log: %s", logPath)
	}
	if err := run(logPath); err != nil {
		desktop.ShowStartupError(err)
	}
}

func run(logPath string) error {
	cfg, configPath, err := config.Load()
	if err != nil {
		return err
	}
	engine, err := app.ConfiguredEngine(cfg)
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

	var runningController atomic.Pointer[app.Controller]
	stopControl, err := startControl(func() {
		if controller := runningController.Load(); controller != nil {
			if err := controller.Toggle(); err != nil {
				log.Print(err)
			}
		}
	})
	if err != nil {
		return err
	}

	systray.Run(func() {
		systray.SetIcon(trayIcon())
		systray.SetTitle("VoxSteer")
		systray.SetTooltip("VoxSteer — " + activation.Display)
		statusItem := systray.AddMenuItem("Status: idle", "Current dictation status")
		statusItem.Disable()
		overlay, overlayErr := hud.New()
		if overlayErr != nil {
			log.Printf("recording HUD unavailable: %v", overlayErr)
		}
		controller := app.New(
			desktop.Recorder{},
			engine,
			desktop.Injector{},
			func(status app.Status) {
				log.Printf("status: %s", status)
				statusItem.SetTitle("Status: " + string(status))
				overlay.SetStatus(string(status))
			},
			overlay.SetLevel,
			func(err error) {
				log.Printf("dictation failed: %v", err)
				statusItem.SetTitle("Error: see VoxSteer log")
				overlay.SetError()
			},
		)
		runningController.Store(controller)
		toggleItem := systray.AddMenuItem("Start / stop dictation", activation.Display)
		systray.AddSeparator()
		logItem := systray.AddMenuItem("Open diagnostic log", logPath)
		quitItem := systray.AddMenuItem("Quit", "Quit VoxSteer")

		stopActivation := configureActivation(activation, controller)
		go func() {
			for {
				select {
				case <-toggleItem.ClickedCh:
					if err := controller.Toggle(); err != nil {
						log.Print(err)
					}
				case <-logItem.ClickedCh:
					if err := openPath(logPath); err != nil {
						log.Printf("open diagnostic log: %v", err)
					}
				case <-quitItem.ClickedCh:
					stopActivation()
					stopControl()
					overlay.Close()
					systray.Quit()
					return
				}
			}
		}()
		log.Printf("ready: %s", desktop.InjectionRequirements())
	}, func() {})
	return nil
}

func startLogging() (string, func(), error) {
	configDir, err := os.UserConfigDir()
	if err != nil {
		return "", func() {}, err
	}
	logDir := filepath.Join(configDir, "VoxSteer")
	if err := os.MkdirAll(logDir, 0o700); err != nil {
		return "", func() {}, err
	}
	logPath := filepath.Join(logDir, "voxsteer.log")
	file, err := os.OpenFile(logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return "", func() {}, err
	}
	log.SetOutput(file)
	return logPath, func() { _ = file.Close() }, nil
}

