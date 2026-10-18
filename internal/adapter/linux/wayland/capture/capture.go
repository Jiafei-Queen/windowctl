// Package capture holds the PNG plumbing shared by the Wayland
// backends' CaptureRect implementations.
//
// Core contract: CaptureRect must emit a PNG whose pixels are 1:1 with
// coordinate points (1 image pixel == 1 coordinate point), so an agent
// can feed pixel coordinates read off the screenshot straight into
// MouseClick. Compositors hand out device pixels (logical × scale
// factor), so every backend crops/resamples through the helpers here —
// docs/wayland/design.md §5.
package capture

import (
	"fmt"
	"image"
	"image/draw"
	"image/png"
	"os"

	xdraw "golang.org/x/image/draw"
)

// LoadPNG decodes the PNG file at path.
func LoadPNG(path string) (image.Image, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open capture: %w", err)
	}
	defer f.Close()
	img, err := png.Decode(f)
	if err != nil {
		return nil, fmt.Errorf("decode capture PNG: %w", err)
	}
	return img, nil
}

// WritePNG encodes img as a PNG file at outPath.
func WritePNG(img image.Image, outPath string) error {
	f, err := os.Create(outPath)
	if err != nil {
		return fmt.Errorf("create %s: %w", outPath, err)
	}
	defer f.Close()
	if err := png.Encode(f, img); err != nil {
		return fmt.Errorf("encode PNG to %s: %w", outPath, err)
	}
	return nil
}

// ResampleToPoints downsamples a device-pixel image to exactly
// w × h points (logical pixels) with bilinear filtering. src is
// expected to already be the requested region — callers crop first.
func ResampleToPoints(src image.Image, w, h int) *image.RGBA {
	dst := image.NewRGBA(image.Rect(0, 0, w, h))
	xdraw.BiLinear.Scale(dst, dst.Bounds(), src, src.Bounds(), draw.Src, nil)
	return dst
}

// CopyFile copies the file at srcPath to outPath (content only, no
// mode preservation) — used when a captured PNG is already exactly
// point-sized and must not be re-encoded.
func CopyFile(srcPath, outPath string) error {
	src, err := os.Open(srcPath)
	if err != nil {
		return fmt.Errorf("open %s: %w", srcPath, err)
	}
	defer src.Close()
	dst, err := os.Create(outPath)
	if err != nil {
		return fmt.Errorf("create %s: %w", outPath, err)
	}
	defer dst.Close()
	if _, err := dst.ReadFrom(src); err != nil {
		return fmt.Errorf("copy %s to %s: %w", srcPath, outPath, err)
	}
	return nil
}
