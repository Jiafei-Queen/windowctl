//go:build linux

package wayland

import (
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/Jiafei-Queen/windowctl/internal/core"
)

// fakeBackend is a minimal Backend for input-path tests: no
// compositor calls (only pointerMover behavior differs, via
// fakePointerBackend below).
type fakeBackend struct{}

func (f *fakeBackend) Name() string                          { return "fake" }
func (f *fakeBackend) ListWindows() ([]core.Window, error)   { return nil, nil }
func (f *fakeBackend) ListMonitors() ([]core.Monitor, error) { return nil, nil }
func (f *fakeBackend) Move(string, core.Rect) error          { return nil }
func (f *fakeBackend) Focus(string) error                    { return nil }
func (f *fakeBackend) CursorPosition() (int, int, error)     { return 0, 0, nil }
func (f *fakeBackend) CaptureRect(core.Rect, string) error   { return nil }

// fakePointerBackend additionally implements pointerMover — the exact
// cursor channel the shared input path prefers.
type fakePointerBackend struct {
	fakeBackend
	cursorX, cursorY int
}

func (f *fakePointerBackend) MoveCursor(x, y int) error {
	f.cursorX, f.cursorY = x, y
	return nil
}

// withFakeYdotool replaces the exec seam for one test: ydotool calls
// are captured (one entry per call, argv verbatim) and succeed. PATH
// keeps a stub binary so the presence check passes.
func withFakeYdotool(t *testing.T) *[][]string {
	t.Helper()
	binDir := t.TempDir()
	bin := binDir + "/ydotool"
	if err := os.WriteFile(bin, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatalf("write stub: %v", err)
	}
	t.Setenv("PATH", binDir)

	var captured [][]string
	orig := ydotool
	ydotool = func(args ...string) error {
		captured = append(captured, args)
		return nil
	}
	t.Cleanup(func() { ydotool = orig })
	return &captured
}

// withMissingYdotool empties PATH so the presence check fails.
func withMissingYdotool(t *testing.T) {
	t.Helper()
	t.Setenv("PATH", t.TempDir())
}

func TestMouseMovePointerMoverPreferred(t *testing.T) {
	captured := withFakeYdotool(t)
	b := &fakePointerBackend{}
	a := &Adapter{compositor: "fake", backend: b}

	if err := a.MouseMove(300, 200); err != nil {
		t.Fatalf("MouseMove: %v", err)
	}
	if b.cursorX != 300 || b.cursorY != 200 {
		t.Errorf("MoveCursor = (%d,%d), want (300,200)", b.cursorX, b.cursorY)
	}
	// The compositor channel must fully replace ydotool positioning —
	// its --absolute landing is distorted by pointer acceleration.
	if len(*captured) != 0 {
		t.Errorf("ydotool must not be called when pointerMover exists, got %v", *captured)
	}
}

func TestMouseMoveYdotoolFallback(t *testing.T) {
	captured := withFakeYdotool(t)
	a := &Adapter{compositor: "fake", backend: &fakeBackend{}}

	if err := a.MouseMove(300, 200); err != nil {
		t.Fatalf("MouseMove: %v", err)
	}
	want := []string{"mousemove", "--absolute", "-x", "300", "-y", "200"}
	if strings.Join((*captured)[0], " ") != strings.Join(want, " ") {
		t.Errorf("ydotool args = %v, want %v", (*captured)[0], want)
	}
}

func TestMouseMoveMissingTool(t *testing.T) {
	withMissingYdotool(t)
	a := &Adapter{compositor: "fake", backend: &fakeBackend{}}

	err := a.MouseMove(1, 1)
	if err == nil || !errors.Is(err, core.ErrNotImplemented) {
		t.Fatalf("MouseMove without ydotool = %v, want ErrNotImplemented chain", err)
	}
	if !strings.Contains(err.Error(), "ydotoold") {
		t.Errorf("error should carry daemon setup guidance, got: %v", err)
	}
}

// TestMouseClickPinsYdotoolArgv pins the click contract: button codes
// 0xC0/0xC1/0xC2, and the repeat flags only for clicks > 1.
func TestMouseClickPinsYdotoolArgv(t *testing.T) {
	cases := []struct {
		name    string
		button  core.MouseButton
		clicks  int
		wantCmd string
	}{
		{"left single", core.MouseLeft, 1, "click 0xC0"},
		{"right single", core.MouseRight, 1, "click 0xC1"},
		{"middle single", core.MouseMiddle, 1, "click 0xC2"},
		{"left double", core.MouseLeft, 2, "click --repeat 2 --next-delay 60 0xC0"},
		{"left triple", core.MouseLeft, 3, "click --repeat 3 --next-delay 60 0xC0"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			captured := withFakeYdotool(t)
			a := &Adapter{compositor: "fake", backend: &fakeBackend{}}

			if err := a.MouseClick(10, 20, tc.button, tc.clicks); err != nil {
				t.Fatalf("MouseClick: %v", err)
			}
			var click []string
			for _, call := range *captured {
				if call[0] == "click" {
					click = call
				}
			}
			if click == nil {
				t.Fatalf("no click call captured, got %v", *captured)
			}
			if got := strings.Join(click, " "); got != tc.wantCmd {
				t.Errorf("click argv = %q, want %q", got, tc.wantCmd)
			}
		})
	}
}

// TestMouseClickPositionsViaPointerMover pins that a click on a
// pointerMover backend positions through the compositor channel and
// presses through ydotool.
func TestMouseClickPositionsViaPointerMover(t *testing.T) {
	captured := withFakeYdotool(t)
	b := &fakePointerBackend{}
	a := &Adapter{compositor: "fake", backend: b}

	if err := a.MouseClick(50, 60, core.MouseLeft, 1); err != nil {
		t.Fatalf("MouseClick: %v", err)
	}
	if b.cursorX != 50 || b.cursorY != 60 {
		t.Errorf("MoveCursor = (%d,%d), want (50,60)", b.cursorX, b.cursorY)
	}
	if len(*captured) != 1 || (*captured)[0][0] != "click" {
		t.Errorf("expected exactly one ydotool click, got %v", *captured)
	}
}

// TestTypeTextAndChordThroughSeam covers the remaining ydotool routes:
// literal type, and key events assembled by chordKeyEvents (the
// mapping itself is covered by keys_test.go).
func TestTypeTextAndChordThroughSeam(t *testing.T) {
	captured := withFakeYdotool(t)
	a := &Adapter{compositor: "fake", backend: &fakeBackend{}}

	if err := a.TypeText("hello"); err != nil {
		t.Fatalf("TypeText: %v", err)
	}
	if got := strings.Join((*captured)[0], " "); got != "type hello" {
		t.Errorf("type argv = %q, want %q", got, "type hello")
	}

	// core.Chord is constructed directly: ParseChord lives in the
	// public package, which this adapter's tests cannot import
	// (import cycle). The combo-string parsing is covered there.
	chord := core.Chord{Ctrl: true, Key: "c"}
	if err := a.PressChord(chord); err != nil {
		t.Fatalf("PressChord: %v", err)
	}
	key := (*captured)[1]
	if key[0] != "key" || len(key) < 2 {
		t.Fatalf("key argv = %v, want key + events", key)
	}
	// chordKeyEvents: modifiers down first, ctrl is evdev 29.
	if !strings.HasPrefix(strings.Join(key[1:], " "), "29:1 ") {
		t.Errorf("ctrl+c events should start with evdev 29 (ctrl) down, got %v", key)
	}
}
