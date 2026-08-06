package webapp

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	appaudio "github.com/sawy3r/voxsteer/internal/audio"
	"github.com/sawy3r/voxsteer/internal/transcriber"
)

const (
	testHost   = "127.0.0.1:47823"
	testOrigin = "http://127.0.0.1:47823"
	testToken  = "test-session-token"
)

type transcriberFunc func(context.Context, []float32) (string, error)

func (f transcriberFunc) Transcribe(ctx context.Context, samples []float32) (string, error) {
	return f(ctx, samples)
}

func testServer(t *testing.T, engine transcriber.Transcriber) *Server {
	t.Helper()
	if engine == nil {
		engine = transcriberFunc(func(context.Context, []float32) (string, error) {
			return "test transcript", nil
		})
	}
	server, err := New(Options{
		Host:                 testHost,
		Origin:               testOrigin,
		Token:                testToken,
		MaxRecordingSeconds:  1,
		TranscriptionTimeout: time.Second,
		Engine: func() (transcriber.Transcriber, EngineStatus, error) {
			return engine, EngineStatus{Engine: "cloud", Language: "en"}, nil
		},
		LoadSettings: func() (Settings, error) {
			return Settings{
				Engine:         "cloud",
				Language:       "en",
				CloudEndpoint:  "https://example.test/transcribe",
				CloudModel:     "whisper-1",
				HasCloudAPIKey: true,
				CloudAPIKey:    stringPointer("must-not-leak"),
			}, nil
		},
		SaveSettings: func(Settings) error { return nil },
	})
	if err != nil {
		t.Fatal(err)
	}
	return server
}

func apiRequest(method, path string, body io.Reader) *http.Request {
	request := httptest.NewRequest(method, testOrigin+path, body)
	request.Host = testHost
	request.RemoteAddr = "127.0.0.1:54321"
	request.Header.Set("X-VoxSteer-Token", testToken)
	request.Header.Set("Sec-Fetch-Site", "same-origin")
	if method != http.MethodGet {
		request.Header.Set("Origin", testOrigin)
	}
	return request
}

func TestStaticAssetsHaveSecurityHeaders(t *testing.T) {
	response := httptest.NewRecorder()
	testServer(t, nil).Handler().ServeHTTP(response, httptest.NewRequest(http.MethodGet, testOrigin+"/", nil))
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", response.Code)
	}
	if got := response.Header().Get("Content-Security-Policy"); !strings.Contains(got, "default-src 'none'") || !strings.Contains(got, "worker-src 'self'") {
		t.Fatalf("Content-Security-Policy = %q", got)
	}
	if response.Header().Get("Access-Control-Allow-Origin") != "" {
		t.Fatal("unexpected CORS response header")
	}
	if !strings.Contains(response.Body.String(), "VoxSteer Browser") {
		t.Fatal("root response does not contain browser interface")
	}
}

func TestAPIRejectsUntrustedRequests(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*http.Request)
	}{
		{name: "host", mutate: func(r *http.Request) { r.Host = "attacker.test" }},
		{name: "remote", mutate: func(r *http.Request) { r.RemoteAddr = "192.0.2.10:1234" }},
		{name: "origin", mutate: func(r *http.Request) { r.Header.Set("Origin", "https://attacker.test") }},
		{name: "fetch site", mutate: func(r *http.Request) { r.Header.Set("Sec-Fetch-Site", "cross-site") }},
		{name: "token", mutate: func(r *http.Request) { r.Header.Set("X-VoxSteer-Token", "wrong") }},
		{name: "missing origin", mutate: func(r *http.Request) { r.Header.Del("Origin") }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request := apiRequest(http.MethodPost, "/api/session", nil)
			test.mutate(request)
			response := httptest.NewRecorder()
			testServer(t, nil).Handler().ServeHTTP(response, request)
			if response.Code != http.StatusForbidden {
				t.Fatalf("status = %d, want 403", response.Code)
			}
		})
	}
}

func TestSessionAndSettingsNeverExposeAPIKey(t *testing.T) {
	server := testServer(t, nil)
	for _, request := range []*http.Request{
		apiRequest(http.MethodPost, "/api/session", nil),
		apiRequest(http.MethodGet, "/api/settings", nil),
	} {
		response := httptest.NewRecorder()
		server.Handler().ServeHTTP(response, request)
		if response.Code != http.StatusOK {
			t.Fatalf("%s status = %d, want 200", request.URL.Path, response.Code)
		}
		var payload map[string]any
		if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
			t.Fatal(err)
		}
		_, hasAPIKeyField := payload["cloud_api_key"]
		if strings.Contains(response.Body.String(), "must-not-leak") || hasAPIKeyField {
			t.Fatalf("%s leaked API key: %s", request.URL.Path, response.Body.String())
		}
	}
}

func TestSaveSettingsPreservesOptionalAPIKey(t *testing.T) {
	var saved Settings
	server, err := New(Options{
		Host:   testHost,
		Origin: testOrigin,
		Token:  testToken,
		Engine: func() (transcriber.Transcriber, EngineStatus, error) {
			return transcriberFunc(func(context.Context, []float32) (string, error) { return "", nil }), EngineStatus{}, nil
		},
		LoadSettings: func() (Settings, error) { return Settings{}, nil },
		SaveSettings: func(settings Settings) error {
			saved = settings
			return nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	body := `{"engine":"cloud","language":"en","cloud_endpoint":"https://example.test","cloud_model":"whisper-1","cloud_api_key":"new-secret","local_binary":"","local_model":"","has_cloud_api_key":false}`
	request := apiRequest(http.MethodPut, "/api/settings", strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	server.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204: %s", response.Code, response.Body.String())
	}
	if saved.CloudAPIKey == nil || *saved.CloudAPIKey != "new-secret" {
		t.Fatalf("saved API key = %#v", saved.CloudAPIKey)
	}
}

func TestTranscribeCanonicalWAV(t *testing.T) {
	var gotSamples []float32
	engine := transcriberFunc(func(_ context.Context, samples []float32) (string, error) {
		gotSamples = append([]float32(nil), samples...)
		return " hello world ", nil
	})
	wav, err := appaudio.EncodeWAV([]float32{-0.5, 0, 0.5})
	if err != nil {
		t.Fatal(err)
	}
	request := apiRequest(http.MethodPost, "/api/transcribe", bytes.NewReader(wav))
	request.Header.Set("Content-Type", "audio/wav")
	response := httptest.NewRecorder()
	testServer(t, engine).Handler().ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", response.Code, response.Body.String())
	}
	if len(gotSamples) != 3 {
		t.Fatalf("sample count = %d, want 3", len(gotSamples))
	}
	var result struct {
		Text string `json:"text"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result.Text != "hello world" {
		t.Fatalf("text = %q, want hello world", result.Text)
	}
}

func TestTranscribeRejectsMalformedAndOversizedAudio(t *testing.T) {
	server := testServer(t, nil)
	tests := []struct {
		name string
		body []byte
		want int
	}{
		{name: "malformed", body: []byte("not a wav"), want: http.StatusUnprocessableEntity},
		{name: "oversized", body: make([]byte, 44+appaudio.SampleRate*2+1), want: http.StatusRequestEntityTooLarge},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request := apiRequest(http.MethodPost, "/api/transcribe", bytes.NewReader(test.body))
			request.Header.Set("Content-Type", "audio/wav")
			if test.name == "oversized" {
				request.ContentLength = -1
			}
			response := httptest.NewRecorder()
			server.Handler().ServeHTTP(response, request)
			if response.Code != test.want {
				t.Fatalf("status = %d, want %d: %s", response.Code, test.want, response.Body.String())
			}
		})
	}
}

func TestTranscribeAllowsOnlyOneActiveRequest(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	var once sync.Once
	engine := transcriberFunc(func(context.Context, []float32) (string, error) {
		once.Do(func() { close(started) })
		<-release
		return "first", nil
	})
	server := testServer(t, engine)
	wav, err := appaudio.EncodeWAV([]float32{0.1})
	if err != nil {
		t.Fatal(err)
	}

	firstDone := make(chan struct{})
	go func() {
		defer close(firstDone)
		request := apiRequest(http.MethodPost, "/api/transcribe", bytes.NewReader(wav))
		request.Header.Set("Content-Type", "audio/wav")
		server.Handler().ServeHTTP(httptest.NewRecorder(), request)
	}()
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("first transcription did not start")
	}

	request := apiRequest(http.MethodPost, "/api/transcribe", bytes.NewReader(wav))
	request.Header.Set("Content-Type", "audio/wav")
	response := httptest.NewRecorder()
	server.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409", response.Code)
	}
	close(release)
	select {
	case <-firstDone:
	case <-time.After(time.Second):
		t.Fatal("first transcription did not finish")
	}
}

func TestTranscribeDoesNotExposeProviderError(t *testing.T) {
	engine := transcriberFunc(func(context.Context, []float32) (string, error) {
		return "", errors.New("provider response contains sensitive detail")
	})
	wav, err := appaudio.EncodeWAV([]float32{0.1})
	if err != nil {
		t.Fatal(err)
	}
	request := apiRequest(http.MethodPost, "/api/transcribe", bytes.NewReader(wav))
	request.Header.Set("Content-Type", "audio/wav")
	response := httptest.NewRecorder()
	testServer(t, engine).Handler().ServeHTTP(response, request)
	if response.Code != http.StatusBadGateway {
		t.Fatalf("status = %d, want 502", response.Code)
	}
	if strings.Contains(response.Body.String(), "sensitive detail") {
		t.Fatalf("provider detail leaked to browser: %s", response.Body.String())
	}
}

func TestSafeTranscriptionErrorOmitsProviderDetails(t *testing.T) {
	err := &transcriber.EndpointError{
		StatusCode: http.StatusUnauthorized,
		Status:     "401 Unauthorized",
		Details:    "reflected secret and transcript",
	}
	summary := safeTranscriptionError(err)
	if summary != "endpoint returned HTTP 401" {
		t.Fatalf("summary = %q", summary)
	}
	if strings.Contains(summary, err.Details) {
		t.Fatalf("safe summary leaked details: %q", summary)
	}
}

func TestTranscribeRejectsSuccessAfterDeadline(t *testing.T) {
	engine := transcriberFunc(func(ctx context.Context, _ []float32) (string, error) {
		<-ctx.Done()
		return "late transcript", nil
	})
	server, err := New(Options{
		Host:                 testHost,
		Origin:               testOrigin,
		Token:                testToken,
		MaxRecordingSeconds:  1,
		TranscriptionTimeout: 20 * time.Millisecond,
		Engine: func() (transcriber.Transcriber, EngineStatus, error) {
			return engine, EngineStatus{Engine: "cloud"}, nil
		},
		LoadSettings: func() (Settings, error) { return Settings{}, nil },
		SaveSettings: func(Settings) error { return nil },
	})
	if err != nil {
		t.Fatal(err)
	}
	wav, err := appaudio.EncodeWAV([]float32{0.1})
	if err != nil {
		t.Fatal(err)
	}
	request := apiRequest(http.MethodPost, "/api/transcribe", bytes.NewReader(wav))
	request.Header.Set("Content-Type", "audio/wav")
	response := httptest.NewRecorder()
	server.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusGatewayTimeout {
		t.Fatalf("status = %d, want 504: %s", response.Code, response.Body.String())
	}
}

func stringPointer(value string) *string {
	return &value
}
