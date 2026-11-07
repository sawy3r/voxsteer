//go:build desktop && darwin

package hud

import (
	"sync"
	"time"

	"fyne.io/systray"
)

const (
	idleTitle      = "VoxSteer"
	errorTitle     = "VoxSteer: see log"
	noticeDuration = 8 * time.Second
)

// Overlay reports dictation state through the menu bar title. It is the only
// surface VoxSteer.app has, because the application runs without a window.
type Overlay struct {
	mu         sync.Mutex
	generation int
}

func New() (*Overlay, error) {
	return &Overlay{}, nil
}

func (o *Overlay) SetStatus(status string) {
	title, temporary := statusTitle(status)
	o.show(title, temporary)
}

func (*Overlay) SetLevel(float32) {}

func (o *Overlay) SetError() {
	o.show(errorTitle, true)
}

func (*Overlay) Close() {}

// statusTitle maps a dictation status to the menu bar title. Anything that is
// not a known status is a notice, which clears itself after a while.
func statusTitle(status string) (title string, temporary bool) {
	switch status {
	case "idle":
		return idleTitle, false
	case "recording":
		return "● Recording", false
	case "transcribing":
		return "Transcribing…", false
	default:
		return status, true
	}
}

func (o *Overlay) show(title string, temporary bool) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.generation++
	systray.SetTitle(title)
	if !temporary {
		return
	}
	generation := o.generation
	time.AfterFunc(noticeDuration, func() {
		o.mu.Lock()
		defer o.mu.Unlock()
		if o.generation == generation {
			systray.SetTitle(idleTitle)
		}
	})
}
