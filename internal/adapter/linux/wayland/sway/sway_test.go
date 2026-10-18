//go:build linux

package sway

import (
	"errors"
	"strconv"
	"strings"
	"testing"

	"github.com/Jiafei-Queen/windowctl/internal/core"
)

// treeFixture mirrors the live sway 1.10.1 get_tree shape recorded in
// docs/wayland/sway-mechanism.md: root → output → workspace, one
// native Wayland window split between two containers (id 16 tiled →
// floated during the spike), one XWayland window, and chrome nodes.
const treeFixture = `{
  "id": 1, "type": "root", "name": "root", "rect": {"x":0,"y":0,"width":2336,"height":1261},
  "nodes": [
    {
      "id": 2147483647, "type": "output", "name": "__i3",
      "nodes": [
        {"id": 2147483646, "type": "workspace", "name": "__i3_scratch",
         "nodes": [], "floating_nodes": []}
      ],
      "floating_nodes": []
    },
    {
      "id": 3, "type": "output", "name": "Virtual-1",
      "focus": [4],
      "rect": {"x":0,"y":0,"width":2336,"height":1261},
      "nodes": [
        {
          "id": 4, "type": "workspace", "name": "1",
          "rect": {"x":0,"y":20,"width":2336,"height":1241},
          "nodes": [
            {
              "id": 17, "type": "con", "name": "foot term", "app_id": "foot",
              "shell": "xdg_shell", "pid": 10997, "focused": false,
              "floating": "auto_off",
              "rect": {"x":0,"y":42,"width":1167,"height":1219},
              "deco_rect": {"x":0,"y":20,"width":1167,"height":22},
              "window_rect": {"x":2,"y":0,"width":1163,"height":1217},
              "nodes": [], "floating_nodes": [], "focus": []
            },
            {
              "id": 7, "type": "con", "name": "xclock", "app_id": null,
              "shell": "x11", "pid": 4242, "focused": false,
              "window_properties": {"class": "XClock", "instance": "xclock"},
              "rect": {"x":1167,"y":42,"width":1169,"height":1219},
              "deco_rect": {"x":1167,"y":20,"width":1169,"height":22},
              "nodes": [], "floating_nodes": [], "focus": []
            }
          ],
          "floating_nodes": [
            {
              "id": 18, "type": "floating_con", "name": "foot float 2",
              "app_id": "foot", "shell": "xdg_shell", "pid": 10997,
              "focused": false, "floating": "user_on",
              "rect": {"x":900,"y":600,"width":599,"height":398},
              "deco_rect": {"x":900,"y":558,"width":599,"height":22},
              "window_rect": {"x":2,"y":0,"width":595,"height":376},
              "nodes": [], "floating_nodes": [], "focus": []
            },
            {
              "id": 16, "type": "floating_con", "name": "foot float",
              "app_id": "foot", "shell": "xdg_shell", "pid": 10997,
              "focused": true, "floating": "user_on",
              "rect": {"x":500,"y":500,"width":599,"height":398},
              "deco_rect": {"x":500,"y":458,"width":599,"height":22},
              "window_rect": {"x":2,"y":0,"width":595,"height":376},
              "nodes": [], "floating_nodes": [], "focus": []
            }
          ],
          "focus": [16, 18, 17, 7]
        }
      ],
      "floating_nodes": []
    }
  ],
  "floating_nodes": [],
  "focus": [3, 2147483647]
}`

func TestWalkWindows(t *testing.T) {
	tt, err := parseTree(treeFixture)
	if err != nil {
		t.Fatalf("parseTree: %v", err)
	}
	ws := walkWindows(tt)
	if len(ws) != 4 {
		t.Fatalf("got %d windows, want 4 (3 wayland + 1 xwayland; chrome excluded): %+v", len(ws), ids(ws))
	}
	// Order is FOCUS-RECENCY (frontmost first), walked through each
	// container's `focus` array — NOT the nodes/floating_nodes list
	// order, which is creation order (the fixture puts floating node
	// 18 before the focused 16 in floating_nodes while the focus
	// array has 16 first). The focused floating window (16) must come
	// out on top: the public layer's stampFocused marks the first
	// window of this list on the focused monitor.
	if got, want := ids(ws), []string{"16", "18", "17", "7"}; strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("order = %v, want %v", got, want)
	}
}

func ids(ws []*node) []string {
	out := make([]string, len(ws))
	for i, w := range ws {
		out[i] = strconv.FormatInt(w.ID, 10)
	}
	return out
}

func TestWindowFields(t *testing.T) {
	tt, err := parseTree(treeFixture)
	if err != nil {
		t.Fatalf("parseTree: %v", err)
	}
	n := findNode(tt, 16)
	if n == nil {
		t.Fatal("node 16 not found")
	}
	if got := n.app(); got != "foot" {
		t.Errorf("app = %q, want foot", got)
	}
	if !n.isFloating() {
		t.Error("node 16 (floating_con) should be floating")
	}
	if n.DecoRect.Height != 22 {
		t.Errorf("deco height = %d, want 22 (SSD title bar)", n.DecoRect.Height)
	}
	if !n.Focused {
		t.Error("node 16 should be focused")
	}

	// Tiled node: floating=false, deco present (SSD applies to tiled
	// windows too — the deco_rect in the fixture is workspace-relative).
	n17 := findNode(tt, 17)
	if n17.isFloating() {
		t.Error("node 17 (con) should not be floating")
	}

	// XWayland window: app falls back to WM_CLASS.
	n7 := findNode(tt, 7)
	if got := n7.app(); got != "XClock" {
		t.Errorf("xwayland app = %q, want XClock", got)
	}
}

func TestFindNodeMiss(t *testing.T) {
	tt, err := parseTree(treeFixture)
	if err != nil {
		t.Fatalf("parseTree: %v", err)
	}
	if n := findNode(tt, 99999); n != nil {
		t.Fatalf("findNode(99999) = %+v, want nil", n)
	}
}

// TestWalkWindowsFocusMissing pins the defensive branch: children
// absent from (or with an empty) focus array must still be walked, in
// tree order — the focus array is documented to cover all children,
// but the walker must not silently drop windows if sway ever omits
// one. No panics, no lost windows, deterministic output.
func TestWalkWindowsFocusMissing(t *testing.T) {
	tt, err := parseTree(`{
      "id": 1, "type": "root",
      "nodes": [
        {
          "id": 2, "type": "workspace",
          "nodes": [
            {"id": 10, "type": "con", "name": "a", "app_id": "a",
             "rect": {"x":0,"y":0,"width":10,"height":10},
             "nodes": [], "floating_nodes": [], "focus": []},
            {"id": 11, "type": "con", "name": "b", "app_id": "b",
             "rect": {"x":10,"y":0,"width":10,"height":10},
             "nodes": [], "floating_nodes": [], "focus": []}
          ],
          "floating_nodes": [],
          "focus": []
        }
      ],
      "floating_nodes": [], "focus": [2]
    }`)
	if err != nil {
		t.Fatalf("parseTree: %v", err)
	}
	ws := walkWindows(tt)
	if len(ws) != 2 {
		t.Fatalf("got %d windows, want 2: %+v", len(ws), ids(ws))
	}
	// Empty focus arrays fall back to tree order.
	if got, want := ids(ws), []string{"10", "11"}; strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("order = %v, want %v", got, want)
	}
}

// outputsFixture mirrors get_outputs on the single-output VM: a real
// primary flag (false — the QEMU display declares none) and focused
// on the only output.
const outputsFixture = `[
  {
    "id": 3, "type": "output", "name": "Virtual-1",
    "active": true, "primary": false, "focused": true,
    "scale": 1.0, "current_workspace": "1",
    "rect": {"x":0,"y":0,"width":2336,"height":1261}
  },
  {
    "id": 99, "type": "output", "name": "HEADLESS-9",
    "active": false, "primary": false, "focused": false,
    "scale": 1.0, "current_workspace": "9",
    "rect": {"x":0,"y":0,"width":800,"height":600}
  }
]`

func TestOutputs(t *testing.T) {
	os, err := parseOutputs(outputsFixture)
	if err != nil {
		t.Fatalf("parseOutputs: %v", err)
	}
	if len(os) != 2 {
		t.Fatalf("got %d outputs, want 2", len(os))
	}
	if os[0].Name != "Virtual-1" || os[0].Rect.Width != 2336 || os[0].Scale != 1.0 {
		t.Errorf("output[0] = %+v", os[0])
	}
	if os[1].Active {
		t.Error("HEADLESS-9 should be inactive")
	}

	// The real filter (the one outputs() feeds ListMonitors with)
	// must drop the inactive output.
	active := activeOutputs(os)
	if len(active) != 1 {
		t.Fatalf("active outputs = %d, want 1", len(active))
	}
	if active[0].Name != "Virtual-1" {
		t.Errorf("active[0] = %s, want Virtual-1", active[0].Name)
	}
}

func TestParseVerdicts(t *testing.T) {
	vs, err := parseVerdicts(`[
      {"success": true},
      {"success": false, "parse_error": false, "error": "No matching node."}
    ]`)
	if err != nil {
		t.Fatalf("parseVerdicts: %v", err)
	}
	if len(vs) != 2 || vs[1].Error != noMatchingNode {
		t.Fatalf("verdicts = %+v", vs)
	}

	err = commandError([]string{"[con_id=9]", "focus"}, noMatchingNode)
	if err == nil || !isNoMatch(err) {
		t.Fatalf("criteria miss should map to ErrNoMatch, got %v", err)
	}

	err = commandError([]string{"[con_id=9]", "focus"}, "Only floating containers can be moved to an absolute position")
	if err == nil || isNoMatch(err) {
		t.Fatalf("non-criteria error should not be ErrNoMatch, got %v", err)
	}
}

func isNoMatch(err error) bool {
	return errors.Is(err, core.ErrNoMatch)
}

func TestParseID(t *testing.T) {
	if id, err := parseID("16"); err != nil || id != 16 {
		t.Fatalf("parseID(16) = %d, %v", id, err)
	}
	if _, err := parseID("{0f0f0f0f-…}"); !errors.Is(err, core.ErrNoMatch) {
		t.Fatalf("non-numeric ID should be ErrNoMatch, got %v", err)
	}
}

// TestActiveOutputsMixed pins the multi-output filter: inactive
// outputs are dropped, active ones (any scale) kept, order preserved.
func TestActiveOutputsMixed(t *testing.T) {
	payload := `[
      {"name":"DP-1","active":true,"primary":true,"focused":false,
       "scale":1.25,"current_workspace":"1",
       "rect":{"x":0,"y":0,"width":1536,"height":864}},
      {"name":"HEADLESS-1","active":false,"primary":false,"focused":false,
       "scale":1.0,"current_workspace":"9",
       "rect":{"x":0,"y":0,"width":800,"height":600}},
      {"name":"DP-2","active":true,"primary":false,"focused":true,
       "scale":2.0,"current_workspace":"2",
       "rect":{"x":1536,"y":0,"width":1168,"height":630}}
    ]`
	os, err := parseOutputs(payload)
	if err != nil {
		t.Fatalf("parseOutputs: %v", err)
	}
	active := activeOutputs(os)
	if len(active) != 2 {
		t.Fatalf("active = %d, want 2 (DP-1 + DP-2, HEADLESS dropped)", len(active))
	}
	if active[0].Name != "DP-1" || active[0].Scale != 1.25 {
		t.Errorf("active[0] = %+v", active[0])
	}
	if active[1].Name != "DP-2" || !active[1].Focused {
		t.Errorf("active[1] = %+v", active[1])
	}
}

// TestParseBadJSON pins that malformed IPC payloads surface as
// descriptive errors, never as partially-populated results.
func TestParseBadJSON(t *testing.T) {
	if _, err := parseTree(`{broken`); err == nil {
		t.Error("parseTree: expected error for invalid JSON")
	}
	if _, err := parseOutputs(`not json`); err == nil {
		t.Error("parseOutputs: expected error for invalid JSON")
	}
}

// TestParseVerdictsBadInput covers the two verdict failure modes:
// unparseable JSON and the zero-verdict reply (swaymsg quirk).
func TestParseVerdictsBadInput(t *testing.T) {
	if _, err := parseVerdicts(`not json`); err == nil {
		t.Error("expected error for invalid JSON")
	}
	if _, err := parseVerdicts(`[]`); err == nil {
		t.Error("expected error for an empty verdict array")
	}
}
