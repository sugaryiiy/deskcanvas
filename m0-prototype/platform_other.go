//go:build !darwin

package main

import "log"

// Non-macOS stub: the desktop-layer trick is inherently native, so on
// other platforms (including the Linux build machine) the prototype
// just logs what it *would* do. This keeps `go build` / `go vet` green
// off a Mac; the real validation only counts on macOS hardware.

func registerHotkeyApp(a *App) {}

func applyWindowMode(locked bool, levelOffset int) {
	log.Printf("deskcanvas [stub]: applyWindowMode locked=%v levelOffset=%d (no-op off macOS)", locked, levelOffset)
}

func installHotkey() {
	log.Printf("deskcanvas [stub]: global hotkey not installed off macOS")
}
