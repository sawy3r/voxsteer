package settings

import (
	"strings"
	"testing"

	"github.com/sawy3r/voxsteer/internal/config"
)

// isolate points the per-user configuration at an empty directory.
func isolate(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("XDG_CONFIG_HOME", dir)
	t.Setenv("AppData", dir)
}

func cloudValues() Values {
	return Values{
		Engine:        "cloud",
		CloudEndpoint: "https://example.test/v1/audio/transcriptions",
		CloudModel:    "whisper-1",
	}
}

func TestValidateCloud(t *testing.T) {
	var cfg config.Config
	cfg.Engine = "cloud"
	cfg.Cloud.Endpoint = "https://example.test/v1/audio/transcriptions"
	cfg.Cloud.Model = "whisper-1"
	if err := Validate(cfg); err != nil {
		t.Fatal(err)
	}

	cfg.Cloud.Endpoint = "file:///tmp/transcript"
	if err := Validate(cfg); err == nil {
		t.Fatalf("Validate() error = %v", err)
	}

	cfg.Cloud.Endpoint = "http://provider.example/transcribe"
	if err := Validate(cfg); err == nil || !strings.Contains(err.Error(), "loopback") {
		t.Fatalf("Validate() error = %v", err)
	}

	cfg.Cloud.Endpoint = "http://127.0.0.1:8080/transcribe"
	if err := Validate(cfg); err != nil {
		t.Fatalf("Validate(loopback): %v", err)
	}
}

func TestSaveRejectsEnvironmentOverrides(t *testing.T) {
	t.Setenv("VTT_ENGINE", "cloud")
	err := Save(Values{})
	if err == nil || !strings.Contains(err.Error(), "VTT_ENGINE") {
		t.Fatalf("Save() error = %v", err)
	}
}

func TestSaveWritesShortcut(t *testing.T) {
	isolate(t)
	values := cloudValues()
	values.Shortcut = "double-left-ctrl"
	if err := Save(values); err != nil {
		t.Fatal(err)
	}
	loaded, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Shortcut != "double-left-ctrl" {
		t.Fatalf("Shortcut = %q", loaded.Shortcut)
	}
}

func TestSaveKeepsShortcutWhenInterfaceOmitsIt(t *testing.T) {
	isolate(t)
	values := cloudValues()
	values.Shortcut = "mouse4"
	if err := Save(values); err != nil {
		t.Fatal(err)
	}
	if err := Save(cloudValues()); err != nil {
		t.Fatal(err)
	}
	loaded, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Shortcut != "mouse4" {
		t.Fatalf("Shortcut = %q, want it preserved", loaded.Shortcut)
	}
}

func TestSaveRejectsInvalidShortcut(t *testing.T) {
	isolate(t)
	values := cloudValues()
	values.Shortcut = "option+space"
	err := Save(values)
	if err == nil || !strings.Contains(err.Error(), "shortcut") {
		t.Fatalf("Save() error = %v", err)
	}
}

func TestSaveKeepsAPIKeyUnlessReplaced(t *testing.T) {
	isolate(t)
	key := "secret"
	values := cloudValues()
	values.CloudAPIKey = &key
	if err := Save(values); err != nil {
		t.Fatal(err)
	}
	if err := Save(cloudValues()); err != nil {
		t.Fatal(err)
	}
	loaded, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if !loaded.HasCloudAPIKey {
		t.Fatal("saved API key was lost")
	}
	if loaded.CloudAPIKey != nil {
		t.Fatal("Load() exposed the saved API key")
	}

	empty := ""
	values.CloudAPIKey = &empty
	if err := Save(values); err != nil {
		t.Fatal(err)
	}
	if loaded, err = Load(); err != nil {
		t.Fatal(err)
	}
	if loaded.HasCloudAPIKey {
		t.Fatal("an empty replacement should remove the saved API key")
	}
}
