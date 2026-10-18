//go:build linux

package wayland

import (
	"fmt"
	"os/exec"
	"strconv"
	"strings"

	"github.com/muthuishere/windowctl/internal/core"
)

// ydotoolMissing returns a descriptive error when ydotool is not on
// PATH. ydotool is the only compositor-independent input channel on
// Wayland (uinput virtual device); without it the mouse/keyboard
// methods are unavailable, and the error must say exactly how to fix
// that rather than surfacing a bare exec failure.
func ydotoolMissing() error {
	if _, err := exec.LookPath("ydotool"); err != nil {
		return fmt.Errorf("ydotool not found on PATH — required for mouse/keyboard input on Wayland. "+
			"Install it plus the ydotoold daemon (the daemon needs /dev/uinput access: "+
			"add a udev rule KERNEL==\"uinput\", GROUP=\"input\", MODE=\"0660\", join the input group, "+
			"and enable the ydotoold service): %w", core.ErrNotImplemented)
	}
	return nil
}

// ydotool runs the ydotool CLI, wrapping combined output into the
// error so daemon-not-running / permission failures are actionable.
// It is a package var so tests can capture argv without touching the
// real CLI (the presence check stays on the real PATH).
var ydotool = func(args ...string) error {
	cmd := exec.Command("ydotool", args...)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("ydotool %s: %v: %s",
			strings.Join(args, " "), err, strings.TrimSpace(string(out)))
	}
	return nil
}

// MouseMove positions the OS cursor. Backends with an exact cursor
// channel (pointerMover — sway's seat cursor set, Hyprland's movecursor
// dispatcher) take precedence: ydotool's --absolute emulation is
// distorted by libinput pointer acceleration on default configs.
// Backends without one use ydotool --absolute directly (dpr=1
// setups — see pointerMover docs).
func (a *Adapter) MouseMove(x, y int) error {
	if pm, ok := a.backend.(pointerMover); ok {
		return pm.MoveCursor(x, y)
	}
	if err := ydotoolMissing(); err != nil {
		return err
	}
	return ydotool("mousemove", "--absolute", "-x", strconv.Itoa(x), "-y", strconv.Itoa(y))
}

func (a *Adapter) MouseClick(x, y int, button core.MouseButton, clicks int) error {
	if err := ydotoolMissing(); err != nil {
		return err
	}
	if err := a.MouseMove(x, y); err != nil {
		return err
	}
	// ydotool button codes: 0xC0 left, 0xC1 right, 0xC2 middle.
	btn := "0xC0"
	switch button {
	case core.MouseRight:
		btn = "0xC1"
	case core.MouseMiddle:
		btn = "0xC2"
	}
	args := []string{"click"}
	if clicks > 1 {
		args = append(args, "--repeat", strconv.Itoa(clicks), "--next-delay", "60")
	}
	args = append(args, btn)
	return ydotool(args...)
}

func (a *Adapter) TypeText(text string) error {
	if err := ydotoolMissing(); err != nil {
		return err
	}
	// ydotool type takes the literal string; it is keycode/layout-based
	// (unlike xdotool type on X11) — non-ASCII text depends on the
	// daemon's configured keyboard layout.
	return ydotool("type", text)
}

func (a *Adapter) PressChord(chord core.Chord) error {
	if err := ydotoolMissing(); err != nil {
		return err
	}
	events, err := chordKeyEvents(chord)
	if err != nil {
		return err
	}
	return ydotool(append([]string{"key"}, events...)...)
}
