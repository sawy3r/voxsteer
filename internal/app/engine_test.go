package app

import (
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

func TestConfiguredEngineRejectsUnknownEngine(t *testing.T) {
	var cfg config.Config
	cfg.Engine = "other"
	if _, err := ConfiguredEngine(cfg); err == nil {
		t.Fatal("ConfiguredEngine() error = nil, want invalid engine error")
	}
}
