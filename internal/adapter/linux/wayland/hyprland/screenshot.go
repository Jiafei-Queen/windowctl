//go:build linux

package hyprland

import (
	"github.com/Jiafei-Queen/windowctl/internal/adapter/linux/wayland/capture"
	"github.com/Jiafei-Queen/windowctl/internal/core"
)

// CaptureRect captures bounds (global virtual-desktop coordinates in
// points) into outPath as a PNG whose pixels are 1:1 with points.
//
// The capture runs through grim (wlr-screencopy), shared with the
// other wlroots backends — see capture.GrimCapture for the pipeline.
// Verified on Hyprland 0.55.2 with a scale-2 monitor: the region PNG
// is logical × scale device pixels (100×100 requested → 200×200
// written), and the shared resample path brings it back to point
// size — the invariant held on a real HiDPI setup.
func (b *Backend) CaptureRect(bounds core.Rect, outPath string) error {
	return capture.GrimCapture(bounds, outPath)
}
