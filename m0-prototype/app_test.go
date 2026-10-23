//go:build !darwin

package main

import (
	"net/http/httptest"
	"strings"
	"testing"
)

// The native layer is stubbed off macOS, so these tests cover the parts
// that are testable anywhere: the mode state machine and the HTTP API
// the frontend polls. The desktop-layer behaviour itself can only be
// verified on a real Mac (see README checklist).

func TestToggleRoundTrip(t *testing.T) {
	a := NewApp()
	if m, _ := a.state(); m != ModeLocked {
		t.Fatalf("initial mode = %v, want locked", m)
	}
	if m := a.toggleMode(); m != ModeEdit {
		t.Fatalf("after toggle mode = %v, want edit", m)
	}
	if m := a.toggleMode(); m != ModeLocked {
		t.Fatalf("after second toggle mode = %v, want locked", m)
	}
}

func TestStateEndpoint(t *testing.T) {
	a := NewApp()
	rec := httptest.NewRecorder()
	a.ServeHTTP(rec, httptest.NewRequest("GET", "/api/state", nil))
	if rec.Code != 200 {
		t.Fatalf("GET /api/state code = %d", rec.Code)
	}
	if body := rec.Body.String(); !strings.Contains(body, `"mode":"locked"`) ||
		!strings.Contains(body, `"levelOffset":-1`) {
		t.Fatalf("GET /api/state body = %s", body)
	}
}

func TestModeEndpointRejectsBadMode(t *testing.T) {
	a := NewApp()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/api/mode", strings.NewReader(`{"mode":"sideways"}`))
	a.ServeHTTP(rec, req)
	if rec.Code != 400 {
		t.Fatalf("POST /api/mode bad mode code = %d, want 400", rec.Code)
	}
}
