//go:build desktop

package desktop

import (
	"testing"
	"time"
)

func TestWaitUntilReturnsOnceReady(t *testing.T) {
	checks := 0
	ready := func() bool {
		checks++
		return checks == 3
	}
	if !waitUntil(ready, time.Millisecond, make(chan struct{})) {
		t.Fatal("waitUntil reported cancellation")
	}
	if checks != 3 {
		t.Fatalf("checks = %d, want 3", checks)
	}
}

func TestWaitUntilStopsWhenCancelled(t *testing.T) {
	cancel := make(chan struct{})
	close(cancel)
	if waitUntil(func() bool { return false }, time.Hour, cancel) {
		t.Fatal("waitUntil reported success after cancellation")
	}
}
