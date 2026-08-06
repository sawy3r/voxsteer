package app

import (
	"fmt"

	"github.com/sawy3r/voxsteer/internal/config"
	"github.com/sawy3r/voxsteer/internal/transcriber"
)

// ConfiguredEngine creates the transcription engine selected by cfg.
func ConfiguredEngine(cfg config.Config) (transcriber.Transcriber, error) {
	switch cfg.Engine {
	case "cloud":
		if cfg.Cloud.Endpoint == "" {
			return nil, fmt.Errorf("VTT_CLOUD_ENDPOINT is required for the cloud engine")
		}
		return &transcriber.Cloud{
			Endpoint: cfg.Cloud.Endpoint,
			APIKey:   cfg.Cloud.APIKey,
			Model:    cfg.Cloud.Model,
			Language: cfg.Language,
		}, nil
	case "local":
		if cfg.Local.Model == "" {
			return nil, fmt.Errorf("VTT_WHISPER_MODEL is required for the local engine")
		}
		return &transcriber.LocalWhisper{
			Binary:   cfg.Local.Binary,
			Model:    cfg.Local.Model,
			Language: cfg.Language,
		}, nil
	default:
		return nil, fmt.Errorf("VTT_ENGINE must be cloud or local")
	}
}
