//go:build linux

package activation

import (
	"bufio"
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"github.com/sawy3r/voxsteer/internal/shortcut"
)

// The listener reads the kernel input devices directly so the shortcut works
// on Wayland and X11 alike. It needs read access to /dev/input/event*, which
// distributions grant to members of the input group.

const (
	evKey = 0x01

	keyReleased = 0
	keyPressed  = 1

	keyLeftCtrl   = 29
	keyRightCtrl  = 97
	keyLeftAlt    = 56
	keyRightAlt   = 100
	keyLeftShift  = 42
	keyRightShift = 54
	keyLeftMeta   = 125
	keyRightMeta  = 126

	btnMiddle = 274 // mouse3
	btnSide   = 275 // mouse4
	btnExtra  = 276 // mouse5

	rescanInterval = 15 * time.Second
)

var keyCodes = map[string]uint16{
	"SPACE": 57,
	"A":     30, "B": 48, "C": 46, "D": 32, "E": 18, "F": 33,
	"G": 34, "H": 35, "I": 23, "J": 36, "K": 37, "L": 38,
	"M": 50, "N": 49, "O": 24, "P": 25, "Q": 16, "R": 19,
	"S": 31, "T": 20, "U": 22, "V": 47, "W": 17, "X": 45,
	"Y": 21, "Z": 44,
	"1": 2, "2": 3, "3": 4, "4": 5, "5": 6,
	"6": 7, "7": 8, "8": 9, "9": 10, "0": 11,
	"F1": 59, "F2": 60, "F3": 61, "F4": 62, "F5": 63, "F6": 64,
	"F7": 65, "F8": 66, "F9": 67, "F10": 68, "F11": 87, "F12": 88,
}

// inputEvent matches struct input_event on 64-bit kernels.
type inputEvent struct {
	Sec   int64
	Usec  int64
	Type  uint16
	Code  uint16
	Value int32
}

type listener struct {
	spec   shortcut.Spec
	target uint16

	events chan struct{}
	done   chan struct{}

	mu      sync.Mutex
	devices map[string]*os.File
	pressed map[uint16]bool
	stopped bool
}

// Listen fires an event whenever the configured shortcut is pressed on any
// input device. The returned stop function releases every device.
func Listen(spec shortcut.Spec) (<-chan struct{}, func(), error) {
	target, err := targetCode(spec)
	if err != nil {
		return nil, nil, err
	}
	l := &listener{
		spec:    spec,
		target:  target,
		events:  make(chan struct{}, 1),
		done:    make(chan struct{}),
		devices: make(map[string]*os.File),
		pressed: make(map[uint16]bool),
	}
	opened, err := l.scan()
	if err != nil {
		return nil, nil, err
	}
	if opened == 0 {
		return nil, nil, fmt.Errorf("no readable devices in /dev/input")
	}
	go l.rescanLoop()
	return l.events, l.stop, nil
}

func targetCode(spec shortcut.Spec) (uint16, error) {
	if spec.MouseButton != 0 {
		switch spec.MouseButton {
		case 3:
			return btnMiddle, nil
		case 4:
			return btnSide, nil
		case 5:
			return btnExtra, nil
		}
		return 0, fmt.Errorf("unsupported mouse button %d", spec.MouseButton)
	}
	code, ok := keyCodes[spec.Key]
	if !ok {
		return 0, fmt.Errorf("unsupported shortcut key %q", spec.Key)
	}
	return code, nil
}

// scan opens input devices that are not open yet and reports how many
// devices are open in total.
func (l *listener) scan() (int, error) {
	paths, err := filepath.Glob("/dev/input/event*")
	if err != nil {
		return 0, err
	}
	sort.Strings(paths)

	l.mu.Lock()
	defer l.mu.Unlock()
	if l.stopped {
		return len(l.devices), nil
	}
	for _, path := range paths {
		if _, open := l.devices[path]; open {
			continue
		}
		file, err := os.Open(path)
		if err != nil {
			continue
		}
		l.devices[path] = file
		go l.readLoop(path, file)
	}
	return len(l.devices), nil
}

func (l *listener) rescanLoop() {
	ticker := time.NewTicker(rescanInterval)
	defer ticker.Stop()
	for {
		select {
		case <-l.done:
			return
		case <-ticker.C:
			_, _ = l.scan()
		}
	}
}

func (l *listener) readLoop(path string, file *os.File) {
	defer func() {
		l.mu.Lock()
		if l.devices[path] == file {
			delete(l.devices, path)
		}
		l.mu.Unlock()
		_ = file.Close()
	}()

	reader := bufio.NewReaderSize(file, 4096)
	var event inputEvent
	for {
		if err := binary.Read(reader, binary.LittleEndian, &event); err != nil {
			if err == io.EOF {
				return
			}
			select {
			case <-l.done:
			default:
			}
			return
		}
		if event.Type != evKey {
			continue
		}
		l.handleKey(event.Code, event.Value)
	}
}

func (l *listener) handleKey(code uint16, value int32) {
	l.mu.Lock()
	switch code {
	case keyLeftCtrl, keyRightCtrl, keyLeftAlt, keyRightAlt,
		keyLeftShift, keyRightShift, keyLeftMeta, keyRightMeta:
		if value == keyReleased {
			delete(l.pressed, code)
		} else {
			l.pressed[code] = true
		}
	}
	matched := code == l.target && value == keyPressed && l.modifiersMatch()
	l.mu.Unlock()

	if matched {
		select {
		case l.events <- struct{}{}:
		default:
		}
	}
}

// modifiersMatch requires the held modifiers to be exactly the configured
// ones so overlapping shortcuts do not both fire. Mouse buttons carry no
// modifiers and always match.
func (l *listener) modifiersMatch() bool {
	if l.spec.MouseButton != 0 {
		return true
	}
	ctrl := l.pressed[keyLeftCtrl] || l.pressed[keyRightCtrl]
	alt := l.pressed[keyLeftAlt] || l.pressed[keyRightAlt]
	shift := l.pressed[keyLeftShift] || l.pressed[keyRightShift]
	super := l.pressed[keyLeftMeta] || l.pressed[keyRightMeta]
	return ctrl == l.spec.Ctrl && alt == l.spec.Alt &&
		shift == l.spec.Shift && super == l.spec.Super
}

func (l *listener) stop() {
	l.mu.Lock()
	if l.stopped {
		l.mu.Unlock()
		return
	}
	l.stopped = true
	files := make([]*os.File, 0, len(l.devices))
	for _, file := range l.devices {
		files = append(files, file)
	}
	l.mu.Unlock()

	close(l.done)
	for _, file := range files {
		_ = file.Close()
	}
}
