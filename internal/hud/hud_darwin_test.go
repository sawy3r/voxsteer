//go:build desktop && darwin

package hud

import "testing"

func TestStatusTitle(t *testing.T) {
	tests := []struct {
		status    string
		title     string
		temporary bool
	}{
		{"idle", idleTitle, false},
		{"recording", "● Recording", false},
		{"transcribing", "Transcribing…", false},
		{"Copied: press ⌘V to paste", "Copied: press ⌘V to paste", true},
	}
	for _, test := range tests {
		title, temporary := statusTitle(test.status)
		if title != test.title || temporary != test.temporary {
			t.Errorf("statusTitle(%q) = %q, %v; want %q, %v",
				test.status, title, temporary, test.title, test.temporary)
		}
	}
}
