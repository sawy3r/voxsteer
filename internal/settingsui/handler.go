// Package settingsui shows the native settings window.
package settingsui

import (
	"encoding/json"

	"github.com/sawy3r/voxsteer/internal/settings"
)

// Options connects the window to the application.
type Options struct {
	Load func() (settings.Values, error)
	Save func(settings.Values) error
	// OnSaved runs after a successful save. It is called on the main thread, so
	// it must not block: re-registering a shortcut there would deadlock.
	OnSaved func()
}

type request struct {
	Action   string          `json:"action"`
	Settings settings.Values `json:"settings"`
}

type response struct {
	OK       bool             `json:"ok"`
	Settings *settings.Values `json:"settings,omitempty"`
	Error    string           `json:"error,omitempty"`
}

// handle answers one message from the page. The page and this function are the
// whole interface between the window and the application.
func handle(options Options, raw []byte, closeWindow func()) []byte {
	var message request
	if err := json.Unmarshal(raw, &message); err != nil {
		return encode(response{Error: "The settings window sent an unreadable request."})
	}
	switch message.Action {
	case "load":
		values, err := options.Load()
		if err != nil {
			return encode(response{Error: err.Error()})
		}
		values.CloudAPIKey = nil
		return encode(response{OK: true, Settings: &values})
	case "save":
		if err := options.Save(message.Settings); err != nil {
			return encode(response{Error: err.Error()})
		}
		if options.OnSaved != nil {
			options.OnSaved()
		}
		return encode(response{OK: true})
	case "close":
		closeWindow()
		return encode(response{OK: true})
	default:
		return encode(response{Error: "The settings window sent an unknown request."})
	}
}

func encode(value response) []byte {
	encoded, err := json.Marshal(value)
	if err != nil {
		return []byte(`{"ok":false,"error":"VoxSteer could not encode its reply."}`)
	}
	return encoded
}
