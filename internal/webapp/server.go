package webapp

import (
	"context"
	"crypto/subtle"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net"
	"net/http"
	"net/url"
	"os/exec"
	"strings"
	"time"
	"unicode/utf8"

	appaudio "github.com/sawy3r/voxsteer/internal/audio"
	"github.com/sawy3r/voxsteer/internal/transcriber"
)

const (
	// DefaultMaxRecordingSeconds is used when Options.MaxRecordingSeconds is
	// left at its zero value.
	DefaultMaxRecordingSeconds = 600
	// MaxRecordingSecondsCeiling is the absolute upper bound New() will
	// accept for Options.MaxRecordingSeconds, regardless of caller input.
	MaxRecordingSecondsCeiling = 30 * 60
	maxSettingsBody            = 32 << 10
	maxTranscriptBytes         = 256 << 10
)

//go:embed assets/*
var assets embed.FS

type EngineStatus struct {
	Engine   string
	Language string
}

type EngineProvider func() (transcriber.Transcriber, EngineStatus, error)

type Settings struct {
	Engine         string  `json:"engine"`
	Language       string  `json:"language"`
	CloudEndpoint  string  `json:"cloud_endpoint"`
	CloudModel     string  `json:"cloud_model"`
	HasCloudAPIKey bool    `json:"has_cloud_api_key"`
	CloudAPIKey    *string `json:"cloud_api_key,omitempty"`
	LocalBinary    string  `json:"local_binary"`
	LocalModel     string  `json:"local_model"`
}

type Options struct {
	Host                 string
	Origin               string
	Token                string
	MaxRecordingSeconds  int
	TranscriptionTimeout time.Duration
	Engine               EngineProvider
	LoadSettings         func() (Settings, error)
	SaveSettings         func(Settings) error
	OpenLog              func() error
	Quit                 func()
	OnActivity           func()
	Logf                 func(string, ...any)
}

type Server struct {
	host                 string
	origin               string
	token                []byte
	maxRecordingSeconds  int
	maxSamples           int
	transcriptionTimeout time.Duration
	engine               EngineProvider
	loadSettings         func() (Settings, error)
	saveSettings         func(Settings) error
	openLog              func() error
	quit                 func()
	onActivity           func()
	logf                 func(string, ...any)
	transcriptionSlot    chan struct{}
}

func New(options Options) (*Server, error) {
	if options.Host == "" || options.Origin == "" || options.Token == "" {
		return nil, fmt.Errorf("host, origin, and session token are required")
	}
	if options.Engine == nil || options.LoadSettings == nil || options.SaveSettings == nil {
		return nil, fmt.Errorf("engine and settings providers are required")
	}
	maxSeconds := options.MaxRecordingSeconds
	if maxSeconds == 0 {
		maxSeconds = DefaultMaxRecordingSeconds
	}
	if maxSeconds < 1 || maxSeconds > MaxRecordingSecondsCeiling {
		return nil, fmt.Errorf("recording limit must be between 1 and %d seconds", MaxRecordingSecondsCeiling)
	}
	timeout := options.TranscriptionTimeout
	if timeout == 0 {
		timeout = 2 * time.Minute
	}
	if timeout < 10*time.Millisecond {
		return nil, fmt.Errorf("transcription timeout must be at least 10 milliseconds")
	}
	logf := options.Logf
	if logf == nil {
		logf = func(string, ...any) {}
	}
	return &Server{
		host:                 options.Host,
		origin:               options.Origin,
		token:                []byte(options.Token),
		maxRecordingSeconds:  maxSeconds,
		maxSamples:           appaudio.SampleRate * maxSeconds,
		transcriptionTimeout: timeout,
		engine:               options.Engine,
		loadSettings:         options.LoadSettings,
		saveSettings:         options.SaveSettings,
		openLog:              options.OpenLog,
		quit:                 options.Quit,
		onActivity:           options.OnActivity,
		logf:                 logf,
		transcriptionSlot:    make(chan struct{}, 1),
	}, nil
}

func (s *Server) Handler() http.Handler {
	return s.securityHeaders(http.HandlerFunc(s.serveHTTP))
}

func (s *Server) serveHTTP(w http.ResponseWriter, r *http.Request) {
	if strings.HasPrefix(r.URL.Path, "/api/") {
		if !s.authorizeAPI(w, r) {
			return
		}
		s.touch()
		s.serveAPI(w, r)
		return
	}
	s.serveAsset(w, r)
}

func (s *Server) serveAsset(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.Header().Set("Allow", "GET, HEAD")
		s.writeError(w, http.StatusMethodNotAllowed, "Method not allowed.")
		return
	}
	files := map[string]struct {
		name        string
		contentType string
	}{
		"/":                    {name: "assets/index.html", contentType: "text/html; charset=utf-8"},
		"/app.css":             {name: "assets/app.css", contentType: "text/css; charset=utf-8"},
		"/app.js":              {name: "assets/app.js", contentType: "text/javascript; charset=utf-8"},
		"/recorder-worklet.js": {name: "assets/recorder-worklet.js", contentType: "text/javascript; charset=utf-8"},
	}
	asset, ok := files[r.URL.Path]
	if !ok {
		s.writeError(w, http.StatusNotFound, "Not found.")
		return
	}
	body, err := assets.ReadFile(asset.name)
	if err != nil {
		s.logf("read embedded asset %s: %v", asset.name, err)
		s.writeError(w, http.StatusInternalServerError, "VoxSteer could not load its interface.")
		return
	}
	w.Header().Set("Content-Type", asset.contentType)
	w.Header().Set("Content-Length", fmt.Sprint(len(body)))
	w.WriteHeader(http.StatusOK)
	if r.Method == http.MethodGet {
		_, _ = w.Write(body)
	}
}

func (s *Server) authorizeAPI(w http.ResponseWriter, r *http.Request) bool {
	if r.Host != s.host {
		s.writeError(w, http.StatusForbidden, "Request rejected.")
		return false
	}
	remoteHost, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil || remoteHost != "127.0.0.1" {
		s.writeError(w, http.StatusForbidden, "Request rejected.")
		return false
	}
	origin := r.Header.Get("Origin")
	if origin != "" && origin != s.origin {
		s.writeError(w, http.StatusForbidden, "Request rejected.")
		return false
	}
	if r.Method != http.MethodGet && origin != s.origin {
		s.writeError(w, http.StatusForbidden, "Request rejected.")
		return false
	}
	if site := r.Header.Get("Sec-Fetch-Site"); site != "" && site != "same-origin" {
		s.writeError(w, http.StatusForbidden, "Request rejected.")
		return false
	}
	provided := []byte(r.Header.Get("X-VoxSteer-Token"))
	if len(provided) != len(s.token) || subtle.ConstantTimeCompare(provided, s.token) != 1 {
		s.writeError(w, http.StatusForbidden, "The VoxSteer session is invalid. Launch browser mode again.")
		return false
	}
	return true
}

func (s *Server) serveAPI(w http.ResponseWriter, r *http.Request) {
	switch r.URL.Path {
	case "/api/session":
		s.requireMethod(w, r, http.MethodPost, s.handleSession)
	case "/api/settings":
		s.handleSettings(w, r)
	case "/api/transcribe":
		s.requireMethod(w, r, http.MethodPost, s.handleTranscribe)
	case "/api/ping":
		s.requireMethod(w, r, http.MethodPost, func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusNoContent)
		})
	case "/api/open-log":
		s.requireMethod(w, r, http.MethodPost, s.handleOpenLog)
	case "/api/quit":
		s.requireMethod(w, r, http.MethodPost, s.handleQuit)
	default:
		s.writeError(w, http.StatusNotFound, "Not found.")
	}
}

func (s *Server) requireMethod(w http.ResponseWriter, r *http.Request, method string, handler http.HandlerFunc) {
	if r.Method != method {
		w.Header().Set("Allow", method)
		s.writeError(w, http.StatusMethodNotAllowed, "Method not allowed.")
		return
	}
	handler(w, r)
}

func (s *Server) handleSession(w http.ResponseWriter, _ *http.Request) {
	_, status, err := s.engine()
	response := struct {
		Ready               bool   `json:"ready"`
		Engine              string `json:"engine"`
		Language            string `json:"language"`
		MaxRecordingSeconds int    `json:"max_recording_seconds"`
		Error               string `json:"error,omitempty"`
	}{
		Ready:               err == nil,
		Engine:              status.Engine,
		Language:            status.Language,
		MaxRecordingSeconds: s.maxRecordingSeconds,
	}
	if err != nil {
		response.Error = err.Error()
	}
	s.writeJSON(w, http.StatusOK, response)
}

func (s *Server) handleSettings(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		settings, err := s.loadSettings()
		if err != nil {
			s.logf("load browser settings: %v", err)
			s.writeError(w, http.StatusUnprocessableEntity, "VoxSteer could not load its settings. Open the diagnostic log for details.")
			return
		}
		settings.CloudAPIKey = nil
		s.writeJSON(w, http.StatusOK, settings)
	case http.MethodPut:
		if !hasMediaType(r, "application/json") {
			s.writeError(w, http.StatusUnsupportedMediaType, "Settings must be sent as JSON.")
			return
		}
		var settings Settings
		if err := decodeJSONBody(w, r, maxSettingsBody, &settings); err != nil {
			s.writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		if err := s.saveSettings(settings); err != nil {
			s.logf("save browser settings: %v", err)
			s.writeError(w, http.StatusUnprocessableEntity, err.Error())
			return
		}
		w.WriteHeader(http.StatusNoContent)
	default:
		w.Header().Set("Allow", "GET, PUT")
		s.writeError(w, http.StatusMethodNotAllowed, "Method not allowed.")
	}
}

func (s *Server) handleTranscribe(w http.ResponseWriter, r *http.Request) {
	if r.Header.Get("Content-Encoding") != "" {
		s.writeError(w, http.StatusUnsupportedMediaType, "Compressed request bodies are not supported.")
		return
	}
	if !hasMediaType(r, "audio/wav") && !hasMediaType(r, "audio/x-wav") {
		s.writeError(w, http.StatusUnsupportedMediaType, "Audio must be a WAV file.")
		return
	}
	select {
	case s.transcriptionSlot <- struct{}{}:
		defer func() { <-s.transcriptionSlot }()
	default:
		s.writeError(w, http.StatusConflict, "Another transcription is already running.")
		return
	}

	maxBody := int64(44 + s.maxSamples*2)
	if r.ContentLength > maxBody {
		s.writeError(w, http.StatusRequestEntityTooLarge, "Recording exceeds the configured duration limit.")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxBody)
	body, err := io.ReadAll(r.Body)
	if err != nil {
		var maxErr *http.MaxBytesError
		if errors.As(err, &maxErr) {
			s.writeError(w, http.StatusRequestEntityTooLarge, "Recording exceeds the configured duration limit.")
			return
		}
		s.writeError(w, http.StatusBadRequest, "VoxSteer could not read the recording.")
		return
	}
	samples, err := appaudio.DecodeWAV(body, s.maxSamples)
	if err != nil {
		s.writeError(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	if len(samples) == 0 {
		s.writeError(w, http.StatusUnprocessableEntity, "Recording is empty.")
		return
	}

	engine, status, err := s.engine()
	if err != nil {
		s.writeError(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	s.logf("browser transcription started: engine=%s samples=%d duration=%.2fs", status.Engine, len(samples), float64(len(samples))/appaudio.SampleRate)
	ctx, cancel := context.WithTimeout(r.Context(), s.transcriptionTimeout)
	defer cancel()
	text, err := engine.Transcribe(ctx, samples)
	if err == nil && ctx.Err() != nil {
		err = ctx.Err()
	}
	if err != nil {
		s.logf("browser transcription failed: %s", safeTranscriptionError(err))
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(ctx.Err(), context.DeadlineExceeded) {
			s.writeError(w, http.StatusGatewayTimeout, "Transcription timed out. Try a shorter recording.")
			return
		}
		if errors.Is(err, context.Canceled) || errors.Is(ctx.Err(), context.Canceled) {
			s.writeError(w, http.StatusRequestTimeout, "Transcription was cancelled.")
			return
		}
		s.writeError(w, http.StatusBadGateway, "Transcription failed. Open the diagnostic log for details.")
		return
	}
	text = strings.TrimSpace(text)
	if len(text) > maxTranscriptBytes || !utf8.ValidString(text) {
		s.logf("browser transcription returned invalid or oversized text: bytes=%d", len(text))
		s.writeError(w, http.StatusBadGateway, "The transcription engine returned an invalid result.")
		return
	}
	s.logf("browser transcription complete: characters=%d", utf8.RuneCountInString(text))
	s.writeJSON(w, http.StatusOK, struct {
		Text string `json:"text"`
	}{Text: text})
}

func (s *Server) handleOpenLog(w http.ResponseWriter, _ *http.Request) {
	if s.openLog == nil {
		s.writeError(w, http.StatusNotImplemented, "Opening the diagnostic log is unavailable.")
		return
	}
	if err := s.openLog(); err != nil {
		s.logf("open diagnostic log: %v", err)
		s.writeError(w, http.StatusInternalServerError, "VoxSteer could not open the diagnostic log.")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleQuit(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusNoContent)
	if s.quit != nil {
		go s.quit()
	}
}

func (s *Server) securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		header := w.Header()
		header.Set("Cache-Control", "no-store")
		header.Set("Content-Security-Policy", "default-src 'none'; script-src 'self'; style-src 'self'; connect-src 'self'; worker-src 'self'; img-src 'self'; media-src 'self'; base-uri 'none'; form-action 'none'; frame-ancestors 'none'")
		header.Set("Cross-Origin-Opener-Policy", "same-origin")
		header.Set("Cross-Origin-Resource-Policy", "same-origin")
		header.Set("Permissions-Policy", "microphone=(self)")
		header.Set("Referrer-Policy", "no-referrer")
		header.Set("X-Content-Type-Options", "nosniff")
		header.Set("X-Frame-Options", "DENY")
		next.ServeHTTP(w, r)
	})
}

func (s *Server) touch() {
	if s.onActivity != nil {
		s.onActivity()
	}
}

func (s *Server) writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(value); err != nil {
		s.logf("write JSON response: %v", err)
	}
}

func (s *Server) writeError(w http.ResponseWriter, status int, message string) {
	s.writeJSON(w, status, struct {
		Error string `json:"error"`
	}{Error: message})
}

func hasMediaType(r *http.Request, expected string) bool {
	mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	return err == nil && strings.EqualFold(mediaType, expected)
}

func decodeJSONBody(w http.ResponseWriter, r *http.Request, limit int64, value any) error {
	r.Body = http.MaxBytesReader(w, r.Body, limit)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(value); err != nil {
		var maxErr *http.MaxBytesError
		if errors.As(err, &maxErr) {
			return fmt.Errorf("settings request exceeds %d bytes", limit)
		}
		return fmt.Errorf("invalid settings: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return fmt.Errorf("invalid settings: multiple JSON values")
		}
		return fmt.Errorf("invalid settings: %w", err)
	}
	return nil
}

func safeTranscriptionError(err error) string {
	var safe interface{ SafeSummary() string }
	if errors.As(err, &safe) {
		return safe.SafeSummary()
	}
	var networkError *url.Error
	if errors.As(err, &networkError) {
		return "network request failed"
	}
	var exitError *exec.ExitError
	if errors.As(err, &exitError) {
		return fmt.Sprintf("local process exited with code %d", exitError.ExitCode())
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return "transcription timed out"
	}
	if errors.Is(err, context.Canceled) {
		return "transcription was cancelled"
	}
	return fmt.Sprintf("error_type=%T", err)
}
