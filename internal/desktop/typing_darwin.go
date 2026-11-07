//go:build desktop && darwin

package desktop

/*
#cgo LDFLAGS: -framework ApplicationServices

#include <ApplicationServices/ApplicationServices.h>

// postText types text as one key press carrying a Unicode string, which
// applications insert as it is, whatever the keyboard layout.
static int postText(const UniChar *text, int length) {
	CGEventSourceRef source = CGEventSourceCreate(kCGEventSourceStateHIDSystemState);
	CGEventRef down = CGEventCreateKeyboardEvent(source, 0, true);
	CGEventRef up = CGEventCreateKeyboardEvent(source, 0, false);
	int ok = down != NULL && up != NULL;
	if (ok) {
		// A modifier still held from the shortcut would otherwise turn the
		// text into key commands.
		CGEventSetFlags(down, 0);
		CGEventSetFlags(up, 0);
		CGEventKeyboardSetUnicodeString(down, length, text);
		CGEventKeyboardSetUnicodeString(up, length, text);
		CGEventPost(kCGHIDEventTap, down);
		CGEventPost(kCGHIDEventTap, up);
	}
	if (down != NULL) {
		CFRelease(down);
	}
	if (up != NULL) {
		CFRelease(up);
	}
	if (source != NULL) {
		CFRelease(source);
	}
	return ok;
}
*/
import "C"

import (
	"errors"
	"time"
	"unicode/utf16"
	"unsafe"
)

// maxEventUnits is the most text one keyboard event is relied on to carry,
// in UTF-16 units.
const maxEventUnits = 20

// chunkPause gives the receiving application a moment between events.
const chunkPause = 2 * time.Millisecond

// typeText sends text in chunks rather than a key press per character, which
// takes several milliseconds each and made long transcripts slow to appear.
func typeText(text string) error {
	for i, chunk := range eventChunks(text) {
		if i > 0 {
			time.Sleep(chunkPause)
		}
		if C.postText((*C.UniChar)(unsafe.Pointer(&chunk[0])), C.int(len(chunk))) == 0 {
			return errors.New("keyboard events could not be created")
		}
	}
	return nil
}

// eventChunks splits text into UTF-16 runs of at most maxEventUnits, never
// separating the two halves of a surrogate pair, so emoji arrive whole.
func eventChunks(text string) [][]uint16 {
	units := utf16.Encode([]rune(text))
	var chunks [][]uint16
	for len(units) > 0 {
		size := min(len(units), maxEventUnits)
		if size < len(units) && isHighSurrogate(units[size-1]) {
			size--
		}
		chunks = append(chunks, units[:size])
		units = units[size:]
	}
	return chunks
}

func isHighSurrogate(unit uint16) bool {
	return unit >= 0xD800 && unit <= 0xDBFF
}
