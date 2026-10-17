//go:build linux

// Package linux is the Linux platform adapter entry point for windowctl.
//
// It is a runtime dispatcher, not an adapter itself: New() inspects the
// session environment and returns the X11 adapter (shelling out to
// wmctrl/xdotool, the historical implementation now living in x11/) or
// the Wayland adapter (wayland/, per-compositor backends; currently KDE
// Plasma/KWin is implemented, wlroots compositors are a follow-up).
//
// See docs/specs/17-wayland-adapter.md for the spec.
package linux

import (
	"github.com/muthuishere/windowctl/internal/adapter/linux/wayland"
	"github.com/muthuishere/windowctl/internal/adapter/linux/x11"
	"github.com/muthuishere/windowctl/internal/core"
)

// New returns the adapter for the current session: the Wayland adapter
// when running under a Wayland compositor, the X11 adapter otherwise.
// Detection is environment-only (no D-Bus, no subprocess) so it stays
// cheap enough to run at package init, which is where
// newPlatformAdapter() calls it.
func New() core.Adapter {
	if IsWaylandSession() {
		return wayland.New()
	}
	return x11.New()
}
