package app

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/sawy3r/voxsteer/internal/config"
	"github.com/sawy3r/voxsteer/internal/transcriber"
)

func TestConfiguredEngineCloud(t *testing.T) {
	var cfg config.Config
	cfg.Engine = "cloud"
	cfg.Language = "fr"
	cfg.Cloud.Endpoint = "https://example.test/transcribe"
	cfg.Cloud.APIKey = "secret"
	cfg.Cloud.Model = "whisper-test"

	engine, err := ConfiguredEngine(cfg)
	if err != nil {
		t.Fatal(err)
	}
	cloud, ok := engine.(*transcriber.Cloud)
	if !ok {
		t.Fatalf("engine type = %T, want *transcriber.Cloud", engine)
	}
	if cloud.Endpoint != cfg.Cloud.Endpoint || cloud.APIKey != cfg.Cloud.APIKey || cloud.Model != cfg.Cloud.Model || cloud.Language != cfg.Language {
		t.Fatalf("cloud engine = %#v", cloud)
	}
}

func TestConfiguredEngineLocalRequiresModel(t *testing.T) {
	var cfg config.Config
	cfg.Engine = "local"
	if _, err := ConfiguredEngine(cfg); err == nil {
		t.Fatal("ConfiguredEngine() error = nil, want missing model error")
	}
}

func TestConfiguredEngineLocal(t *testing.T) {
	dir := t.TempDir()
	model := filepath.Join(dir, "model.bin")
	binary := filepath.Join(dir, "whisper-cli")
	for _, path := range []string{model, binary} {
		if err := os.WriteFile(path, []byte("stub"), 0o700); err != nil {
			t.Fatal(err)
		}
	}

	var cfg config.Config
	cfg.Engine = "local"
	cfg.Language = "en"
	cfg.Local.Model = model
	cfg.Local.Binary = binary

	engine, err := ConfiguredEngine(cfg)
	if err != nil {
		t.Fatal(err)
	}
	local, ok := engine.(*transcriber.LocalWhisper)
	if !ok {
		t.Fatalf("engine type = %T, want *transcriber.LocalWhisper", engine)
	}
	if local.Binary != binary || local.Model != model || local.Language != "en" {
		t.Fatalf("local engine = %#v", local)
	}
}

func TestConfiguredEngineLocalRejectsMissingModelFile(t *testing.T) {
	var cfg config.Config
	cfg.Engine = "local"
	cfg.Local.Model = filepath.Join(t.TempDir(), "missing.bin")
	if _, err := ConfiguredEngine(cfg); err == nil {
		t.Fatal("ConfiguredEngine() error = nil, want unreadable model error")
	}
}

func TestConfiguredEngineLocalRejectsMissingBinary(t *testing.T) {
	dir := t.TempDir()
	model := filepath.Join(dir, "model.bin")
	if err := os.WriteFile(model, []byte("stub"), 0o600); err != nil {
		t.Fatal(err)
	}

	var cfg config.Config
	cfg.Engine = "local"
	cfg.Local.Model = model
	cfg.Local.Binary = filepath.Join(dir, "missing", "whisper-cli")
	if _, err := ConfiguredEngine(cfg); err == nil {
		t.Fatal("ConfiguredEngine() error = nil, want missing binary error")
	}
}

func TestConfiguredEngineRejectsUnknownEngine(t *testing.T) {
	var cfg config.Config
	cfg.Engine = "other"
	if _, err := ConfiguredEngine(cfg); err == nil {
		t.Fatal("ConfiguredEngine() error = nil, want invalid engine error")
	}
}

func TestSwitchableEngineUsesTheLatestTranscriber(t *testing.T) {
	var engine SwitchableEngine
	engine.Set(transcriberFunc(func(context.Context, []float32) (string, error) {
		return "first", nil
	}))
	engine.Set(transcriberFunc(func(context.Context, []float32) (string, error) {
		return "second", nil
	}))
	got, err := engine.Transcribe(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if got != "second" {
		t.Fatalf("Transcribe() = %q", got)
	}
}

func TestSwitchableEngineAsksForSetupWhenEmpty(t *testing.T) {
	var engine SwitchableEngine
	_, err := engine.Transcribe(context.Background(), nil)
	var notice *Notice
	if !errors.As(err, &notice) {
		t.Fatalf("Transcribe() error = %v, want a notice", err)
	}
}
