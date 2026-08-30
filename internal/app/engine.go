package app

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/sawy3r/voxsteer/internal/config"
	"github.com/sawy3r/voxsteer/internal/transcriber"
)

// ConfiguredEngine creates the transcription engine selected by cfg. The
// configuration is validated up front so a broken setup fails at startup with
// an actionable message instead of on the first dictation.
func ConfiguredEngine(cfg config.Config) (transcriber.Transcriber, error) {
	switch cfg.Engine {
	case "cloud":
		if cfg.Cloud.Endpoint == "" {
			return nil, fmt.Errorf(`the cloud engine needs a transcription URL: set "cloud.endpoint" in the configuration file or VTT_CLOUD_ENDPOINT`)
		}
		return &transcriber.Cloud{
			Endpoint: cfg.Cloud.Endpoint,
			APIKey:   cfg.Cloud.APIKey,
			Model:    cfg.Cloud.Model,
			Language: cfg.Language,
		}, nil
	case "local":
		if cfg.Local.Model == "" {
			return nil, fmt.Errorf(`the local engine needs a whisper model: set "local.model" in the configuration file or VTT_WHISPER_MODEL to a ggml model file (downloadable from https://huggingface.co/ggerganov/whisper.cpp)`)
		}
		if _, err := os.Stat(cfg.Local.Model); err != nil {
			return nil, fmt.Errorf(`whisper model is not readable: %w; fix "local.model" in the configuration file or download a ggml model from https://huggingface.co/ggerganov/whisper.cpp`, err)
		}
		if err := checkWhisperCLI(cfg.Local.Binary); err != nil {
			return nil, err
		}
		if cfg.Local.Threads < 0 {
			return nil, fmt.Errorf(`"local.threads" in the configuration file (or VTT_WHISPER_THREADS) must be zero for automatic or a positive thread count, not %d`, cfg.Local.Threads)
		}
		return &transcriber.LocalWhisper{
			Binary:   cfg.Local.Binary,
			Model:    cfg.Local.Model,
			Language: cfg.Language,
			Threads:  cfg.Local.Threads,
		}, nil
	default:
		return nil, fmt.Errorf(`unknown engine %q: set "engine" in the configuration file (or VTT_ENGINE) to cloud or local`, cfg.Engine)
	}
}

// checkWhisperCLI verifies that the whisper.cpp CLI the local engine will
// execute actually exists, mirroring the lookup LocalWhisper performs.
func checkWhisperCLI(binary string) error {
	const remedy = `build whisper.cpp (https://github.com/ggml-org/whisper.cpp) and point "local.binary" in the configuration file or VTT_WHISPER_BINARY at the built whisper-cli`
	if binary == "" {
		binary = "whisper-cli"
	}
	if strings.ContainsRune(binary, os.PathSeparator) {
		if _, err := os.Stat(binary); err != nil {
			return fmt.Errorf("whisper.cpp CLI is not usable: %w; %s", err, remedy)
		}
		return nil
	}
	if _, err := exec.LookPath(binary); err != nil {
		return fmt.Errorf("whisper.cpp CLI is not usable: %w; %s", err, remedy)
	}
	return nil
}
