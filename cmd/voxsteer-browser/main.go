package main

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"sync"
	"syscall"
	"time"

	"github.com/sawy3r/voxsteer/internal/app"
	"github.com/sawy3r/voxsteer/internal/config"
	"github.com/sawy3r/voxsteer/internal/settings"
	"github.com/sawy3r/voxsteer/internal/transcriber"
	"github.com/sawy3r/voxsteer/internal/webapp"
)

const defaultAddress = "127.0.0.1:47823"

// maxRecordingSeconds bounds a single browser-mode recording. Raising it
// increases per-request memory (roughly 4 bytes/sample at 16 kHz mono float32
// plus a 2-byte-per-sample WAV encode) and must stay comfortably under
// transcriptionTimeout and the HTTP server's WriteTimeout below.
const maxRecordingSeconds = 10 * 60

// transcriptionTimeout bounds a single transcription call (cloud request or
// local whisper-cli process) for the longest permitted recording.
const transcriptionTimeout = 15 * time.Minute

var version = "dev"

func main() {
	os.Exit(realMain())
}

func realMain() int {
	address := flag.String("address", defaultAddress, "loopback address for the browser interface")
	noOpen := flag.Bool("no-open", false, "print the session URL instead of opening a browser")
	idleTimeout := flag.Duration("idle-timeout", 30*time.Minute, "exit after this long without browser activity; zero disables")
	flag.Parse()

	logPath, logger, closeLog, err := startLogging()
	if err != nil {
		log.Print(err)
		if !*noOpen {
			showStartupError(err)
		}
		return 1
	}
	defer closeLog()
	if err := run(*address, *noOpen, *idleTimeout, logPath, logger); err != nil {
		logger.Printf("VoxSteer browser stopped: %v", err)
		log.Print(err)
		if !*noOpen {
			showStartupError(err)
		}
		return 1
	}
	return 0
}

func run(address string, noOpen bool, idleTimeout time.Duration, logPath string, logger *log.Logger) error {
	if idleTimeout < 0 {
		return fmt.Errorf("idle timeout cannot be negative")
	}
	if err := validateLoopbackAddress(address); err != nil {
		return err
	}
	listener, err := net.Listen("tcp4", address)
	if err != nil {
		return fmt.Errorf("listen on %s: %w", address, err)
	}
	defer listener.Close()
	host := listener.Addr().String()
	if err := validateLoopbackAddress(host); err != nil {
		return err
	}
	origin := "http://" + host
	token, err := sessionToken()
	if err != nil {
		return err
	}

	quit := make(chan struct{})
	var quitOnce sync.Once
	requestQuit := func() { quitOnce.Do(func() { close(quit) }) }
	activity := make(chan struct{}, 1)

	ui, err := webapp.New(webapp.Options{
		Host:         host,
		Origin:       origin,
		Token:        token,
		Engine:       configuredEngine,
		LoadSettings: settings.Load,
		SaveSettings: settings.Save,
		OpenLog:      func() error { return openPath(logPath) },
		Quit:         requestQuit,
		OnActivity: func() {
			select {
			case activity <- struct{}{}:
			default:
			}
		},
		Logf:                 logger.Printf,
		MaxRecordingSeconds:  maxRecordingSeconds,
		TranscriptionTimeout: transcriptionTimeout,
	})
	if err != nil {
		return err
	}

	httpServer := &http.Server{
		Handler:           ui.Handler(),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       60 * time.Second,
		WriteTimeout:      transcriptionTimeout + 5*time.Minute,
		IdleTimeout:       60 * time.Second,
		MaxHeaderBytes:    16 << 10,
	}
	serveErrors := make(chan error, 1)
	go func() {
		serveErrors <- httpServer.Serve(listener)
	}()

	launchURL := origin + "/#session=" + token
	if noOpen {
		fmt.Println(launchURL)
	} else if err := openBrowser(launchURL); err != nil {
		shutdownServer(httpServer)
		return fmt.Errorf("open browser: %w", err)
	}
	logger.Printf("VoxSteer browser %s ready on %s", version, origin)

	signals := make(chan os.Signal, 1)
	signal.Notify(signals, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(signals)

	var idle <-chan time.Time
	var idleTimer *time.Timer
	if idleTimeout > 0 {
		idleTimer = time.NewTimer(idleTimeout)
		defer idleTimer.Stop()
		idle = idleTimer.C
	}
	for {
		select {
		case <-quit:
			logger.Print("browser requested shutdown")
			return shutdownServer(httpServer)
		case signalValue := <-signals:
			logger.Printf("received signal %s", signalValue)
			return shutdownServer(httpServer)
		case err := <-serveErrors:
			if errors.Is(err, http.ErrServerClosed) {
				return nil
			}
			return fmt.Errorf("browser server: %w", err)
		case <-activity:
			if idleTimer != nil {
				if !idleTimer.Stop() {
					select {
					case <-idleTimer.C:
					default:
					}
				}
				idleTimer.Reset(idleTimeout)
			}
		case <-idle:
			logger.Printf("browser session idle for %s; shutting down", idleTimeout)
			return shutdownServer(httpServer)
		}
	}
}

func configuredEngine() (transcriber.Transcriber, webapp.EngineStatus, error) {
	cfg, _, err := config.Load()
	status := webapp.EngineStatus{Engine: cfg.Engine, Language: cfg.Language}
	if err != nil {
		return nil, status, err
	}
	if err := settings.Validate(cfg); err != nil {
		return nil, status, err
	}
	engine, err := app.ConfiguredEngine(cfg)
	if cloud, ok := engine.(*transcriber.Cloud); ok {
		cloud.Client = secureCloudClient()
	}
	return engine, status, err
}

func validateLoopbackAddress(address string) error {
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		return fmt.Errorf("invalid browser address %q: %w", address, err)
	}
	ip := net.ParseIP(host)
	if ip == nil || !ip.Equal(net.IPv4(127, 0, 0, 1)) {
		return fmt.Errorf("browser address must bind exactly to IPv4 loopback")
	}
	return nil
}

func secureCloudClient() *http.Client {
	return &http.Client{
		// No Client-level Timeout: the per-request context deadline set by
		// webapp.Server (transcriptionTimeout) is authoritative and already
		// bounds cloud requests without duplicating that value here.
		CheckRedirect: func(request *http.Request, via []*http.Request) error {
			if len(via) >= 10 {
				return fmt.Errorf("too many transcription endpoint redirects")
			}
			if err := settings.ValidateCloudURL(request.URL); err != nil {
				return fmt.Errorf("refuse transcription endpoint redirect: %w", err)
			}
			return nil
		},
	}
}

func sessionToken() (string, error) {
	value := make([]byte, 32)
	if _, err := rand.Read(value); err != nil {
		return "", fmt.Errorf("generate browser session: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(value), nil
}

func startLogging() (string, *log.Logger, func(), error) {
	configDir, err := os.UserConfigDir()
	if err != nil {
		return "", nil, func() {}, err
	}
	logDir := filepath.Join(configDir, "VoxSteer")
	if err := os.MkdirAll(logDir, 0o700); err != nil {
		return "", nil, func() {}, err
	}
	if err := os.Chmod(logDir, 0o700); err != nil {
		return "", nil, func() {}, err
	}
	logPath := filepath.Join(logDir, "voxsteer-browser.log")
	if info, statErr := os.Stat(logPath); statErr == nil && info.Size() > 5<<20 {
		_ = os.Remove(logPath + ".1")
		if err := os.Rename(logPath, logPath+".1"); err != nil {
			return "", nil, func() {}, err
		}
	}
	file, err := os.OpenFile(logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return "", nil, func() {}, err
	}
	if err := file.Chmod(0o600); err != nil {
		_ = file.Close()
		return "", nil, func() {}, err
	}
	logger := log.New(file, "", log.LstdFlags|log.Lmicroseconds)
	return logPath, logger, func() { _ = file.Close() }, nil
}

func shutdownServer(server *http.Server) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := server.Shutdown(ctx); err != nil {
		_ = server.Close()
		return err
	}
	return nil
}

func openBrowser(target string) error {
	var command *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		command = exec.Command("/usr/bin/open", target)
	case "windows":
		command = exec.Command("rundll32", "url.dll,FileProtocolHandler", target)
	default:
		command = exec.Command("xdg-open", target)
	}
	return startAndReap(command)
}

func openPath(path string) error {
	var command *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		command = exec.Command("/usr/bin/open", path)
	case "windows":
		command = exec.Command("explorer", path)
	default:
		command = exec.Command("xdg-open", path)
	}
	return startAndReap(command)
}

func startAndReap(command *exec.Cmd) error {
	if err := command.Start(); err != nil {
		return err
	}
	go func() { _ = command.Wait() }()
	return nil
}
