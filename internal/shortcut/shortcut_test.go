package shortcut

import "testing"

func TestParse(t *testing.T) {
	tests := []struct {
		value       string
		wantDisplay string
		wantKey     string
	}{
		{"", "Ctrl+Alt+Space", "SPACE"},
		{"ctrl+shift+d", "Ctrl+Shift+D", "D"},
		{"F8", "F8", "F8"},
		{"disabled", "Mouse / tray only", ""},
		{"cmd+alt+v", "Alt+Win/Cmd+V", "V"},
	}
	for _, test := range tests {
		t.Run(test.value, func(t *testing.T) {
			got, err := Parse(test.value)
			if err != nil {
				t.Fatal(err)
			}
			if got.Display != test.wantDisplay || got.Key != test.wantKey {
				t.Fatalf("Parse(%q) = %#v", test.value, got)
			}
		})
	}
}

func TestParseRejectsInvalidShortcuts(t *testing.T) {
	for _, value := range []string{"s", "ctrl", "ctrl+s+d", "ctrl+escape", "ctrl+ctrl+s"} {
		t.Run(value, func(t *testing.T) {
			if _, err := Parse(value); err == nil {
				t.Fatalf("Parse(%q) succeeded", value)
			}
		})
	}
}
