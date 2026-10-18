package capture

import (
	"fmt"
	"os"
	"os/exec"

	"github.com/muthuishere/windowctl/internal/core"
)

// GrimCapture captures bounds (global virtual-desktop coordinates in
// points) into outPath as a PNG whose pixels are 1:1 with points,
// using grim's wlr-screencopy support — shared by the wlroots backends
// (sway, Hyprland).
//
// grim captures the requested rect directly: the -g geometry is in
// logical points — the same space the window-management queries
// report — and the written PNG is device pixels (logical × output
// scale). The only Go-side work is the HiDPI downsample:
//
//   - at scale 1 the PNG is exactly bounds.W × bounds.H (verified on
//     sway 1.10.1) and is copied without re-encoding;
//   - at scale ≠ 1 the PNG is larger and is resampled down to exactly
//     bounds.W × bounds.H — the 1px == 1pt normalization invariant
//     (same direction as the kde/gnome pipelines).
//
// Verified quirk: grim does NOT clamp the rect
// to the desktop — an out-of-screen rect still yields a full-size
// image (with empty content outside the screen). Caller bounds always
// come from window rects, which lie within the desktop, so no
// pre-clamping is done here (clamping would desync from grim's own
// geometry).
//
// grim is a Wayland client: it needs WAYLAND_DISPLAY +
// XDG_RUNTIME_DIR from the inherited environment (the variables the
// compositor session provides; an SSH session must inject them
// alongside the compositor's IPC variable).
func GrimCapture(bounds core.Rect, outPath string) error {
	if _, err := exec.LookPath("grim"); err != nil {
		return fmt.Errorf("grim not found on PATH — required for screenshots on this Wayland compositor. "+
			"Install it (Debian: sudo apt install grim): %w", core.ErrNotImplemented)
	}

	tmp, err := os.CreateTemp("", "windowctl-grim-capture-*.png")
	if err != nil {
		return fmt.Errorf("grim: capture temp file: %w", err)
	}
	tmpPath := tmp.Name()
	_ = tmp.Close()
	defer os.Remove(tmpPath)

	geom := grimGeom(bounds)
	cmd := exec.Command("grim", "-g", geom, tmpPath)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("grim capture %q: %v: %s", geom, err, out)
	}

	src, err := LoadPNG(tmpPath)
	if err != nil {
		return err
	}
	if !needResample(src.Bounds().Dx(), src.Bounds().Dy(), bounds.W, bounds.H) {
		// Already exactly the requested point size — copy without
		// re-encoding.
		return CopyFile(tmpPath, outPath)
	}
	return WritePNG(ResampleToPoints(src, bounds.W, bounds.H), outPath)
}

// grimGeom formats bounds as grim's -g geometry string: "X,Y WxH"
// (logical points, the same coordinate space the window queries
// report). Extracted for unit testing.
func grimGeom(bounds core.Rect) string {
	return fmt.Sprintf("%d,%d %dx%d", bounds.X, bounds.Y, bounds.W, bounds.H)
}

// needResample reports whether the captured image is not already
// exactly the requested point size — i.e. grim wrote device pixels
// (scale ≠ 1) and the HiDPI downsample must run. Extracted for unit
// testing.
func needResample(srcW, srcH, wantW, wantH int) bool {
	return srcW != wantW || srcH != wantH
}
