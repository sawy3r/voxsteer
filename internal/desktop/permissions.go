//go:build desktop

package desktop

import "time"

const permissionPollInterval = time.Second

// Permission names something the operating system must allow before dictation
// works. The zero value means nothing is missing.
type Permission struct {
	Name        string
	SettingsURL string
}

// WaitForInputAccess blocks until global input is allowed. It reports false if
// cancel closes first.
func WaitForInputAccess(cancel <-chan struct{}) bool {
	return waitUntil(InputAccessGranted, permissionPollInterval, cancel)
}

func waitUntil(ready func() bool, interval time.Duration, cancel <-chan struct{}) bool {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for !ready() {
		select {
		case <-cancel:
			return false
		case <-ticker.C:
		}
	}
	return true
}
