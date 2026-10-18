//go:build linux

package gnome

import (
	"fmt"
	"os"

	"github.com/Jiafei-Queen/windowctl/internal/adapter/linux/wayland/capture"
	"github.com/Jiafei-Queen/windowctl/internal/core"
)

// CaptureRect captures bounds (global coordinates in points) into
// outPath as a PNG whose pixels are 1:1 with points.
//
// The capture itself happens in the extension via the in-process
// Shell.Screenshot API (the D-Bus screenshot interface is
// whitelist-gated and spectacle has no GNOME equivalent): the
// extension scales the point rect to device pixels with the stage's
// scale factor and writes the PNG to a temp file this process passes
// in, reporting back the factor it used.
//
// Go side: the PNG is logical×scale, so on HiDPI (scale ≠ 1) it is
// resampled down to exactly bounds.W × bounds.H — the 1px==1pt
// normalization invariant, same direction as the kde pipeline. At
// scale 1 the PNG is already point-sized (device-rect rounding can be
// off by ≤1px on fractional scales, which the resample path absorbs).
func (b *Backend) CaptureRect(bounds core.Rect, outPath string) error {
	tmp, err := os.CreateTemp("", "windowctl-gnome-capture-*.png")
	if err != nil {
		return fmt.Errorf("gnome: capture temp file: %w", err)
	}
	tmpPath := tmp.Name()
	_ = tmp.Close()
	defer os.Remove(tmpPath)

	payload, err := b.call("Screenshot",
		int32(bounds.X), int32(bounds.Y), int32(bounds.W), int32(bounds.H), tmpPath)
	if err != nil {
		return err
	}
	scale, err := parseScale(payload)
	if err != nil {
		return err
	}

	src, err := capture.LoadPNG(tmpPath)
	if err != nil {
		return fmt.Errorf("gnome: %w", err)
	}

	if scale == 1 &&
		src.Bounds().Dx() == bounds.W && src.Bounds().Dy() == bounds.H {
		// Already exactly the requested point size — copy without
		// re-encoding.
		return capture.CopyFile(tmpPath, outPath)
	}

	return capture.WritePNG(capture.ResampleToPoints(src, bounds.W, bounds.H), outPath)
}
