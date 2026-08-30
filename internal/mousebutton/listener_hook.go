//go:build desktop && darwin && cgo

package mousebutton

import (
	"fmt"
	"sync"

	hook "github.com/robotn/gohook"
)

func Listen(button int) (<-chan struct{}, func(), error) {
	if button < 3 || button > 5 {
		return nil, nil, fmt.Errorf("unsupported mouse button %d", button)
	}
	events := make(chan struct{}, 1)
	nativeEvents := hook.Start()
	var stopOnce sync.Once
	stop := func() {
		stopOnce.Do(hook.End)
	}
	go func() {
		defer close(events)
		for event := range nativeEvents {
			if event.Kind == hook.MouseDown && int(event.Button) == button {
				select {
				case events <- struct{}{}:
				default:
				}
			}
		}
	}()
	return events, stop, nil
}
