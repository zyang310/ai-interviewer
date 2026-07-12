//go:build !darwin

package main

// Non-darwin stubs for the native zoom path. Returning false makes
// ToggleMaximiseWindow (window.go) fall back to the Wails-runtime zoom, which
// is instant but correct on Windows/Linux. Keep these signatures in sync with
// window_zoom_darwin.go by hand — nothing enforces it.

// nativeZoomWindow reports that no native animated zoom exists on this platform.
func nativeZoomWindow() bool { return false }

// nativeRestoreWindow reports that no native restore exists on this platform.
func nativeRestoreWindow() bool { return false }

// nativeResetZoom is a no-op; the fallback keeps its state in zoomState.
func nativeResetZoom() {}
