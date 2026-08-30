//go:build desktop && !windows

package main

import (
	"testing"
	"time"
)

func TestControlSocketToggleRoundTrip(t *testing.T) {
	t.Setenv("XDG_RUNTIME_DIR", t.TempDir())

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
	t.Setenv("XDG_RUNTIME_DIR", t.TempDir())
	if err := sendToggle(); err == nil {
		t.Fatal("expected an error when no instance is running")
	}
}
