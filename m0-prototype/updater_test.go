//go:build !darwin

package main

import "testing"

func TestCompareVersions(t *testing.T) {
	cases := []struct {
		a, b string
		want int
	}{
		{"v0.1.0-m0", "v0.1.1-m0", -1},
		{"v0.1.1-m0", "v0.1.0-m0", 1},
		{"v0.1.1-m0", "v0.1.1-m0", 0},
		{"v0.1.1-m0", "v0.1.1", -1}, // prerelease sorts before release
		{"v0.1.1", "v0.1.1-m0", 1},
		{"0.2", "0.10", -1}, // numeric, not lexical
		{"v1", "v1.0.0", 0},
	}
	for _, c := range cases {
		if got := compareVersions(c.a, c.b); got != c.want {
			t.Errorf("compareVersions(%q, %q) = %d, want %d", c.a, c.b, got, c.want)
		}
	}
}

func TestStateIncludesUpdate(t *testing.T) {
	a := NewApp()
	s := a.currentState()
	if s.Update.Current != Version {
		t.Fatalf("state update.current = %q, want %q", s.Update.Current, Version)
	}
	if s.Update.Status != "idle" {
		t.Fatalf("state update.status = %q, want idle", s.Update.Status)
	}
}
