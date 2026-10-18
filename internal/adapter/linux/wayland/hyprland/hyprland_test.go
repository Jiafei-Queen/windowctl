//go:build linux

package hyprland

import (
	"errors"
	"testing"

	"github.com/muthuishere/windowctl/internal/core"
)

// clientsFixture mirrors the live `hyprctl -j clients` shape recorded
// in docs/wayland/hyprland-mechanism.md (Hyprland 0.55.2, scale-2
// monitor): one native Wayland client, one XWayland client, one
// unmapped special-workspace client that must be filtered out.
const clientsFixture = `[
  {
    "address": "0x5631bd3f6d30", "class": "foot", "title": "jiafei114@debian:~",
    "initialClass": "foot", "pid": 14307, "xwayland": false,
    "at": [21, 21], "size": [1126, 589],
    "floating": false, "fullscreen": 0, "mapped": true, "hidden": false,
    "pinned": false, "monitor": 0,
    "workspace": {"id": 1, "name": "1"},
    "focusHistoryID": 0
  },
  {
    "address": "0x55f1aa2b1110", "class": "XClock", "title": "xclock",
    "initialClass": "xclock", "pid": 4242, "xwayland": true,
    "at": [1200, 100], "size": [300, 300],
    "floating": true, "fullscreen": 0, "mapped": true, "hidden": false,
    "pinned": false, "monitor": 0,
    "workspace": {"id": 1, "name": "1"},
    "focusHistoryID": 1
  },
  {
    "address": "0x55f1aa2b2222", "class": "hidden-app", "title": "hidden",
    "pid": 77, "at": [0, 0], "size": [10, 10],
    "floating": true, "fullscreen": 0, "mapped": false, "hidden": true,
    "pinned": false, "monitor": 0,
    "workspace": {"id": 2, "name": "2"},
    "focusHistoryID": 2
  }
]`

func TestParseClients(t *testing.T) {
	cs, err := parseClients(clientsFixture)
	if err != nil {
		t.Fatalf("parseClients: %v", err)
	}
	if len(cs) != 3 {
		t.Fatalf("got %d clients, want 3", len(cs))
	}
	if !cs[0].isWindow() || !cs[1].isWindow() {
		t.Error("clients 0/1 should pass the window filter")
	}
	if cs[2].isWindow() {
		t.Error("unmapped+hidden client must be filtered out")
	}
	if cs[0].Address != "0x5631bd3f6d30" || cs[0].Class != "foot" ||
		cs[0].At != [2]int{21, 21} || cs[0].Size != [2]int{1126, 589} {
		t.Errorf("client[0] = %+v", cs[0])
	}
}

func TestListWindowsMapping(t *testing.T) {
	cs, err := parseClients(clientsFixture)
	if err != nil {
		t.Fatalf("parseClients: %v", err)
	}
	var got []core.Window
	for i := range cs {
		if !cs[i].isWindow() {
			continue
		}
		got = append(got, core.Window{
			ID:     cs[i].Address,
			Title:  cs[i].Title,
			App:    cs[i].Class,
			PID:    cs[i].PID,
			Bounds: core.Rect{X: cs[i].At[0], Y: cs[i].At[1], W: cs[i].Size[0], H: cs[i].Size[1]},
		})
	}
	if len(got) != 2 {
		t.Fatalf("got %d windows, want 2", len(got))
	}
	want := core.Window{
		ID: "0x5631bd3f6d30", Title: "jiafei114@debian:~", App: "foot", PID: 14307,
		Bounds: core.Rect{X: 21, Y: 21, W: 1126, H: 589},
	}
	if got[0] != want {
		t.Errorf("window[0] = %+v, want %+v", got[0], want)
	}
	if got[1].App != "XClock" {
		t.Errorf("xwayland app = %q, want XClock", got[1].App)
	}
}

// monitorsFixture mirrors `hyprctl -j monitors` on the VM: the
// reported width/height are the output MODE's pixels (2336×1261 with
// scale 2.0 → logical 1168×630), there is no primary flag, and
// disabled outputs must be filtered.
const monitorsFixture = `[
  {
    "id": 0, "name": "Virtual-1", "description": "",
    "x": 0, "y": 0, "width": 2336, "height": 1261, "scale": 2.0,
    "refreshRate": 60.0, "focused": true, "disabled": false,
    "activeWorkspace": {"id": 1, "name": "1"},
    "availableModes": ["2336x1261@60.00Hz"]
  }
]`

func TestParseMonitors(t *testing.T) {
	ms, err := parseMonitors(monitorsFixture)
	if err != nil {
		t.Fatalf("parseMonitors: %v", err)
	}
	if len(ms) != 1 {
		t.Fatalf("got %d monitors, want 1", len(ms))
	}
	m := ms[0]
	if m.Name != "Virtual-1" || m.Width != 2336 || m.Height != 1261 ||
		m.Scale != 2.0 || !m.Focused {
		t.Errorf("monitor[0] = %+v", m)
	}
}

func TestMonitorLogicalBox(t *testing.T) {
	// width/scale → logical: 2336/2 × 1261/2 = 1168×630 (630.5 floor).
	ms, _ := parseMonitors(monitorsFixture)
	m := ms[0]
	scale := m.Scale
	w := int(float64(m.Width) / scale)
	h := int(float64(m.Height) / scale)
	if w != 1168 || h != 630 {
		t.Errorf("logical box = %dx%d, want 1168x630", w, h)
	}
}

func TestParseCursorPos(t *testing.T) {
	// hyprctl cursorpos prints "x, y".
	for _, tc := range []struct {
		out     string
		x, y    int
		wantErr bool
	}{
		{"300, 200\n", 300, 200, false},
		{"0, 0\n", 0, 0, false},
		{"garbage", 0, 0, true},
		{"300, 200, 400\n", 0, 0, true},
	} {
		x, y, err := parseCursorPos(tc.out)
		if tc.wantErr {
			if err == nil {
				t.Errorf("%q: expected parse failure, got (%d,%d)", tc.out, x, y)
			}
			continue
		}
		if err != nil {
			t.Errorf("%q: %v", tc.out, err)
			continue
		}
		if x != tc.x || y != tc.y {
			t.Errorf("%q: parsed (%d,%d), want (%d,%d)", tc.out, x, y, tc.x, tc.y)
		}
	}
}

func TestDispatchVerdict(t *testing.T) {
	err := dispatchVerdict([]string{"focuswindow", "address:0xdeadbeef"}, "No such window found")
	if err == nil || !errors.Is(err, core.ErrNoMatch) {
		t.Fatalf("criteria miss should map to ErrNoMatch, got %v", err)
	}

	err = dispatchVerdict([]string{"togglefloating", "address:0xdeadbeef"}, "No such window found")
	if err == nil || !errors.Is(err, core.ErrNoMatch) {
		t.Fatalf("criteria miss should map to ErrNoMatch, got %v", err)
	}

	err = dispatchVerdict([]string{"movecursor", "300", "200"}, "ok")
	if err != nil {
		t.Fatalf("ok verdict should be nil, got %v", err)
	}

	err = dispatchVerdict([]string{"no-such-dispatcher", "foo"}, "Invalid dispatcher")
	if err == nil || errors.Is(err, core.ErrNoMatch) {
		t.Fatalf("non-criteria error should not be ErrNoMatch, got %v", err)
	}
}
