//go:build desktop && darwin

package doubletap

/*
#cgo LDFLAGS: -framework ApplicationServices -framework CoreFoundation

void *startListener(void);
void stopListener(void *listener);
*/
import "C"

import (
	"errors"
	"sync"
	"sync/atomic"
	"time"
)

// Event kinds reported by listener_darwin.c.
const (
	keyPressed     = 0
	keyReleased    = 1
	keyInterrupted = 2
)

type listener struct {
	mu       sync.Mutex
	detector Detector
	events   chan struct{}
	closed   bool
}

// active is the running listener. The native callback is a plain C function, so
// it finds its Go state here rather than through a pointer it carries.
var active atomic.Pointer[listener]

// Listen reports each double tap of the left Control key. It needs Accessibility
// access, and only one listener runs at a time.
func Listen() (<-chan struct{}, func(), error) {
	current := &listener{events: make(chan struct{}, 1)}
	if !active.CompareAndSwap(nil, current) {
		return nil, nil, errors.New("double-tap activation is already running")
	}
	tap := C.startListener()
	if tap == nil {
		active.Store(nil)
		return nil, nil, errors.New("macOS refused to report key events; check Accessibility access")
	}
	var once sync.Once
	stop := func() {
		once.Do(func() {
			C.stopListener(tap)
			active.Store(nil)
			current.mu.Lock()
			defer current.mu.Unlock()
			current.closed = true
			close(current.events)
		})
	}
	return current.events, stop, nil
}

//export doubleTapKeyEvent
func doubleTapKeyEvent(kind C.int) {
	current := active.Load()
	if current == nil {
		return
	}
	current.mu.Lock()
	defer current.mu.Unlock()
	if current.closed {
		return
	}
	now := time.Now()
	switch kind {
	case keyPressed:
		if current.detector.Press(now) {
			select {
			case current.events <- struct{}{}:
			default:
			}
		}
	case keyReleased:
		current.detector.Release(now)
	default:
		current.detector.Interrupt()
	}
}
