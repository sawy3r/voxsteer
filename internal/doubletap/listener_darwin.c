//go:build desktop && darwin

#include <ApplicationServices/ApplicationServices.h>
#include <stdlib.h>

#include "_cgo_export.h"

// Kinds shared with listener_darwin.go.
#define KEY_PRESSED 0
#define KEY_RELEASED 1
#define KEY_INTERRUPTED 2

#define LEFT_CONTROL_KEYCODE 59
// NX_DEVICELCTLKEYMASK: distinguishes the left key from the right, which share
// the ordinary Control flag.
#define LEFT_CONTROL_DEVICE_MASK 0x00000001

typedef struct {
	CFMachPortRef port;
	CFRunLoopSourceRef source;
} listener;

// The tap observes only. It always returns the event unchanged, so a double tap
// still reaches the focused application as two harmless Control presses.
static CGEventRef tapCallback(CGEventTapProxy proxy, CGEventType type,
                              CGEventRef event, void *userInfo) {
	listener *l = (listener *)userInfo;

	// The system disables a tap that is too slow or interrupted; re-enable it.
	if (type == kCGEventTapDisabledByTimeout ||
	    type == kCGEventTapDisabledByUserInput) {
		CGEventTapEnable(l->port, true);
		return event;
	}

	if (type == kCGEventFlagsChanged &&
	    CGEventGetIntegerValueField(event, kCGKeyboardEventKeycode) == LEFT_CONTROL_KEYCODE) {
		int down = (CGEventGetFlags(event) & LEFT_CONTROL_DEVICE_MASK) != 0;
		doubleTapKeyEvent(down ? KEY_PRESSED : KEY_RELEASED);
		return event;
	}

	doubleTapKeyEvent(KEY_INTERRUPTED);
	return event;
}

void *startListener(void) {
	if (!AXIsProcessTrusted()) {
		return NULL;
	}
	listener *l = calloc(1, sizeof(listener));
	if (l == NULL) {
		return NULL;
	}
	CGEventMask mask = CGEventMaskBit(kCGEventFlagsChanged) |
	                   CGEventMaskBit(kCGEventKeyDown) |
	                   CGEventMaskBit(kCGEventLeftMouseDown) |
	                   CGEventMaskBit(kCGEventRightMouseDown) |
	                   CGEventMaskBit(kCGEventOtherMouseDown);
	// kCGEventTapOptionDefault rather than ListenOnly: a listen-only tap can
	// require the separate Input Monitoring permission, and Accessibility is
	// the one VoxSteer already holds.
	l->port = CGEventTapCreate(kCGSessionEventTap, kCGHeadInsertEventTap,
	                           kCGEventTapOptionDefault, mask, tapCallback, l);
	if (l->port == NULL) {
		free(l);
		return NULL;
	}
	l->source = CFMachPortCreateRunLoopSource(kCFAllocatorDefault, l->port, 0);
	CFRunLoopAddSource(CFRunLoopGetMain(), l->source, kCFRunLoopCommonModes);
	CGEventTapEnable(l->port, true);
	return l;
}

void stopListener(void *value) {
	listener *l = (listener *)value;
	if (l == NULL) {
		return;
	}
	CGEventTapEnable(l->port, false);
	CFRunLoopRemoveSource(CFRunLoopGetMain(), l->source, kCFRunLoopCommonModes);
	CFRelease(l->source);
	CFMachPortInvalidate(l->port);
	CFRelease(l->port);
	free(l);
}
