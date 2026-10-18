//go:build linux

package kde

import (
	"fmt"
	"image"
	"image/draw"
	"os"
	"os/exec"
	"strings"

	"github.com/muthuishere/windowctl/internal/adapter/linux/wayland/capture"
	"github.com/muthuishere/windowctl/internal/core"
)

// CaptureRect captures bounds (global virtual-desktop coordinates in
// points) into outPath as a PNG whose pixels are 1:1 with points.
//
// KWin 6 removed its Screenshot D-Bus interface (it existed on Plasma
// 5), and the xdg-desktop-portal Screenshot API cannot capture an
// arbitrary rect non-interactively — so the capture goes through
// spectacle's full-desktop background capture followed by Go-side
// cropping:
//
//  1. spectacle -b -n -f grabs the whole virtual desktop at device
//     pixels (bounds × devicePixelRatio).
//  2. The device-pixel region for bounds is cropped out.
//  3. On HiDPI (dpr ≠ 1) the crop is resampled down to exactly
//     bounds.W × bounds.H so 1 image pixel == 1 coordinate point —
//     the invariant the automation loop (screenshot → read a pixel →
//     click it) depends on.
func (b *Backend) CaptureRect(bounds core.Rect, outPath string) error {
	if _, err := exec.LookPath("spectacle"); err != nil {
		return fmt.Errorf("spectacle not found on PATH — required for screenshots on KDE Plasma. "+
			"Install it (e.g. sudo pacman -S spectacle): %w", core.ErrNotImplemented)
	}

	dpr := b.scaleForBounds(bounds)

	tmp, err := os.CreateTemp("", "windowctl-capture-*.png")
	if err != nil {
		return fmt.Errorf("kde: capture temp file: %w", err)
	}
	tmpPath := tmp.Name()
	_ = tmp.Close()
	defer os.Remove(tmpPath)

	cmd := exec.Command("spectacle", "-b", "-n", "-f", "-o", tmpPath)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("kde: spectacle capture: %v: %s", err, strings.TrimSpace(string(out)))
	}

	src, err := capture.LoadPNG(tmpPath)
	if err != nil {
		return err
	}

	// Device-pixel source region for the requested point rect.
	srcRect := image.Rect(
		int(float64(bounds.X)*dpr),
		int(float64(bounds.Y)*dpr),
		int(float64(bounds.X+bounds.W)*dpr),
		int(float64(bounds.Y+bounds.H)*dpr),
	)
	// Clamp the device-pixel region to what was actually captured; a
	// fully out-of-range rect is a hard error, partial clamping just
	// trims to the desktop edge.
	srcRect = srcRect.Intersect(src.Bounds())
	if srcRect.Empty() || srcRect.Dx() == 0 || srcRect.Dy() == 0 {
		return fmt.Errorf("kde: requested rect %v lies outside the captured desktop %v", bounds, src.Bounds())
	}

	if dpr == 1 {
		// Fast path: the crop is already point-sized.
		dst := image.NewRGBA(image.Rect(0, 0, bounds.W, bounds.H))
		draw.Draw(dst, dst.Bounds(), src, srcRect.Min, draw.Src)
		return capture.WritePNG(dst, outPath)
	}

	// HiDPI: resample the device-pixel crop down to point size.
	cropped := image.NewRGBA(image.Rect(0, 0, srcRect.Dx(), srcRect.Dy()))
	draw.Draw(cropped, cropped.Bounds(), src, srcRect.Min, draw.Src)
	return capture.WritePNG(capture.ResampleToPoints(cropped, bounds.W, bounds.H), outPath)
}

// scaleForBounds returns the devicePixelRatio of the output containing
// the majority of bounds, falling back to 1 when the compositor query
// fails — a plain crop at 1:1 is still better than no capture.
func (b *Backend) scaleForBounds(bounds core.Rect) float64 {
	screens, err := b.screens()
	if err != nil || len(screens) == 0 {
		return 1
	}
	cx := bounds.X + bounds.W/2
	cy := bounds.Y + bounds.H/2
	for _, s := range screens {
		if cx >= s.X && cx < s.X+s.Width && cy >= s.Y && cy < s.Y+s.Height {
			if s.DPR > 0 {
				return s.DPR
			}
			return 1
		}
	}
	return 1
}
