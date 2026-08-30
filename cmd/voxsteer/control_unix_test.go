//go:build desktop && !windows

package main

import (
	"os"
	"testing"
	"time"
)

// shortSocketDir returns a directory whose socket paths stay inside the
// unix socket path limit; macOS puts t.TempDir() beyond it.
func shortSocketDir(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("/tmp", "vox")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	return dir
}

func TestControlSocketToggleRoundTrip(t *testing.T) {
	t.Setenv("XDG_RUNTIME_DIR", shortSocketDir(t))

	toggled := make(chan struct{}, 1)
	stop, err := startControl(func() { toggled <- struct{}{} })
	if err != nil {
		t.Fatal(err)
	}
	defer stop()

	if err := sendToggle(); err != nil {
		t.Fatal(err)
	}
	select {
	case <-toggled:
	case <-time.After(2 * time.Second):
		t.Fatal("toggle was not delivered through the control socket")
	}

	if _, err := startControl(func() {}); err == nil {
		t.Fatal("a second instance was not rejected while the socket is live")
	}
}

func TestSendToggleWithoutRunningInstance(t *testing.T) {
	t.Setenv("XDG_RUNTIME_DIR", shortSocketDir(t))
	if err := sendToggle(); err == nil {
		t.Fatal("expected an error when no instance is running")
	}
}
