//go:build darwin

package main

// Native (Cocoa) control over which macOS Spaces the window joins. Wails v2's
// runtime exposes always-on-top (a window level) but NOT NSWindow
// collectionBehavior — and a level alone cannot float a window over another
// app's full-screen Space. A full-screen app gets its own dedicated Space, and
// a window shows up there only if its collectionBehavior opts in; otherwise it
// stays behind on the desktop Space and is simply invisible while the browser
// is full-screen. So the overlay talks to AppKit directly, the same way
// window_zoom_darwin.go does for the animated zoom.

/*
#cgo CFLAGS: -x objective-c
#cgo LDFLAGS: -framework Cocoa

#import <Cocoa/Cocoa.h>
#include <stdbool.h>

// mogiSpaceWindow resolves the app's single window. Duplicated from
// window_zoom_darwin.go rather than shared: each cgo file is its own C
// translation unit, so a file-static helper cannot cross between them. May be
// nil during teardown.
static NSWindow *mogiSpaceWindow(void) {
    NSWindow *w = [NSApp mainWindow];
    return w != nil ? w : [[NSApp windows] firstObject];
}

// mogiSetFloatOverFullscreen makes the window join every Space — including
// another app's full-screen Space — and float above its content (on=true), or
// return to normal single-Space behavior (on=false).
//
// Two things are required, and BOTH matter:
//   1. collectionBehavior — canJoinAllSpaces brings the window into whatever
//      Space is active; fullScreenAuxiliary is the flag that specifically lets a
//      non-full-screen window ride along on a full-screen Space. Without these
//      the window stays on the desktop Space and is invisible while another app
//      is full-screen.
//   2. A high window level. This is the subtle one: always-on-top's floating
//      level (3) lets the window JOIN the full-screen Space but renders it
//      BEHIND the full-screen app's content, so it looks like it "doesn't stay."
//      The full-screen app's content sits above the menu-bar level, so the
//      overlay must too. NSScreenSaverWindowLevel (1000) is the standard tier
//      overlay tools use to sit above full-screen content (short of the
//      discouraged maximum/shielding level, which would also cover system
//      alerts). Stationary keeps the bar from sliding during Space transitions.
static void mogiSetFloatOverFullscreen(bool on) {
    dispatch_async(dispatch_get_main_queue(), ^{
        NSWindow *w = mogiSpaceWindow();
        if (w == nil) return;
        if (on) {
            w.collectionBehavior = NSWindowCollectionBehaviorCanJoinAllSpaces
                                 | NSWindowCollectionBehaviorFullScreenAuxiliary
                                 | NSWindowCollectionBehaviorStationary;
            [w setLevel:NSScreenSaverWindowLevel];
        } else {
            w.collectionBehavior = NSWindowCollectionBehaviorDefault;
            [w setLevel:NSNormalWindowLevel];
        }
    });
}
*/
import "C"

// nativeFloatOverFullscreen toggles whether the window floats over other apps'
// full-screen Spaces (macOS only). Called on overlay enter/exit. Fire-and-forget:
// the work runs async on the Cocoa main thread.
func nativeFloatOverFullscreen(on bool) {
	C.mogiSetFloatOverFullscreen(C.bool(on))
}
