//go:build windows

package main

import (
	"bytes"
	"testing"
)

func TestBuildAndLoadCustomTrayIcons(t *testing.T) {
	states := []string{"running", "active", "waiting", "paused", "stopped"}
	seenDIBs := map[string][]byte{}
	for _, state := range states {
		dib := buildTrayIconDIB(state)
		if len(dib) != 40+32*32*4+32*4 {
			t.Fatalf("unexpected DIB byte length for %s: got %d", state, len(dib))
		}
		for otherState, otherDIB := range seenDIBs {
			if bytes.Equal(dib, otherDIB) {
				t.Fatalf("expected distinct tray icon pixels for %s vs %s", state, otherState)
			}
		}
		seenDIBs[state] = dib

		hicon := loadCustomTrayIcon(state)
		if hicon == 0 {
			t.Fatalf("expected non-zero HICON from CreateIconFromResourceEx for %s", state)
		}
		cached := loadCustomTrayIcon(state)
		if cached != hicon {
			t.Fatalf("expected cached HICON handle for %s", state)
		}
	}
}
