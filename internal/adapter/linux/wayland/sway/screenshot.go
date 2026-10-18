//go:build linux

package sway

import (
	"github.com/muthuishere/windowctl/internal/adapter/linux/wayland/capture"
	"github.com/muthuishere/windowctl/internal/core"
)

// CaptureRect captures bounds (global virtual-desktop coordinates in
// points) into outPath as a PNG whose pixels are 1:1 with points.
//
// The capture runs through grim (wlr-screencopy), shared with the
// other wlroots backends — see capture.GrimCapture for the pipeline
// and sway-mechanism.md §9 for the verified geometry semantics.
func (b *Backend) CaptureRect(bounds core.Rect, outPath string) error {
	return capture.GrimCapture(bounds, outPath)
}
