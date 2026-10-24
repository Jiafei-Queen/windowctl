//go:build linux

package x11

import (
	"strings"
	"testing"

	"github.com/Jiafei-Queen/windowctl/internal/core"
)

// parseWindowLineFixture mirrors a real `wmctrl -lpGx` record: id,
// desktop (-1 = sticky), pid, geometry, WM_CLASS ("instance.class"),
// client machine, title (spaces collapse).
const windowLine = "0x03a00007  1 13487 640 24 640 480 gnome-terminal.Gnome-terminal myhost terminal title"

// TestParseWindowLine covers the column contract (per wmctrl's
// list_windows source) and the App fix: WM_CLASS's class component,
// not the client machine.
func TestParseWindowLine(t *testing.T) {
	w, ok := parseWindowLine(windowLine, nil)
	if !ok {
		t.Fatal("expected the sample line to parse")
	}
	if w.ID != "0x03a00007" {
		t.Errorf("ID = %q", w.ID)
	}
	if w.App != "Gnome-terminal" {
		t.Errorf("App = %q, want the WM_CLASS class component", w.App)
	}
	if w.PID != 13487 {
		t.Errorf("PID = %d", w.PID)
	}
	want := core.Rect{X: 640, Y: 24, W: 640, H: 480}
	if w.Bounds != want {
		t.Errorf("Bounds = %+v, want %+v", w.Bounds, want)
	}
	if w.Title != "terminal title" {
		t.Errorf("Title = %q", w.Title)
	}
}

// TestParseWindowLineDesktopFilter covers CurrentDesktopOnly: windows
// on other desktops drop, the current one and sticky (-1) stay.
func TestParseWindowLineDesktopFilter(t *testing.T) {
	cur := 1
	if _, ok := parseWindowLine(windowLine, &cur); !ok {
		t.Error("current-desktop window must stay")
	}
	other := strings.Replace(windowLine, " 1 13487", " 2 13487", 1)
	if _, ok := parseWindowLine(other, &cur); ok {
		t.Error("other-desktop window must drop")
	}
	sticky := strings.Replace(windowLine, " 1 13487", " -1 13487", 1)
	if _, ok := parseWindowLine(sticky, &cur); !ok {
		t.Error("sticky (-1) window must stay")
	}
	// Unfiltered (nil current): the other-desktop window stays.
	if _, ok := parseWindowLine(other, nil); !ok {
		t.Error("nil current must not filter anything")
	}
}

// TestParseWindowLineMalformed guards the len floor and garbage rows.
func TestParseWindowLineMalformed(t *testing.T) {
	if _, ok := parseWindowLine("", nil); ok {
		t.Error("empty line must not parse")
	}
	if _, ok := parseWindowLine("0x1 0 1 2 3 4 5 6 7", nil); ok {
		t.Error("short line must not parse")
	}
	if _, ok := parseWindowLine("garbage", nil); ok {
		t.Error("garbage line must not parse")
	}
}

// TestAppClass covers the class extraction: first-dot split, and the
// no-dot defensive case.
func TestAppClass(t *testing.T) {
	cases := map[string]string{
		"Navigator.Firefox":     "Firefox",
		"gnome-term.Gnome-term": "Gnome-term",
		"no-dot":                "no-dot",
		"":                      "",
	}
	for in, want := range cases {
		if got := appClass(in); got != want {
			t.Errorf("appClass(%q) = %q, want %q", in, got, want)
		}
	}
}

// TestParseCurrentDesktop covers the '*' marker pick.
func TestParseCurrentDesktop(t *testing.T) {
	out := `0 - DG: 1280x1024  VP: 0,0  WA: 0,0 1280x998  Desk 1
1 * DG: 1280x1024  VP: 0,0  WA: 0,0 1280x998  Desk 2
2 - DG: 1280x1024  VP: 0,0  WA: 0,0 1280x998  Desk 3
`
	n, err := parseCurrentDesktop(out)
	if err != nil || n != 1 {
		t.Errorf("parseCurrentDesktop = %d, %v; want 1, nil", n, err)
	}
	if _, err := parseCurrentDesktop("0 - DG: 1280x1024"); err == nil {
		t.Error("expected error when no desktop is marked current")
	}
}
