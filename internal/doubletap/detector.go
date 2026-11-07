// Package doubletap recognises a deliberate double tap of the left Control key.
package doubletap

import "time"

const (
	// maxHold is the longest press that still counts as a tap. Holding Control
	// longer means it is being used as a modifier.
	maxHold = 300 * time.Millisecond
	// maxGap is the longest pause between the two taps.
	maxGap = 350 * time.Millisecond
)

type detectorState int

const (
	idle detectorState = iota
	firstPress
	betweenTaps
)

// Detector recognises the gesture from a stream of events. Keeping it free of
// operating system types lets its timing rules be tested directly. The caller
// passes every press and release of the key, and calls Interrupt for anything
// else, so that Ctrl+C followed by Ctrl+V or two Ctrl-clicks do not count.
type Detector struct {
	state detectorState
	at    time.Time
}

// Press records the key going down and reports whether it completes a double
// tap. The gesture fires on the second press so dictation starts promptly.
func (d *Detector) Press(now time.Time) bool {
	if d.state == betweenTaps && now.Sub(d.at) <= maxGap {
		d.state = idle
		return true
	}
	d.state = firstPress
	d.at = now
	return false
}

// Release records the key coming back up.
func (d *Detector) Release(now time.Time) {
	if d.state == firstPress && now.Sub(d.at) <= maxHold {
		d.state = betweenTaps
		d.at = now
		return
	}
	d.state = idle
}

// Interrupt abandons any gesture in progress.
func (d *Detector) Interrupt() {
	d.state = idle
}
