package doubletap

import (
	"testing"
	"time"
)

const ms = time.Millisecond

// script drives a Detector and returns how many gestures fired. Events are
// offsets from the start: p presses, r releases, x interrupts.
type event struct {
	kind byte
	at   time.Duration
}

func fired(events ...event) int {
	var detector Detector
	start := time.Unix(1000, 0)
	count := 0
	for _, e := range events {
		now := start.Add(e.at)
		switch e.kind {
		case 'p':
			if detector.Press(now) {
				count++
			}
		case 'r':
			detector.Release(now)
		case 'x':
			detector.Interrupt()
		}
	}
	return count
}

func TestDetector(t *testing.T) {
	tests := []struct {
		name   string
		events []event
		want   int
	}{
		{
			name:   "a quick double tap fires on the second press",
			events: []event{{'p', 0}, {'r', 80 * ms}, {'p', 200 * ms}},
			want:   1,
		},
		{
			name:   "a slow but deliberate double tap fires",
			events: []event{{'p', 0}, {'r', 300 * ms}, {'p', 650 * ms}},
			want:   1,
		},
		{
			name:   "one tap does nothing",
			events: []event{{'p', 0}, {'r', 80 * ms}},
			want:   0,
		},
		{
			name:   "too long a pause between taps does nothing",
			events: []event{{'p', 0}, {'r', 80 * ms}, {'p', 431 * ms}},
			want:   0,
		},
		{
			name:   "holding the key as a modifier does nothing",
			events: []event{{'p', 0}, {'r', 301 * ms}, {'p', 400 * ms}},
			want:   0,
		},
		{
			name:   "a late second tap can start a new gesture",
			events: []event{{'p', 0}, {'r', 80 * ms}, {'p', 1000 * ms}, {'r', 1080 * ms}, {'p', 1200 * ms}},
			want:   1,
		},
		{
			name:   "a keystroke during the first press cancels, as in Ctrl+C",
			events: []event{{'p', 0}, {'x', 40 * ms}, {'r', 80 * ms}, {'p', 200 * ms}},
			want:   0,
		},
		{
			name:   "a keystroke between taps cancels",
			events: []event{{'p', 0}, {'r', 80 * ms}, {'x', 120 * ms}, {'p', 200 * ms}},
			want:   0,
		},
		{
			name:   "two Ctrl-clicks do not count",
			events: []event{{'p', 0}, {'x', 30 * ms}, {'r', 60 * ms}, {'p', 150 * ms}, {'x', 180 * ms}, {'r', 210 * ms}},
			want:   0,
		},
		{
			name:   "a third tap does not fire again",
			events: []event{{'p', 0}, {'r', 60 * ms}, {'p', 120 * ms}, {'r', 180 * ms}, {'p', 240 * ms}},
			want:   1,
		},
		{
			name:   "four quick taps are two gestures",
			events: []event{{'p', 0}, {'r', 60 * ms}, {'p', 120 * ms}, {'r', 180 * ms}, {'p', 240 * ms}, {'r', 300 * ms}, {'p', 360 * ms}},
			want:   2,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := fired(test.events...); got != test.want {
				t.Fatalf("fired %d times, want %d", got, test.want)
			}
		})
	}
}
