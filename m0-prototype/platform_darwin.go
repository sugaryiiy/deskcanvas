//go:build darwin

package main

/*
#cgo LDFLAGS: -framework Cocoa -framework Carbon

// Sink/raise the Wails window. locked=1 puts it on the desktop layer at
// (desktop-icon window level + levelOffset) with mouse events ignored;
// locked=0 restores a normal, clickable window level.
void dc_apply_mode(int locked, int levelOffset);

// Register the global ⌥⌘D hotkey (Carbon). The callback lands in
// dcHotkeyPressed below.
void dc_install_hotkey(void);
*/
import "C"

// hotkeyApp is how the C hotkey callback reaches the Go app. Set in
// NewApp via registerHotkeyApp — kept as a package var because the
// Carbon callback carries no user data pointer we control end-to-end.
var hotkeyApp *App

func registerHotkeyApp(a *App) { hotkeyApp = a }

func applyWindowMode(locked bool, levelOffset int) {
	l := C.int(0)
	if locked {
		l = 1
	}
	C.dc_apply_mode(l, C.int(levelOffset))
}

func installHotkey() {
	C.dc_install_hotkey()
}

//export dcHotkeyPressed
func dcHotkeyPressed() {
	if hotkeyApp != nil {
		hotkeyApp.toggleMode()
	}
}
