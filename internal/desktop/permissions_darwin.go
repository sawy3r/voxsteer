//go:build desktop && darwin

package desktop

/*
#cgo CFLAGS: -x objective-c
#cgo LDFLAGS: -framework AppKit -framework ApplicationServices -framework AVFoundation -framework Carbon -framework Foundation

#import <AppKit/AppKit.h>
#import <ApplicationServices/ApplicationServices.h>
#import <AVFoundation/AVFoundation.h>
#import <Carbon/Carbon.h>
#include <stdlib.h>

static int inApplicationBundle(void) {
	@autoreleasepool {
		return [[NSBundle mainBundle] bundleIdentifier] != nil;
	}
}

static int microphoneStatus(void) {
	@autoreleasepool {
		return (int)[AVCaptureDevice authorizationStatusForMediaType:AVMediaTypeAudio];
	}
}

static void requestMicrophone(void) {
	[AVCaptureDevice requestAccessForMediaType:AVMediaTypeAudio
	                         completionHandler:^(BOOL granted) {}];
}

static int accessibilityTrusted(void) {
	return AXIsProcessTrusted() ? 1 : 0;
}

static void promptAccessibility(void) {
	@autoreleasepool {
		NSDictionary *options = @{(__bridge id)kAXTrustedCheckOptionPrompt: @YES};
		AXIsProcessTrustedWithOptions((__bridge CFDictionaryRef)options);
	}
}

// The Accessibility prompt waits for the microphone answer so that the two
// system dialogs do not stack on top of each other.
static void requestPermissions(void) {
	if (microphoneStatus() == AVAuthorizationStatusNotDetermined) {
		[AVCaptureDevice requestAccessForMediaType:AVMediaTypeAudio
		                         completionHandler:^(BOOL granted) {
			promptAccessibility();
		}];
		return;
	}
	promptAccessibility();
}

static int secureInputEnabled(void) {
	return IsSecureEventInputEnabled() ? 1 : 0;
}

static int writeClipboard(const char *text) {
	@autoreleasepool {
		NSString *value = [NSString stringWithUTF8String:text];
		if (value == nil) {
			return 0;
		}
		NSPasteboard *board = [NSPasteboard generalPasteboard];
		[board clearContents];
		return [board setString:value forType:NSPasteboardTypeString] ? 1 : 0;
	}
}
*/
import "C"

import (
	"errors"
	"time"
	"unsafe"

	"github.com/sawy3r/voxsteer/internal/app"
)

// Values of AVAuthorizationStatus.
const (
	microphoneNotDetermined = 0
	microphoneAuthorized    = 3
)

var (
	accessibilityPermission = Permission{
		Name:        "Accessibility",
		SettingsURL: "x-apple.systempreferences:com.apple.preference.security?Privacy_Accessibility",
	}
	microphonePermission = Permission{
		Name:        "Microphone",
		SettingsURL: "x-apple.systempreferences:com.apple.preference.security?Privacy_Microphone",
	}
)

// RequestPermissions raises the macOS consent prompts at launch, so the first
// dictation is not the moment they appear. It only acts inside VoxSteer.app:
// macOS attributes a binary run from a shell to the terminal, and prompting
// there would hand the terminal control of the Mac rather than VoxSteer.
func RequestPermissions() {
	if C.inApplicationBundle() != 0 {
		C.requestPermissions()
	}
}

// InputAccessGranted reports whether global shortcuts and typing are allowed.
func InputAccessGranted() bool {
	return C.accessibilityTrusted() != 0
}

// MissingPermission returns the first permission dictation still needs.
func MissingPermission() Permission {
	if C.microphoneStatus() != microphoneAuthorized {
		return microphonePermission
	}
	if !InputAccessGranted() {
		return accessibilityPermission
	}
	return Permission{}
}

// WatchPermissions calls onChange whenever the missing permission changes. It
// never returns, because access can also be revoked while VoxSteer runs.
func WatchPermissions(onChange func(Permission)) {
	var last Permission
	for {
		if current := MissingPermission(); current != last {
			last = current
			onChange(current)
		}
		time.Sleep(permissionPollInterval)
	}
}

// microphoneReady stops a recording before it reaches the audio device. Opening
// the device while the consent prompt is unanswered blocks inside CoreAudio
// until the user responds, and neither stopping dictation nor a timeout can
// interrupt that.
func microphoneReady() error {
	switch C.microphoneStatus() {
	case microphoneAuthorized:
		return nil
	case microphoneNotDetermined:
		C.requestMicrophone()
		return &app.Notice{Message: "Allow the microphone, then retry"}
	default:
		return &app.Notice{Message: "Microphone access is off"}
	}
}

// typingBlocked explains why macOS would silently drop posted keystrokes, or
// returns an empty string when typing should work.
func typingBlocked() string {
	if !InputAccessGranted() {
		return "Accessibility access is missing"
	}
	if C.secureInputEnabled() != 0 {
		return "secure input is on"
	}
	return ""
}

// writeClipboard uses NSPasteboard rather than pbcopy, which misreads UTF-8 in
// applications launched from Finder because they have no locale set.
func writeClipboard(text string) error {
	value := C.CString(text)
	defer C.free(unsafe.Pointer(value))
	if C.writeClipboard(value) == 0 {
		return errors.New("the pasteboard rejected the transcript")
	}
	return nil
}
