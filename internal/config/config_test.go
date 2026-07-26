package config

import (
	"os"
	"path/filepath"
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
