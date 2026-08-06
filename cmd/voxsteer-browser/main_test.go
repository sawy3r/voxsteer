package main

import (
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/sawy3r/voxsteer/internal/config"
	"github.com/sawy3r/voxsteer/internal/webapp"
)

func TestSessionTokenIsURLSafeAndRandom(t *testing.T) {
	first, err := sessionToken()
	if err != nil {
		t.Fatal(err)
	}
	second, err := sessionToken()
	if err != nil {
		t.Fatal(err)
	}
	if first == second {
		t.Fatal("session tokens unexpectedly match")
	}
	if len(first) != 43 || url.QueryEscape(first) != first {
		t.Fatalf("token is not an unpadded URL-safe 32-byte value: %q", first)
	}
}

func TestValidateLoopbackAddress(t *testing.T) {
	for _, address := range []string{"127.0.0.1:47823", "127.0.0.1:0"} {
		if err := validateLoopbackAddress(address); err != nil {
			t.Fatalf("validateLoopbackAddress(%q): %v", address, err)
		}
	}
	for _, address := range []string{"0.0.0.0:47823", "[::1]:47823", "localhost:47823", "bad"} {
		if err := validateLoopbackAddress(address); err == nil {
			t.Fatalf("validateLoopbackAddress(%q) error = nil", address)
		}
	}
}

func TestValidateConfigCloud(t *testing.T) {
	var cfg config.Config
	cfg.Engine = "cloud"
	cfg.Cloud.Endpoint = "https://example.test/v1/audio/transcriptions"
	cfg.Cloud.Model = "whisper-1"
	if err := validateConfig(cfg); err != nil {
		t.Fatal(err)
	}

	cfg.Cloud.Endpoint = "file:///tmp/transcript"
	if err := validateConfig(cfg); err == nil {
		t.Fatalf("validateConfig() error = %v", err)
	}

	cfg.Cloud.Endpoint = "http://provider.example/transcribe"
	if err := validateConfig(cfg); err == nil || !strings.Contains(err.Error(), "loopback") {
		t.Fatalf("validateConfig() error = %v", err)
	}

	cfg.Cloud.Endpoint = "http://127.0.0.1:8080/transcribe"
	if err := validateConfig(cfg); err != nil {
		t.Fatalf("validateConfig(loopback): %v", err)
	}
}

func TestSecureCloudClientRejectsInsecureRedirect(t *testing.T) {
	request, err := http.NewRequest(http.MethodGet, "http://provider.example/transcribe", nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := secureCloudClient().CheckRedirect(request, nil); err == nil {
		t.Fatal("CheckRedirect() error = nil, want insecure redirect error")
	}
}

func TestSaveSettingsRejectsEnvironmentOverrides(t *testing.T) {
	t.Setenv("VTT_ENGINE", "cloud")
	err := saveSettings(webapp.Settings{})
	if err == nil || !strings.Contains(err.Error(), "VTT_ENGINE") {
		t.Fatalf("saveSettings() error = %v", err)
	}
}
