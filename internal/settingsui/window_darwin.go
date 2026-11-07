//go:build desktop && darwin

package settingsui

/*
#cgo CFLAGS: -x objective-c -fobjc-arc
#cgo LDFLAGS: -framework AppKit -framework Foundation -framework WebKit
#include <stdlib.h>

void openSettingsWindow(const char *html);
void closeSettingsWindow(void);
*/
import "C"

import (
	_ "embed"
	"sync"
	"unsafe"
)

// Available reports whether this platform has a settings window.
const Available = true

//go:embed page.html
var page string

var (
	mu      sync.Mutex
	current Options
)

// Open shows the settings window, or brings it forward if it is already open.
func Open(options Options) {
	mu.Lock()
	current = options
	mu.Unlock()
	html := C.CString(page)
	defer C.free(unsafe.Pointer(html))
	C.openSettingsWindow(html)
}

//export settingsWindowRequest
func settingsWindowRequest(raw *C.char) *C.char {
	mu.Lock()
	options := current
	mu.Unlock()
	reply := handle(options, []byte(C.GoString(raw)), func() { C.closeSettingsWindow() })
	return C.CString(string(reply))
}
