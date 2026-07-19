//go:build darwin

package hotkey

// macOS gates the global keyboard hook behind the Accessibility permission.
// gohook checks it too, but only from inside its async start path, where a
// refusal is reported to stderr and then turns into a crash at teardown (see
// endHook in listener.go). Checking it up front lets the listener stay idle
// instead of installing a hook the OS has already refused.

/*
#cgo LDFLAGS: -framework ApplicationServices

#include <ApplicationServices/ApplicationServices.h>
#include <stdbool.h>

static bool mogi_accessibility_trusted(void) {
	// Mirrors libuiohook's own is_accessibility_enabled(), prompt included: it
	// passes kAXTrustedCheckOptionPrompt=true, so a user who has not granted
	// the permission still gets the system "open Accessibility settings"
	// dialog they used to get from gohook. macOS shows it only while the
	// process is untrusted, so a granted app never sees it.
	const void *keys[] = {kAXTrustedCheckOptionPrompt};
	const void *values[] = {kCFBooleanTrue};
	CFDictionaryRef options = CFDictionaryCreate(
		kCFAllocatorDefault, keys, values, 1,
		&kCFCopyStringDictionaryKeyCallBacks, &kCFTypeDictionaryValueCallBacks);
	bool trusted = AXIsProcessTrustedWithOptions(options);
	CFRelease(options);
	return trusted;
}
*/
import "C"

// accessibilityTrusted reports whether macOS will let this process install a
// global event tap, prompting the user once if it has not been granted yet.
func accessibilityTrusted() bool { return bool(C.mogi_accessibility_trusted()) }
