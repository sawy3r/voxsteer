//go:build desktop

package main

import (
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"

	"fyne.io/systray"
	"github.com/sawy3r/voxsteer/internal/app"
	"github.com/sawy3r/voxsteer/internal/config"
	"github.com/sawy3r/voxsteer/internal/desktop"
	"github.com/sawy3r/voxsteer/internal/hud"
	"github.com/sawy3r/voxsteer/internal/settings"
	"github.com/sawy3r/voxsteer/internal/settingsui"
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
	engine := &app.SwitchableEngine{}
	needsSetup := false
	configured, err := app.ConfiguredEngine(cfg)
	switch {
	case err == nil:
		engine.Set(configured)
	case settingsui.Available:
		log.Printf("transcription is not set up: %v", err)
		needsSetup = true
	case configPath == "":
		location := "config.json beside the executable"
		if userPath, pathErr := config.UserPath(); pathErr == nil {
			location = userPath
		}
		return fmt.Errorf("%w; no configuration file was found — create %s (start from config.example.json) or set VTT_CONFIG", err, location)
	default:
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
				var notice *app.Notice
				if errors.As(err, &notice) {
					log.Printf("dictation: %v", err)
					statusItem.SetTitle(notice.Message)
					overlay.SetStatus(notice.Message)
					return
				}
				log.Printf("dictation failed: %v", err)
				statusItem.SetTitle("Error: see VoxSteer log")
				overlay.SetError()
			},
		)
		runningController.Store(controller)
		toggleItem := systray.AddMenuItem("Start / stop dictation", activation.Display)
		permissionItem := systray.AddMenuItem("", "Open System Settings")
		permissionItem.Hide()
		var settingsClicked <-chan struct{}
		if settingsui.Available {
			settingsClicked = systray.AddMenuItem("Settings…", "Change VoxSteer settings").ClickedCh
		}
		systray.AddSeparator()
		logItem := systray.AddMenuItem("Open diagnostic log", logPath)
		quitItem := systray.AddMenuItem("Quit", "Quit VoxSteer")

		desktop.RequestPermissions()
		go desktop.WatchPermissions(func(missing desktop.Permission) {
			if missing.Name == "" {
				log.Print("all permissions granted")
				permissionItem.Hide()
				return
			}
			log.Printf("waiting for %s permission", missing.Name)
			permissionItem.SetTitle("Allow " + missing.Name + " access…")
			permissionItem.Show()
		})
		var current runningActivation
		current.replace(func() func() { return configureActivation(activation, controller) })

		applySettings := func() {
			cfg, _, err := config.Load()
			if err != nil {
				log.Printf("apply settings: %v", err)
				return
			}
			if configured, err := app.ConfiguredEngine(cfg); err != nil {
				log.Printf("apply settings: %v", err)
			} else {
				engine.Set(configured)
			}
			spec, err := shortcut.Parse(cfg.Shortcut)
			if err != nil {
				log.Printf("apply settings: %v", err)
				return
			}
			current.replace(func() func() { return configureActivation(spec, controller) })
			systray.SetTooltip("VoxSteer — " + spec.Display)
			toggleItem.SetTooltip(spec.Display)
			log.Printf("settings applied: engine %s, activation %s", cfg.Engine, spec.Display)
		}
		openSettings := func() {
			settingsui.Open(settingsui.Options{
				Load: settings.Load,
				Save: settings.Save,
				// Replacing a shortcut waits on the main thread, which is
				// the thread saving runs on.
				OnSaved: func() { go applySettings() },
			})
		}
		if needsSetup {
			openSettings()
		}

		go func() {
			for {
				select {
				case <-toggleItem.ClickedCh:
					if err := controller.Toggle(); err != nil {
						log.Print(err)
					}
				case <-settingsClicked:
					openSettings()
				case <-permissionItem.ClickedCh:
					if url := desktop.MissingPermission().SettingsURL; url != "" {
						if err := openPath(url); err != nil {
							log.Printf("open permission settings: %v", err)
						}
					}
				case <-logItem.ClickedCh:
					if err := openPath(logPath); err != nil {
						log.Printf("open diagnostic log: %v", err)
					}
				case <-quitItem.ClickedCh:
					current.stop()
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

// runningActivation holds whichever activation is live, so saved settings can
// swap it without a restart.
type runningActivation struct {
	mu      sync.Mutex
	release func()
}

func (r *runningActivation) replace(start func() func()) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.release != nil {
		r.release()
	}
	r.release = start()
}

func (r *runningActivation) stop() {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.release != nil {
		r.release()
		r.release = nil
	}
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
	// Mirror the log to stderr so someone launching VoxSteer from a terminal
	// sees problems immediately instead of having to find the log file. The
	// file comes first: MultiWriter stops at the first failing writer, and
	// stderr may be closed when launched from a desktop entry.
	log.SetOutput(io.MultiWriter(file, os.Stderr))
	return logPath, func() { _ = file.Close() }, nil
}
