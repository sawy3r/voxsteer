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
	"net/url"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"time"
	"unicode"

	"github.com/sawy3r/voxsteer/internal/app"
	"github.com/sawy3r/voxsteer/internal/config"
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

var settingsMu sync.Mutex

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
		LoadSettings: loadSettings,
		SaveSettings: saveSettings,
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
	if err := validateConfig(cfg); err != nil {
		return nil, status, err
	}
	engine, err := app.ConfiguredEngine(cfg)
	if cloud, ok := engine.(*transcriber.Cloud); ok {
		cloud.Client = secureCloudClient()
	}
	return engine, status, err
}

func loadSettings() (webapp.Settings, error) {
	cfg, _, err := config.Load()
	if err != nil {
		return webapp.Settings{}, err
	}
	return webapp.Settings{
		Engine:         cfg.Engine,
		Language:       cfg.Language,
		CloudEndpoint:  cfg.Cloud.Endpoint,
		CloudModel:     cfg.Cloud.Model,
		HasCloudAPIKey: cfg.Cloud.APIKey != "",
		LocalBinary:    cfg.Local.Binary,
		LocalModel:     cfg.Local.Model,
	}, nil
}

func saveSettings(settings webapp.Settings) error {
	settingsMu.Lock()
	defer settingsMu.Unlock()

	for _, name := range []string{
		"VTT_CONFIG",
		"VTT_ENGINE",
		"VTT_LANGUAGE",
		"VTT_WHISPER_BINARY",
		"VTT_WHISPER_MODEL",
		"VTT_CLOUD_ENDPOINT",
		"VTT_CLOUD_API_KEY",
		"VTT_CLOUD_MODEL",
		"VTT_SHORTCUT",
	} {
		if _, set := os.LookupEnv(name); set {
			return fmt.Errorf("settings are controlled by %s; remove that environment override before saving", name)
		}
	}
	cfg, loadedPath, err := config.Load()
	if err != nil {
		return err
	}
	userPath, err := config.UserPath()
	if err != nil {
		return err
	}
	if loadedPath != "" && loadedPath != userPath {
		return fmt.Errorf("settings are controlled by %s; update that file directly", loadedPath)
	}

	cfg.Engine = strings.TrimSpace(settings.Engine)
	cfg.Language = strings.TrimSpace(settings.Language)
	cfg.Cloud.Endpoint = strings.TrimSpace(settings.CloudEndpoint)
	cfg.Cloud.Model = strings.TrimSpace(settings.CloudModel)
	cfg.Local.Binary = strings.TrimSpace(settings.LocalBinary)
	cfg.Local.Model = strings.TrimSpace(settings.LocalModel)
	if settings.CloudAPIKey != nil {
		cfg.Cloud.APIKey = strings.TrimSpace(*settings.CloudAPIKey)
	}
	if cfg.Cloud.Model == "" {
		cfg.Cloud.Model = "whisper-1"
	}
	if err := validateConfig(cfg); err != nil {
		return err
	}
	_, err = config.SaveUser(cfg)
	return err
}

func validateConfig(cfg config.Config) error {
	if len(cfg.Language) > 32 || containsControl(cfg.Language) {
		return fmt.Errorf("language must be at most 32 characters and contain no control characters")
	}
	if len(cfg.Cloud.APIKey) > 8192 {
		return fmt.Errorf("cloud API key exceeds 8192 characters")
	}
	switch cfg.Engine {
	case "cloud":
		if len(cfg.Cloud.Endpoint) > 4096 {
			return fmt.Errorf("cloud endpoint exceeds 4096 characters")
		}
		endpoint, err := url.ParseRequestURI(cfg.Cloud.Endpoint)
		if err != nil {
			return fmt.Errorf("cloud endpoint must be a valid HTTPS URL")
		}
		if err := validateCloudURL(endpoint); err != nil {
			return err
		}
		if len(cfg.Cloud.Model) > 256 || containsControl(cfg.Cloud.Model) {
			return fmt.Errorf("cloud model must be at most 256 characters and contain no control characters")
		}
	case "local":
		if cfg.Local.Model == "" {
			return fmt.Errorf("local Whisper model path is required")
		}
		if len(cfg.Local.Model) > 4096 || len(cfg.Local.Binary) > 4096 {
			return fmt.Errorf("local Whisper paths exceed 4096 characters")
		}
		modelInfo, err := os.Stat(cfg.Local.Model)
		if err != nil || !modelInfo.Mode().IsRegular() {
			return fmt.Errorf("local Whisper model is not a readable file")
		}
		binary := cfg.Local.Binary
		if binary == "" {
			binary = "whisper-cli"
		}
		if _, err := exec.LookPath(binary); err != nil {
			return fmt.Errorf("local whisper-cli executable was not found")
		}
	default:
		return fmt.Errorf("transcription engine must be cloud or local")
	}
	return nil
}

func containsControl(value string) bool {
	return strings.IndexFunc(value, unicode.IsControl) >= 0
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

func validateCloudURL(endpoint *url.URL) error {
	if endpoint == nil || endpoint.Host == "" || endpoint.User != nil {
		return fmt.Errorf("cloud endpoint must be a valid URL without embedded credentials")
	}
	if endpoint.Scheme == "https" {
		return nil
	}
	if endpoint.Scheme != "http" {
		return fmt.Errorf("cloud endpoint must use HTTPS")
	}
	host := endpoint.Hostname()
	if strings.EqualFold(host, "localhost") {
		return nil
	}
	ip := net.ParseIP(host)
	if ip == nil || !ip.IsLoopback() {
		return fmt.Errorf("cloud endpoint must use HTTPS unless it is on loopback")
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
			if err := validateCloudURL(request.URL); err != nil {
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
