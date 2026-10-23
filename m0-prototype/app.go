package main

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"sync"
	"time"
)

// Mode is the whole product in miniature:
//
//   - locked: the canvas sits on the desktop layer, ignores the mouse
//     completely (clicks fall through to desktop icons), and is only
//     there to be looked at.
//   - edit: the canvas is raised to a normal window level and accepts
//     input so cards can be dragged and edited.
//
// Toggling between the two is what M0 has to prove on a real Mac.
type Mode string

const (
	ModeLocked Mode = "locked"
	ModeEdit   Mode = "edit"
)

type App struct {
	ctx context.Context

	mu sync.Mutex
	// mode is the current canvas mode.
	mode Mode
	// levelOffset int
	levelOffset int

	// update is the auto-updater's view of the world (see updater.go);
	// stagedApp is the extracted .app of a downloaded update, ready
	// for the apply helper.
	update    updateInfo
	stagedApp string
}

func NewApp() *App {
	a := &App{
		mode:        ModeLocked,
		levelOffset: -1,
		update:      updateInfo{Current: Version, Status: "idle"},
	}
	registerHotkeyApp(a)
	return a
}

func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
	installHotkey()
	cleanupPreviousBundle()

	// Check shortly after launch, without delaying the first paint. A
	// hit is surfaced in the panel; installation itself only starts
	// when the user presses the update button (or an embedding product
	// flow chooses to call /api/update/apply directly).
	go func() {
		select {
		case <-ctx.Done():
			return
		case <-time.After(4 * time.Second):
			a.checkUpdate(context.Background())
		}
	}()
}

// domReady is the first moment the native window definitely exists, so
// it is where the initial sink to the desktop layer happens.
func (a *App) domReady(ctx context.Context) {
	a.applyNative()
}

// toggleMode flips locked <-> edit. Called from the HTTP API and from
// the global hotkey (⌥⌘D) — the hotkey is the only way back out of
// locked mode, because in locked mode the canvas itself takes no clicks.
func (a *App) toggleMode() Mode {
	a.mu.Lock()
	if a.mode == ModeLocked {
		a.mode = ModeEdit
	} else {
		a.mode = ModeLocked
	}
	m := a.mode
	a.mu.Unlock()
	a.applyNative()
	return m
}

func (a *App) setMode(m Mode) {
	a.mu.Lock()
	a.mode = m
	a.mu.Unlock()
	a.applyNative()
}

func (a *App) setLevelOffset(off int) {
	a.mu.Lock()
	a.levelOffset = off
	a.mu.Unlock()
	a.applyNative()
}

func (a *App) state() (Mode, int) {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.mode, a.levelOffset
}

// applyNative pushes the current Go-side state into the native window
// layer (platform_darwin.go on macOS, a logging stub elsewhere).
func (a *App) applyNative() {
	m, off := a.state()
	applyWindowMode(m == ModeLocked, off)
}

// --- HTTP API (assetserver Handler) -------------------------------------
//
// The frontend talks to Go through plain fetch() calls instead of the
// generated Wails bindings, so this prototype builds with a bare
// `go build` / `wails build` and no binding-generation step.

type stateResponse struct {
	Mode        Mode       `json:"mode"`
	LevelOffset int        `json:"levelOffset"`
	Update      updateInfo `json:"update"`
}

func (a *App) currentState() stateResponse {
	m, off := a.state()
	return stateResponse{Mode: m, LevelOffset: off, Update: a.snapshotUpdate()}
}

func (a *App) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch r.URL.Path {
	case "/api/state":
		writeJSON(w, a.currentState())
	case "/api/toggle":
		a.toggleMode()
		writeJSON(w, a.currentState())
	case "/api/mode":
		var body struct {
			Mode Mode `json:"mode"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil ||
			(body.Mode != ModeLocked && body.Mode != ModeEdit) {
			http.Error(w, "want {\"mode\":\"locked\"|\"edit\"}", http.StatusBadRequest)
			return
		}
		a.setMode(body.Mode)
		writeJSON(w, a.currentState())
	case "/api/level":
		var body struct {
			Offset int `json:"offset"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, "want {\"offset\":int}", http.StatusBadRequest)
			return
		}
		a.setLevelOffset(body.Offset)
		writeJSON(w, a.currentState())
	case "/api/update/check":
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		a.checkUpdate(ctx)
		writeJSON(w, a.currentState())
	case "/api/update/apply":
		// Applying can download several MB and then this process exits
		// as part of the handover, so run it detached and answer now.
		// Progress and failures land in the update status, which the
		// panel keeps polling until the app restarts.
		go func() {
			if err := a.applyUpdate(context.Background()); err != nil {
				a.setUpdateStatus("error", err.Error())
			}
		}()
		writeJSON(w, a.currentState())
	default:
		http.NotFound(w, r)
	}
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(v); err != nil {
		log.Printf("deskcanvas: write state: %v", err)
	}
}
