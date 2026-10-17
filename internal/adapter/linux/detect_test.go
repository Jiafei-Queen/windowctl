//go:build linux

package linux

import (
	"fmt"
	"strings"
	"testing"
)

func TestIsWaylandSession(t *testing.T) {
	cases := []struct {
		name           string
		sessionType    string
		waylandDisplay string
		display        string
		want           bool
	}{
		{"session type wayland wins over XWayland DISPLAY", "wayland", "wayland-0", ":1", true},
		{"session type wayland alone", "wayland", "", "", true},
		{"session type x11 with DISPLAY", "x11", "", ":0", false},
		{"session type x11 with WAYLAND_DISPLAY also set", "x11", "wayland-0", ":1", false},
		{"no session type, WAYLAND_DISPLAY set", "", "wayland-0", "", true},
		{"no session type, only DISPLAY (X11)", "", "", ":0", false},
		{"nothing set at all (legacy X11 default)", "", "", "", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("XDG_SESSION_TYPE", tc.sessionType)
			t.Setenv("WAYLAND_DISPLAY", tc.waylandDisplay)
			t.Setenv("DISPLAY", tc.display)
			if got := IsWaylandSession(); got != tc.want {
				t.Fatalf("IsWaylandSession() = %v, want %v", got, tc.want)
			}
		})
	}
}

// TestNewDispatchesBySession pins the dispatcher's two branches: a
// Wayland session must yield the wayland package's adapter, everything
// else the x11 one. A future refactor that silently routes Wayland
// sessions to the X11 adapter fails here rather than in the field.
func TestNewDispatchesBySession(t *testing.T) {
	cases := []struct {
		name        string
		sessionType string
		display     string
		wantPkg     string
	}{
		{"wayland session -> wayland adapter", "wayland", ":1", "wayland"},
		{"x11 session -> x11 adapter", "x11", ":0", "x11"},
		{"no signal -> x11 adapter", "", ":0", "x11"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("XDG_SESSION_TYPE", tc.sessionType)
			t.Setenv("WAYLAND_DISPLAY", "")
			t.Setenv("DISPLAY", tc.display)
			got := fmt.Sprintf("%T", New())
			if !strings.HasSuffix(got, tc.wantPkg+".Adapter") {
				t.Fatalf("New() returned %s, want the %s adapter", got, tc.wantPkg)
			}
		})
	}
}
