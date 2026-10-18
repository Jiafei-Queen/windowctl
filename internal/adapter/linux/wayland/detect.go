//go:build linux

package wayland

import (
	"os"
	"strings"
)

// Compositor identifiers returned by detectCompositor.
const (
	compositorKDE      = "kde"
	compositorSway     = "sway"
	compositorHyprland = "hyprland"
	compositorGNOME    = "gnome"
	// compositorUnknown is the zero value for "detected nothing
	// recognizable" — New() routes it to the unsupported-adapter path.
	compositorUnknown = ""
)

// detectCompositor identifies the running Wayland compositor from the
// session environment. Returns "" when nothing is recognized.
//
// Detection is environment-only (no D-Bus probes, no subprocesses) so
// it stays cheap enough to run inside New(), which the public package
// calls at init time. Runtime failures — e.g. XDG_CURRENT_DESKTOP says
// KDE but KWin is not actually reachable — surface as errors from the
// first backend call instead, with the same clear-message contract.
func detectCompositor() string {
	// Sway and Hyprland export unique env vars pointing at their IPC
	// sockets; they take priority over desktop-environment hints
	// because a KDE-flavored variable can linger in nested sessions.
	if os.Getenv("SWAYSOCK") != "" {
		return compositorSway
	}
	if os.Getenv("HYPRLAND_INSTANCE_SIGNATURE") != "" {
		return compositorHyprland
	}
	// XDG_CURRENT_DESKTOP is a colon-separated list ("ubuntu:GNOME"),
	// so match by containment rather than equality. The wlroots
	// compositors export their own desktop hint (verified: a sway
	// session reports XDG_CURRENT_DESKTOP=sway) — a fallback for
	// environments where the IPC socket vars are absent.
	de := strings.ToUpper(os.Getenv("XDG_CURRENT_DESKTOP"))
	switch {
	case strings.Contains(de, "KDE"):
		return compositorKDE
	case strings.Contains(de, "GNOME"):
		return compositorGNOME
	case strings.Contains(de, "SWAY"):
		return compositorSway
	case strings.Contains(de, "HYPRLAND"):
		return compositorHyprland
	}
	return compositorUnknown
}
