//go:build linux

package capture

import (
	"errors"
	"image"
	"image/color"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/muthuishere/windowctl/internal/core"
)

// solidRGBA builds a w×h image where every pixel is the given color.
func solidRGBA(w, h int, c color.RGBA) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.SetRGBA(x, y, c)
		}
	}
	return img
}

// TestResampleToPoints pins the 1px==1pt normalization invariant: the
// output is exactly w×h regardless of the source's device-pixel size.
func TestResampleToPoints(t *testing.T) {
	src := solidRGBA(200, 100, color.RGBA{R: 255, A: 255})
	dst := ResampleToPoints(src, 100, 50)
	if got := dst.Bounds().Dx(); got != 100 {
		t.Errorf("Dx = %d, want 100", got)
	}
	if got := dst.Bounds().Dy(); got != 50 {
		t.Errorf("Dy = %d, want 50", got)
	}

	// Upscaling (a source smaller than requested) and fractional
	// ratios must produce the exact requested box too.
	if got := ResampleToPoints(solidRGBA(50, 25, color.RGBA{A: 255}), 100, 50); got.Bounds().Dx() != 100 || got.Bounds().Dy() != 50 {
		t.Errorf("upscale box = %dx%d, want 100x50", got.Bounds().Dx(), got.Bounds().Dy())
	}
	if got := ResampleToPoints(solidRGBA(125, 75, color.RGBA{A: 255}), 100, 50); got.Bounds().Dx() != 100 || got.Bounds().Dy() != 50 {
		t.Errorf("fractional box = %dx%d, want 100x50", got.Bounds().Dx(), got.Bounds().Dy())
	}
}

func TestLoadWriteCopyRoundtrip(t *testing.T) {
	dir := t.TempDir()
	pngPath := filepath.Join(dir, "shot.png")
	outCopy := filepath.Join(dir, "copy.png")

	src := solidRGBA(4, 3, color.RGBA{R: 10, G: 20, B: 30, A: 255})
	if err := WritePNG(src, pngPath); err != nil {
		t.Fatalf("WritePNG: %v", err)
	}

	back, err := LoadPNG(pngPath)
	if err != nil {
		t.Fatalf("LoadPNG: %v", err)
	}
	if got := back.At(2, 1); got != (color.RGBA{R: 10, G: 20, B: 30, A: 255}) {
		t.Errorf("roundtrip pixel = %v, want {10 20 30 255}", got)
	}

	if err := CopyFile(pngPath, outCopy); err != nil {
		t.Fatalf("CopyFile: %v", err)
	}
	again, err := LoadPNG(outCopy)
	if err != nil {
		t.Fatalf("LoadPNG(copy): %v", err)
	}
	if got := again.At(2, 1); got != (color.RGBA{R: 10, G: 20, B: 30, A: 255}) {
		t.Errorf("copy pixel = %v, want {10 20 30 255}", got)
	}
}

func TestLoadPNGErrors(t *testing.T) {
	dir := t.TempDir()

	if _, err := LoadPNG(filepath.Join(dir, "missing.png")); err == nil {
		t.Error("expected error for missing file")
	}

	bad := filepath.Join(dir, "bad.png")
	if err := os.WriteFile(bad, []byte("not a png"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	if _, err := LoadPNG(bad); err == nil {
		t.Error("expected error for invalid PNG content")
	}
}

// TestGrimGeom pins the -g geometry spelling grim parses: "X,Y WxH".
func TestGrimGeom(t *testing.T) {
	if got, want := grimGeom(core.Rect{X: 10, Y: 20, W: 500, H: 600}), "10,20 500x600"; got != want {
		t.Errorf("grimGeom = %q, want %q", got, want)
	}
	if got, want := grimGeom(core.Rect{X: 0, Y: 0, W: 1, H: 1}), "0,0 1x1"; got != want {
		t.Errorf("grimGeom = %q, want %q", got, want)
	}
}

func TestNeedResample(t *testing.T) {
	cases := []struct {
		srcW, srcH, wantW, wantH int
		want                     bool
	}{
		// scale 1: PNG is already point-sized — copy, don't re-encode.
		{500, 600, 500, 600, false},
		// scale 2: device pixels — resample.
		{1000, 1200, 500, 600, true},
		// One axis off is enough.
		{500, 601, 500, 600, true},
	}
	for _, tc := range cases {
		if got := needResample(tc.srcW, tc.srcH, tc.wantW, tc.wantH); got != tc.want {
			t.Errorf("needResample(%d,%d,%d,%d) = %v, want %v",
				tc.srcW, tc.srcH, tc.wantW, tc.wantH, got, tc.want)
		}
	}
}

// TestGrimCaptureMissingTools verifies the actionable-error contract
// when grim is absent from PATH (empty stub dir).
func TestGrimCaptureMissingTools(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	err := GrimCapture(core.Rect{X: 0, Y: 0, W: 10, H: 10}, filepath.Join(t.TempDir(), "out.png"))
	if err == nil {
		t.Fatal("expected error when grim is missing")
	}
	if !errors.Is(err, core.ErrNotImplemented) {
		t.Errorf("err = %v, want ErrNotImplemented chain", err)
	}
	if !strings.Contains(err.Error(), "apt install grim") {
		t.Errorf("error should carry install guidance, got: %v", err)
	}
}
