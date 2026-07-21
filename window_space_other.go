//go:build !darwin

package main

// nativeFloatOverFullscreen is a no-op off macOS. Windows' always-on-top already
// floats over full-screen apps, and X11 has no equivalent per-app full-screen
// Spaces model to opt into. Keep this signature in sync with
// window_space_darwin.go by hand — nothing enforces it.
func nativeFloatOverFullscreen(bool) {}
