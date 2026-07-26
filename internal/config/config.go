package config

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

type Config struct {
	Engine   string `json:"engine"`
	Language string `json:"language"`
	Shortcut string `json:"shortcut"`
	Local    struct {
		Binary string `json:"binary"`
		Model  string `json:"model"`
	} `json:"local"`
	Cloud struct {
		Endpoint string `json:"endpoint"`
		APIKey   string `json:"api_key"`
		Model    string `json:"model"`
	} `json:"cloud"`
}

// Load reads config.json from an explicit VTT_CONFIG path, beside the
// executable, or from the user's config directory, in that order. Environment
// variables override file values.
func Load() (Config, string, error) {
	var cfg Config
	cfg.Engine = "local"
	cfg.Cloud.Model = "whisper-1"
	cfg.Shortcut = "ctrl+alt+space"

	path, required, err := configPath()
	if err != nil {
		return cfg, "", err
	}
	if path != "" {
		if err := decode(path, &cfg); err != nil {
			if required || !os.IsNotExist(err) {
				return cfg, path, err
			}
			path = ""
		}
	}

	applyEnvironment(&cfg)
	if path != "" {
		base := filepath.Dir(path)
		cfg.Local.Binary = resolvePath(base, cfg.Local.Binary)
		cfg.Local.Model = resolvePath(base, cfg.Local.Model)
	}
	return cfg, path, nil
}

func configPath() (path string, required bool, err error) {
	if explicit := os.Getenv("VTT_CONFIG"); explicit != "" {
		absolute, err := filepath.Abs(explicit)
		return absolute, true, err
	}

	executable, err := os.Executable()
	if err == nil {
		portable := filepath.Join(filepath.Dir(executable), "config.json")
		if _, statErr := os.Stat(portable); statErr == nil {
			return portable, false, nil
		}
	}

	configDir, err := os.UserConfigDir()
	if err != nil {
		return "", false, nil
	}
	userPath := filepath.Join(configDir, "VoxSteer", "config.json")
	if _, statErr := os.Stat(userPath); statErr == nil {
		return userPath, false, nil
	}
	return "", false, nil
}

func decode(path string, cfg *Config) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()

	decoder := json.NewDecoder(io.LimitReader(file, 1<<20))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(cfg); err != nil {
		return fmt.Errorf("decode %s: %w", path, err)
	}
	return nil
}

func applyEnvironment(cfg *Config) {
	override(&cfg.Engine, "VTT_ENGINE")
	override(&cfg.Language, "VTT_LANGUAGE")
	override(&cfg.Local.Binary, "VTT_WHISPER_BINARY")
	override(&cfg.Local.Model, "VTT_WHISPER_MODEL")
	override(&cfg.Cloud.Endpoint, "VTT_CLOUD_ENDPOINT")
	override(&cfg.Cloud.APIKey, "VTT_CLOUD_API_KEY")
	override(&cfg.Cloud.Model, "VTT_CLOUD_MODEL")
	override(&cfg.Shortcut, "VTT_SHORTCUT")
}

func override(target *string, name string) {
	if value, ok := os.LookupEnv(name); ok {
		*target = value
	}
}

func resolvePath(base, value string) string {
	if value == "" || filepath.IsAbs(value) {
		return value
	}
	return filepath.Clean(filepath.Join(base, value))
}
