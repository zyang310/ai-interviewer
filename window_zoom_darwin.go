//go:build darwin

package main

// Native (Cocoa) implementation of the green-button zoom. The Wails v2 runtime
// can neither animate a frame change (its darwin layer hard-codes
// setFrame:...animate:FALSE) nor reach the true screen bottom (positions are
// mapped against the visibleFrame, which stops at the Dock, and the
// Dock-inclusive work area is never exposed to Go). So this file talks to
// AppKit directly. The pre-zoom frame is kept on the ObjC side as a static
// NSRect: Wails Go coordinates are visibleFrame-relative and top-anchored, and
// converting them to/from Cocoa's global bottom-left space is exactly the
// error-prone step we want to avoid. All statics are touched only on the main
// dispatch queue, so they need no locking.

/*
#cgo CFLAGS: -x objective-c
#cgo LDFLAGS: -framework Cocoa

#import <Cocoa/Cocoa.h>

// Pre-zoom frame + animation bookkeeping, main-queue only. The window pointer
// is deliberately never stored — it is re-resolved inside every block so a
// torn-down window can't dangle.
static NSRect mogiSavedFrame;
static BOOL mogiHasSavedFrame = NO;
static CFAbsoluteTime mogiAnimEndsAt = 0; // when the in-flight frame animation settles

// mogiWindow resolves the app's single window. mainWindow is set during a
// button click; firstObject covers early startup. May be nil during teardown.
static NSWindow *mogiWindow(void) {
    NSWindow *w = [NSApp mainWindow];
    return w != nil ? w : [[NSApp windows] firstObject];
}

// mogiAnimateToFrame drives the same animated setFrame that native zoom: uses.
// AppKit derives the duration from the frame delta via animationResizeTime:,
// recorded so a re-zoom mid-animation doesn't capture a transient frame.
static void mogiAnimateToFrame(NSWindow *w, NSRect target) {
    mogiAnimEndsAt = CFAbsoluteTimeGetCurrent() + [w animationResizeTime:target];
    [w setFrame:target display:YES animate:YES];
}

// mogiZoomToScreenEdges animates the window to fill its display from the true
// screen bottom (sliding under the translucent Dock — the frameless window has
// no Titled style bit, so AppKit doesn't constrain the frame) up to the bottom
// of the menu bar, keeping the app's own traffic-light buttons visible.
static void mogiZoomToScreenEdges(void) {
    dispatch_async(dispatch_get_main_queue(), ^{
        NSWindow *w = mogiWindow();
        if (w == nil) return;
        NSScreen *screen = w.screen != nil ? w.screen : [NSScreen mainScreen];
        if (screen == nil) return;
        // Capture the restore frame only when no animation is in flight:
        // mid-animation the current frame is transient, and the frame the
        // user actually had is the one already saved.
        if (!mogiHasSavedFrame || CFAbsoluteTimeGetCurrent() >= mogiAnimEndsAt) {
            mogiSavedFrame = w.frame;
            mogiHasSavedFrame = YES;
        }
        NSRect f = screen.frame;        // full display, Dock area included
        NSRect v = screen.visibleFrame; // excludes menu bar and Dock
        NSRect target = NSMakeRect(f.origin.x, f.origin.y,
                                   f.size.width, NSMaxY(v) - f.origin.y);
        mogiAnimateToFrame(w, target);
    });
}

// mogiRestorePreZoomFrame animates the window back to the frame saved by the
// last zoom. No-op if nothing was saved or the window is gone.
static void mogiRestorePreZoomFrame(void) {
    dispatch_async(dispatch_get_main_queue(), ^{
        NSWindow *w = mogiWindow();
        if (w == nil || !mogiHasSavedFrame) return;
        mogiAnimateToFrame(w, mogiSavedFrame);
    });
}

// mogiResetZoomState forgets the saved pre-zoom frame (used when overlay mode
// clobbers the window geometry, making the memory meaningless).
static void mogiResetZoomState(void) {
    dispatch_async(dispatch_get_main_queue(), ^{
        mogiHasSavedFrame = NO;
        mogiAnimEndsAt = 0;
    });
}
*/
import "C"

// nativeZoomWindow animates the window to the screen edges (under the Dock,
// below the menu bar) via AppKit. Returns true meaning the native path handled
// the zoom, so ToggleMaximiseWindow skips the Wails-runtime fallback.
// Fire-and-forget: the work runs async on the Cocoa main thread.
func nativeZoomWindow() bool {
	C.mogiZoomToScreenEdges()
	return true
}

// nativeRestoreWindow animates the window back to its pre-zoom frame (saved on
// the ObjC side). Returns true meaning the native path handled the restore.
func nativeRestoreWindow() bool {
	C.mogiRestorePreZoomFrame()
	return true
}

// nativeResetZoom drops the saved pre-zoom frame. Called when overlay mode
// replaces the window geometry, so a later green-button click zooms fresh
// instead of "restoring" a frame that no longer means anything.
func nativeResetZoom() {
	C.mogiResetZoomState()
}
