package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestLoadExplicitConfigAndEnvironmentOverride(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	body := []byte(`{
		"engine": "local",
		"language": "en",
		"shortcut": "ctrl+shift+d",
		"local": {"binary": "whisper/whisper-cli.exe", "model": "models/model.bin"},
		"cloud": {"endpoint": "", "api_key": "", "model": "whisper-1"}
	}`)
	if err := os.WriteFile(path, body, 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("VTT_CONFIG", path)
	t.Setenv("VTT_LANGUAGE", "fr")

	cfg, loadedPath, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if loadedPath != path {
		t.Fatalf("path = %q, want %q", loadedPath, path)
	}
	if cfg.Language != "fr" {
		t.Fatalf("language = %q, want fr", cfg.Language)
	}
	if cfg.Shortcut != "ctrl+shift+d" {
		t.Fatalf("shortcut = %q, want ctrl+shift+d", cfg.Shortcut)
	}
	if cfg.Local.Model != filepath.Join(dir, "models", "model.bin") {
		t.Fatalf("model path = %q", cfg.Local.Model)
	}
}

func TestLoadThreadsFromFileAndEnvironment(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	body := []byte(`{"engine": "local", "local": {"model": "model.bin", "threads": 6}}`)
	if err := os.WriteFile(path, body, 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("VTT_CONFIG", path)

	cfg, _, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Local.Threads != 6 {
		t.Fatalf("threads = %d, want 6", cfg.Local.Threads)
	}

	t.Setenv("VTT_WHISPER_THREADS", "12")
	cfg, _, err = Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Local.Threads != 12 {
		t.Fatalf("threads = %d, want 12", cfg.Local.Threads)
	}

	t.Setenv("VTT_WHISPER_THREADS", "many")
	if _, _, err := Load(); err == nil {
		t.Fatal("Load() error = nil, want thread parse error")
	}
}

func TestSaveUserWritesPrivateConfig(t *testing.T) {
	home := t.TempDir()
	var configRoot string
	switch runtime.GOOS {
	case "darwin":
		t.Setenv("HOME", home)
		configRoot = filepath.Join(home, "Library", "Application Support")
	case "windows":
		t.Setenv("AppData", home)
		configRoot = home
	default:
		t.Setenv("XDG_CONFIG_HOME", home)
		configRoot = home
	}

	var cfg Config
	cfg.Engine = "cloud"
	cfg.Cloud.Endpoint = "https://example.test/transcribe"
	cfg.Cloud.APIKey = "secret"
	cfg.Cloud.Model = "whisper-1"

	path, err := SaveUser(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(configRoot, "VoxSteer", "config.json"); path != want {
		t.Fatalf("path = %q, want %q", path, want)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); runtime.GOOS != "windows" && got != 0o600 {
		t.Fatalf("mode = %o, want 600", got)
	}
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var saved Config
	if err := json.Unmarshal(body, &saved); err != nil {
		t.Fatal(err)
	}
	if saved.Cloud.APIKey != "secret" || saved.Cloud.Endpoint != cfg.Cloud.Endpoint {
		t.Fatalf("saved config = %#v", saved)
	}
}

func TestLoadRejectsTrailingJSON(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, []byte(`{"engine":"cloud"} {"engine":"local"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("VTT_CONFIG", path)
	if _, _, err := Load(); err == nil {
		t.Fatal("Load() error = nil, want trailing JSON error")
	}
}
