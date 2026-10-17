//go:build linux

package linux

import "os"

// IsWaylandSession reports whether the current process runs inside a
// Wayland session, selecting the Wayland adapter over the X11 one.
//
// Detection order matters: XDG_SESSION_TYPE is the authoritative signal
// (the desktop session declares it), and many Wayland sessions also
// export DISPLAY because XWayland is running — DISPLAY alone must not
// flip the choice to X11. WAYLAND_DISPLAY is the fallback for
// environments that don't set XDG_SESSION_TYPE (minimal compositor
// setups, some nested sessions).
//
//   - XDG_SESSION_TYPE=wayland        -> Wayland
//   - XDG_SESSION_TYPE=x11            -> X11
//   - unset + WAYLAND_DISPLAY set     -> Wayland
//   - unset + no WAYLAND_DISPLAY      -> X11 (legacy default)
func IsWaylandSession() bool {
	switch os.Getenv("XDG_SESSION_TYPE") {
	case "wayland":
		return true
	case "x11":
		return false
	}
	return os.Getenv("WAYLAND_DISPLAY") != ""
}
