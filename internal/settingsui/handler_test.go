package settingsui

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/sawy3r/voxsteer/internal/settings"
)

func reply(t *testing.T, raw []byte) response {
	t.Helper()
	var value response
	if err := json.Unmarshal(raw, &value); err != nil {
		t.Fatalf("decode %s: %v", raw, err)
	}
	return value
}

func TestLoadNeverSendsTheAPIKeyToThePage(t *testing.T) {
	key := "secret"
	options := Options{Load: func() (settings.Values, error) {
		return settings.Values{Engine: "cloud", HasCloudAPIKey: true, CloudAPIKey: &key}, nil
	}}
	raw := handle(options, []byte(`{"action":"load"}`), func() {})
	if strings.Contains(string(raw), "secret") {
		t.Fatalf("reply exposed the API key: %s", raw)
	}
	got := reply(t, raw)
	if !got.OK || got.Settings == nil || !got.Settings.HasCloudAPIKey {
		t.Fatalf("reply = %+v", got)
	}
}

func TestSaveRunsOnSavedOnlyAfterSuccess(t *testing.T) {
	var saved settings.Values
	savedCount := 0
	options := Options{
		Save: func(values settings.Values) error {
			saved = values
			return nil
		},
		OnSaved: func() { savedCount++ },
	}
	raw := handle(options, []byte(`{"action":"save","settings":{"engine":"local","shortcut":"double-left-ctrl"}}`), func() {})
	if got := reply(t, raw); !got.OK {
		t.Fatalf("reply = %+v", got)
	}
	if saved.Shortcut != "double-left-ctrl" || saved.Engine != "local" {
		t.Fatalf("saved = %+v", saved)
	}
	if savedCount != 1 {
		t.Fatalf("OnSaved ran %d times", savedCount)
	}
}

func TestSaveFailureReachesThePage(t *testing.T) {
	options := Options{
		Save:    func(settings.Values) error { return errors.New("local Whisper model is not a readable file") },
		OnSaved: func() { t.Fatal("OnSaved ran after a failed save") },
	}
	got := reply(t, handle(options, []byte(`{"action":"save"}`), func() {}))
	if got.OK || got.Error != "local Whisper model is not a readable file" {
		t.Fatalf("reply = %+v", got)
	}
}

func TestCloseClosesTheWindow(t *testing.T) {
	closed := false
	handle(Options{}, []byte(`{"action":"close"}`), func() { closed = true })
	if !closed {
		t.Fatal("window was not closed")
	}
}

func TestBadRequestsAreReported(t *testing.T) {
	for _, raw := range []string{"not json", `{"action":"explode"}`} {
		if got := reply(t, handle(Options{}, []byte(raw), func() {})); got.OK || got.Error == "" {
			t.Fatalf("handle(%q) = %+v", raw, got)
		}
	}
}
