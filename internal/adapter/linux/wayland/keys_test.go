//go:build linux

package wayland

import (
	"strings"
	"testing"

	"github.com/muthuishere/windowctl/internal/core"
)

func TestEvdevKeycodesUniqueAndInRange(t *testing.T) {
	seen := map[int]string{}
	for name, code := range evdevKeycodes {
		if code < 1 || code > 255 {
			t.Fatalf("key %q maps to out-of-range keycode %d", name, code)
		}
		if other, dup := seen[code]; dup {
			t.Fatalf("keycode %d maps to both %q and %q", code, other, name)
		}
		seen[code] = name
	}
}

// TestChordKeyEvents covers every key name the public API surface
// documents for Chord.Key: a-z, 0-9, f1..f12, the named keys, and the
// punctuation runes.
func TestChordKeyEvents(t *testing.T) {
	cases := []struct {
		name  string
		chord core.Chord
		want  string
	}{
		{"single letter", core.Chord{Key: "a"}, "30:1 30:0"},
		{"digit", core.Chord{Key: "5"}, "6:1 6:0"},
		{"f5", core.Chord{Key: "f5"}, "63:1 63:0"},
		{"enter", core.Chord{Key: "enter"}, "28:1 28:0"},
		{"tab", core.Chord{Key: "tab"}, "15:1 15:0"},
		{"esc", core.Chord{Key: "esc"}, "1:1 1:0"},
		{"space", core.Chord{Key: "space"}, "57:1 57:0"},
		{"arrow up", core.Chord{Key: "up"}, "103:1 103:0"},
		{"delete", core.Chord{Key: "delete"}, "111:1 111:0"},
		{"backspace", core.Chord{Key: "backspace"}, "14:1 14:0"},
		{"pageup", core.Chord{Key: "pageup"}, "104:1 104:0"},
		// Modifiers: down in super,ctrl,alt,shift order; up reversed.
		{"cmd+shift+s", core.Chord{Cmd: true, Shift: true, Key: "s"},
			"125:1 42:1 31:1 31:0 42:0 125:0"},
		{"ctrl+c", core.Chord{Ctrl: true, Key: "c"},
			"29:1 46:1 46:0 29:0"},
		{"alt+f4", core.Chord{Alt: true, Key: "f4"},
			"56:1 62:1 62:0 56:0"},
		// Shifted US punctuation resolves to base key + shift.
		{"shift+1 is bang", core.Chord{Key: "!"},
			"42:1 2:1 2:0 42:0"},
		{"shift+3 is hash", core.Chord{Key: "#"},
			"42:1 4:1 4:0 42:0"},
		{"shift+minus is underscore", core.Chord{Key: "_"},
			"42:1 12:1 12:0 42:0"},
		{"unshifted punctuation passthrough", core.Chord{Key: "."},
			"52:1 52:0"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			events, err := chordKeyEvents(tc.chord)
			if err != nil {
				t.Fatalf("chordKeyEvents(%+v): %v", tc.chord, err)
			}
			if got := strings.Join(events, " "); got != tc.want {
				t.Fatalf("chordKeyEvents(%+v) = %q, want %q", tc.chord, got, tc.want)
			}
		})
	}
}

func TestChordKeyEventsUnmapped(t *testing.T) {
	// A key with no US-layout keycode must fail loudly with the
	// ErrNotImplemented sentinel, not silently type nothing.
	_, err := chordKeyEvents(core.Chord{Key: "€"})
	if err == nil {
		t.Fatal("expected an error for unmapped key €")
	}
	if !errorsIsNotImplemented(err) {
		t.Fatalf("expected ErrNotImplemented, got %v", err)
	}
}

func errorsIsNotImplemented(err error) bool {
	return err != nil && strings.Contains(err.Error(), "not implemented")
}
