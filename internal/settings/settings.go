// Package settings reads and writes the options people change through a user
// interface, so every interface applies the same validation.
package settings

import (
	"fmt"
	"net"
	"net/url"
	"os"
	"os/exec"
	"strings"
	"sync"
	"unicode"

	"github.com/sawy3r/voxsteer/internal/config"
	"github.com/sawy3r/voxsteer/internal/shortcut"
)

// Values is the editable configuration. The saved API key is never read back:
// HasCloudAPIKey reports its presence, and CloudAPIKey is only ever an incoming
// replacement, where nil keeps the saved key. An empty Shortcut also keeps the
// saved shortcut, because not every interface offers one.
type Values struct {
	Engine         string  `json:"engine"`
	Language       string  `json:"language"`
	CloudEndpoint  string  `json:"cloud_endpoint"`
	CloudModel     string  `json:"cloud_model"`
	HasCloudAPIKey bool    `json:"has_cloud_api_key"`
	CloudAPIKey    *string `json:"cloud_api_key,omitempty"`
	LocalBinary    string  `json:"local_binary"`
	LocalModel     string  `json:"local_model"`
	Shortcut       string  `json:"shortcut,omitempty"`
}

var saveMu sync.Mutex

// Load returns the current settings.
func Load() (Values, error) {
	cfg, _, err := config.Load()
	if err != nil {
		return Values{}, err
	}
	return Values{
		Engine:         cfg.Engine,
		Language:       cfg.Language,
		CloudEndpoint:  cfg.Cloud.Endpoint,
		CloudModel:     cfg.Cloud.Model,
		HasCloudAPIKey: cfg.Cloud.APIKey != "",
		LocalBinary:    cfg.Local.Binary,
		LocalModel:     cfg.Local.Model,
		Shortcut:       cfg.Shortcut,
	}, nil
}

// Save validates values and writes them to the per-user configuration file. It
// refuses when an environment variable or another file controls the setting,
// because the saved value would silently have no effect.
func Save(values Values) error {
	saveMu.Lock()
	defer saveMu.Unlock()

	for _, name := range []string{
		"VTT_CONFIG",
		"VTT_ENGINE",
		"VTT_LANGUAGE",
		"VTT_WHISPER_BINARY",
		"VTT_WHISPER_MODEL",
		"VTT_CLOUD_ENDPOINT",
		"VTT_CLOUD_API_KEY",
		"VTT_CLOUD_MODEL",
		"VTT_SHORTCUT",
	} {
		if _, set := os.LookupEnv(name); set {
			return fmt.Errorf("settings are controlled by %s; remove that environment override before saving", name)
		}
	}
	cfg, loadedPath, err := config.Load()
	if err != nil {
		return err
	}
	userPath, err := config.UserPath()
	if err != nil {
		return err
	}
	if loadedPath != "" && loadedPath != userPath {
		return fmt.Errorf("settings are controlled by %s; update that file directly", loadedPath)
	}

	cfg.Engine = strings.TrimSpace(values.Engine)
	cfg.Language = strings.TrimSpace(values.Language)
	cfg.Cloud.Endpoint = strings.TrimSpace(values.CloudEndpoint)
	cfg.Cloud.Model = strings.TrimSpace(values.CloudModel)
	cfg.Local.Binary = strings.TrimSpace(values.LocalBinary)
	cfg.Local.Model = strings.TrimSpace(values.LocalModel)
	if values.CloudAPIKey != nil {
		cfg.Cloud.APIKey = strings.TrimSpace(*values.CloudAPIKey)
	}
	if value := strings.TrimSpace(values.Shortcut); value != "" {
		cfg.Shortcut = value
	}
	if cfg.Cloud.Model == "" {
		cfg.Cloud.Model = "whisper-1"
	}
	if err := Validate(cfg); err != nil {
		return err
	}
	_, err = config.SaveUser(cfg)
	return err
}

// Validate reports the first setting in cfg that cannot work.
func Validate(cfg config.Config) error {
	if len(cfg.Language) > 32 || containsControl(cfg.Language) {
		return fmt.Errorf("language must be at most 32 characters and contain no control characters")
	}
	if len(cfg.Cloud.APIKey) > 8192 {
		return fmt.Errorf("cloud API key exceeds 8192 characters")
	}
	if _, err := shortcut.Parse(cfg.Shortcut); err != nil {
		return fmt.Errorf("activation shortcut: %w", err)
	}
	switch cfg.Engine {
	case "cloud":
		if len(cfg.Cloud.Endpoint) > 4096 {
			return fmt.Errorf("cloud endpoint exceeds 4096 characters")
		}
		endpoint, err := url.ParseRequestURI(cfg.Cloud.Endpoint)
		if err != nil {
			return fmt.Errorf("cloud endpoint must be a valid HTTPS URL")
		}
		if err := ValidateCloudURL(endpoint); err != nil {
			return err
		}
		if len(cfg.Cloud.Model) > 256 || containsControl(cfg.Cloud.Model) {
			return fmt.Errorf("cloud model must be at most 256 characters and contain no control characters")
		}
	case "local":
		if cfg.Local.Model == "" {
			return fmt.Errorf("local Whisper model path is required")
		}
		if len(cfg.Local.Model) > 4096 || len(cfg.Local.Binary) > 4096 {
			return fmt.Errorf("local Whisper paths exceed 4096 characters")
		}
		modelInfo, err := os.Stat(cfg.Local.Model)
		if err != nil || !modelInfo.Mode().IsRegular() {
			return fmt.Errorf("local Whisper model is not a readable file")
		}
		binary := cfg.Local.Binary
		if binary == "" {
			binary = "whisper-cli"
		}
		if _, err := exec.LookPath(binary); err != nil {
			return fmt.Errorf("local whisper-cli executable was not found")
		}
	default:
		return fmt.Errorf("transcription engine must be cloud or local")
	}
	return nil
}

// ValidateCloudURL accepts HTTPS, and plain HTTP only on loopback.
func ValidateCloudURL(endpoint *url.URL) error {
	if endpoint == nil || endpoint.Host == "" || endpoint.User != nil {
		return fmt.Errorf("cloud endpoint must be a valid URL without embedded credentials")
	}
	if endpoint.Scheme == "https" {
		return nil
	}
	if endpoint.Scheme != "http" {
		return fmt.Errorf("cloud endpoint must use HTTPS")
	}
	host := endpoint.Hostname()
	if strings.EqualFold(host, "localhost") {
		return nil
	}
	ip := net.ParseIP(host)
	if ip == nil || !ip.IsLoopback() {
		return fmt.Errorf("cloud endpoint must use HTTPS unless it is on loopback")
	}
	return nil
}

func containsControl(value string) bool {
	return strings.IndexFunc(value, unicode.IsControl) >= 0
}
