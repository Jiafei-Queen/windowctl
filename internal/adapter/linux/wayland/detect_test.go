//go:build linux

package wayland

import (
	"errors"
	"strings"
	"testing"

	"github.com/muthuishere/windowctl/internal/core"
)

func TestDetectCompositor(t *testing.T) {
	cases := []struct {
		name    string
		sway    string
		hypr    string
		desktop string
		want    string
	}{
		{"swaysock wins", "/run/user/1000/sway-ipc.1000.sock", "", "KDE", compositorSway},
		{"hyprland signature wins", "", "abc123", "KDE", compositorHyprland},
		{"sway desktop hint", "", "", "sway", compositorSway},
		{"hyprland desktop hint", "", "", "Hyprland", compositorHyprland},
		{"kde desktop", "", "", "KDE", compositorKDE},
		{"kde plasma variant", "", "", "KDE:PLASMA", compositorKDE},
		{"gnome desktop", "", "", "GNOME", compositorGNOME},
		{"ubuntu gnome colon list", "", "", "ubuntu:GNOME", compositorGNOME},
		{"nothing recognizable", "", "", "", compositorUnknown},
		{"cosmic unknown", "", "", "COSMIC", compositorUnknown},
		// Nested-session matrix: wlroots IPC vars beat any lingering
		// desktop hint, and they are checked in a fixed order.
		{"sway + hyprland sockets", "/run/user/1000/sway-ipc.sock", "abc123", "GNOME", compositorSway},
		{"lowercase gnome hint", "", "", "gnome", compositorGNOME},
		{"mixed colon list with gnome", "", "", "pop:GNOME:ubuntu", compositorGNOME},
		// Desktop-hint matching is ordered KDE → GNOME → sway →
		// Hyprland; the first hit wins a mixed colon list.
		{"mixed colon list sway+kde", "", "", "sway:KDE", compositorKDE},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("SWAYSOCK", tc.sway)
			t.Setenv("HYPRLAND_INSTANCE_SIGNATURE", tc.hypr)
			t.Setenv("XDG_CURRENT_DESKTOP", tc.desktop)
			if got := detectCompositor(); got != tc.want {
				t.Fatalf("detectCompositor() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestNewUnsupportedCompositorFailsClearly(t *testing.T) {
	// An unrecognized compositor must yield an adapter whose
	// backend-dependent calls fail with ErrNotImplemented rather than
	// panicking or silently returning empty results.
	t.Setenv("SWAYSOCK", "")
	t.Setenv("HYPRLAND_INSTANCE_SIGNATURE", "")
	t.Setenv("XDG_CURRENT_DESKTOP", "COSMIC")
	a := New()
	if a.backend != nil {
		t.Fatal("expected nil backend for an unimplemented compositor")
	}
	_, err := a.ListWindows()
	if err == nil {
		t.Fatal("ListWindows on unsupported compositor must fail")
	}
	if !errors.Is(err, core.ErrNotImplemented) {
		t.Errorf("err should chain ErrNotImplemented, got %v", err)
	}
	// The message must name the detected compositor and the
	// implemented set, so users know both the problem and the fix.
	for _, want := range []string{"COSMIC", "KDE", "GNOME", "sway", "Hyprland"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error message missing %q: %v", want, err)
		}
	}
}
