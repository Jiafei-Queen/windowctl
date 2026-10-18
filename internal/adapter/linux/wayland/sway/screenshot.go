//go:build linux

package sway

import (
	"fmt"
	"os"
	"os/exec"

	"github.com/muthuishere/windowctl/internal/adapter/linux/wayland/capture"
	"github.com/muthuishere/windowctl/internal/core"
)

// CaptureRect captures bounds (global virtual-desktop coordinates in
// points) into outPath as a PNG whose pixels are 1:1 with points.
//
// grim captures the requested rect directly (wlr-screencopy): the -g
// geometry is in logical points — the same space as get_tree rects —
// and the written PNG is device pixels (logical × output scale).
// Unlike the KDE pipeline there is no full-grab + crop step; the only
// Go-side work is the HiDPI downsample:
//
//   - at scale 1 the PNG is exactly bounds.W × bounds.H (verified on
//     sway 1.10.1) and is copied without re-encoding;
//   - at scale ≠ 1 the PNG is larger and is resampled down to exactly
//     bounds.W × bounds.H — the 1px == 1pt normalization invariant
//     (same direction as the kde/gnome pipelines).
//
// Verified quirk: grim does NOT clamp the rect to the desktop — an
// out-of-screen rect still yields a full-size image (with empty
// content outside the screen). windowctl bounds always come from
// window rects, which lie within the desktop, so no pre-clamping is
// done here (clamping would desync from grim's own geometry).
func (b *Backend) CaptureRect(bounds core.Rect, outPath string) error {
	if _, err := exec.LookPath("grim"); err != nil {
		return fmt.Errorf("grim not found on PATH — required for screenshots on sway. "+
			"Install it (Debian: sudo apt install grim): %w", core.ErrNotImplemented)
	}

	tmp, err := os.CreateTemp("", "windowctl-sway-capture-*.png")
	if err != nil {
		return fmt.Errorf("sway: capture temp file: %w", err)
	}
	tmpPath := tmp.Name()
	_ = tmp.Close()
	defer os.Remove(tmpPath)

	// grim is a Wayland client: it needs WAYLAND_DISPLAY +
	// XDG_RUNTIME_DIR from the inherited environment (the same
	// variables the compositor session provides; an SSH session
	// must inject them alongside SWAYSOCK — sway-mechanism.md §2).
	geom := fmt.Sprintf("%d,%d %dx%d", bounds.X, bounds.Y, bounds.W, bounds.H)
	cmd := exec.Command("grim", "-g", geom, tmpPath)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("sway: grim capture %q: %v: %s", geom, err, out)
	}

	src, err := capture.LoadPNG(tmpPath)
	if err != nil {
		return fmt.Errorf("sway: %w", err)
	}
	if src.Bounds().Dx() == bounds.W && src.Bounds().Dy() == bounds.H {
		// Already exactly the requested point size — copy without
		// re-encoding.
		return capture.CopyFile(tmpPath, outPath)
	}
	return capture.WritePNG(capture.ResampleToPoints(src, bounds.W, bounds.H), outPath)
}
