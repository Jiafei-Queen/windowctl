//go:build linux

package sway

import (
	"encoding/json"
	"errors"
	"fmt"
	"strconv"

	"github.com/muthuishere/windowctl/internal/core"
)

// errNoMatch reports a window ID that no longer resolves — wrapped as
// core.ErrNoMatch so the public layer's match semantics hold.
var errNoMatch = core.ErrNoMatch

// noMatchingNode is sway's criteria-miss verdict (exit code 2,
// {"success": false, "error": "No matching node."}).
const noMatchingNode = "No matching node."

// verdict is one element of swaymsg's JSON command result.
type verdict struct {
	Success bool   `json:"success"`
	Error   string `json:"error"`
}

func parseVerdicts(payload string) ([]verdict, error) {
	var vs []verdict
	if err := json.Unmarshal([]byte(payload), &vs); err != nil {
		return nil, fmt.Errorf("parse sway command result: %w", err)
	}
	if len(vs) == 0 {
		return nil, errors.New("sway returned no command verdicts")
	}
	return vs, nil
}

// geo is the shared {x,y,width,height} rect of tree nodes. Field
// spellings are sway IPC's (lowercase, full words), not core.Rect's.
type geo struct {
	X      int `json:"x"`
	Y      int `json:"y"`
	Width  int `json:"width"`
	Height int `json:"height"`
}

// winProps is the XWayland-only window_properties block (absent —
// null — on native Wayland windows).
type winProps struct {
	Class *string `json:"class"`
}

// node is one sway tree node. Most fields are kept as pointers because
// the IPC reports null for everything that is not a window leaf
// (outputs, workspaces, split containers).
type node struct {
	ID            int64     `json:"id"`
	Type          string    `json:"type"`
	Name          *string   `json:"name"`
	AppID         *string   `json:"app_id"`
	Shell         *string   `json:"shell"`
	PID           *int      `json:"pid"`
	Rect          geo       `json:"rect"`
	DecoRect      geo       `json:"deco_rect"`
	Floating      *string   `json:"floating"`
	Focused       bool      `json:"focused"`
	Visible       bool      `json:"visible"`
	WinProps      *winProps `json:"window_properties"`
	Nodes         []*node   `json:"nodes"`
	FloatingNodes []*node   `json:"floating_nodes"`
}

// tree is the get_tree root (type "root").
type tree = node

// output is one element of get_outputs.
type output struct {
	Name             string  `json:"name"`
	Active           bool    `json:"active"`
	Primary          bool    `json:"primary"`
	Focused          bool    `json:"focused"`
	Scale            float64 `json:"scale"`
	Rect             geo     `json:"rect"`
	CurrentWorkspace string  `json:"current_workspace"`
}

func parseTree(payload string) (*tree, error) {
	var t tree
	if err := json.Unmarshal([]byte(payload), &t); err != nil {
		return nil, fmt.Errorf("parse get_tree output: %w", err)
	}
	return &t, nil
}

func parseOutputs(payload string) ([]output, error) {
	var os []output
	if err := json.Unmarshal([]byte(payload), &os); err != nil {
		return nil, fmt.Errorf("parse get_outputs output: %w", err)
	}
	return os, nil
}

// isWindow reports whether a node is an application window in the
// same sense as KWin's normalWindow / GNOME's type=NORMAL filter:
// real client surfaces, not output/workspace/split-container chrome.
// A leaf is a window when it has a Wayland app_id or X11 WM_CLASS
// (XWayland windows have shell "x11", a null app_id and a populated
// window_properties block). swaybar never appears in the tree.
func (n *node) isWindow() bool {
	if n.Type != "con" && n.Type != "floating_con" {
		return false
	}
	if (n.AppID != nil && *n.AppID != "") ||
		(n.WinProps != nil && n.WinProps.Class != nil && *n.WinProps.Class != "") {
		return true
	}
	return false
}

// isFloating: type floating_con ⇔ the container is floating in this
// layout ("auto_off"/"user_off" tiled, "auto_on"/"user_on" floating —
// the type field switches in lockstep, verified on sway 1.10.1).
func (n *node) isFloating() bool { return n.Type == "floating_con" }

// app resolves the window's application identity: Wayland app_id,
// falling back to the X11 WM_CLASS for XWayland windows.
func (n *node) app() string {
	if n.AppID != nil && *n.AppID != "" {
		return *n.AppID
	}
	if n.WinProps != nil && n.WinProps.Class != nil {
		return *n.WinProps.Class
	}
	return ""
}

// walkWindows returns every window node in tree order (depth-first,
// tiling layout order — sway has no stacking-order query, so this is
// enumeration order, not z-order; z position is only asserted via the
// `focused` flag). Floating windows come after their workspace's
// tiled ones, matching sway's own floating_nodes placement.
func walkWindows(t *tree) []*node {
	var out []*node
	var walk func(n *node)
	walk = func(n *node) {
		if n.isWindow() {
			out = append(out, n)
		}
		for _, c := range n.Nodes {
			walk(c)
		}
		for _, c := range n.FloatingNodes {
			walk(c)
		}
	}
	walk(t)
	return out
}

// findNode locates the node with the given ID anywhere in the tree —
// including non-window nodes, though callers always pass window IDs.
func findNode(t *tree, id int64) *node {
	var walk func(n *node) *node
	walk = func(n *node) *node {
		if n.ID == id {
			return n
		}
		for _, c := range append(append([]*node{}, n.Nodes...), n.FloatingNodes...) {
			if r := walk(c); r != nil {
				return r
			}
		}
		return nil
	}
	return walk(t)
}

// parseID converts the public string Window.ID back to sway's numeric
// node id. A non-numeric ID can only come from another backend's
// session — treated exactly like a vanished window.
func parseID(windowID string) (int64, error) {
	id, err := strconv.ParseInt(windowID, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("sway: window id %q not found: %w", windowID, errNoMatch)
	}
	return id, nil
}

// ListWindows enumerates application windows. Bounds are the tree's
// `rect` — client area + border, excluding the SSD title bar, in
// absolute screen points (see sway-mechanism.md §6).
func (b *Backend) ListWindows() ([]core.Window, error) {
	payload, err := runQuery("get_tree")
	if err != nil {
		return nil, err
	}
	t, err := parseTree(payload)
	if err != nil {
		return nil, err
	}
	nodes := walkWindows(t)
	ws := make([]core.Window, 0, len(nodes))
	for _, n := range nodes {
		title := ""
		if n.Name != nil {
			title = *n.Name
		}
		pid := 0
		if n.PID != nil {
			pid = *n.PID
		}
		ws = append(ws, core.Window{
			ID:     strconv.FormatInt(n.ID, 10),
			Title:  title,
			App:    n.app(),
			PID:    pid,
			Bounds: core.Rect{X: n.Rect.X, Y: n.Rect.Y, W: n.Rect.Width, H: n.Rect.Height},
		})
	}
	return ws, nil
}

// ListMonitors reports outputs. Focused is the output containing the
// focused window's centroid (the KWin adapter's semantics; sway has
// no cursor query, so Active — "cursor over this monitor" — is
// always false). Primary uses sway's real primary flag; when no
// output is flagged primary (the VM's QEMU display is one), the
// focused output is promoted so monitors list always names one.
func (b *Backend) ListMonitors() ([]core.Monitor, error) {
	os, err := b.outputs()
	if err != nil {
		return nil, err
	}
	focusedNode := b.focusedWindow()
	var focusedOut = -1
	for i := range os {
		if focusedNode != nil && contains(os[i], focusedNode) {
			focusedOut = i
			break
		}
	}
	primaryOut := -1
	for i := range os {
		if os[i].Primary {
			primaryOut = i
			break
		}
	}
	if primaryOut == -1 && focusedOut != -1 {
		primaryOut = focusedOut
	}

	ms := make([]core.Monitor, 0, len(os))
	for i := range os {
		ms = append(ms, core.Monitor{
			// The public layer re-sorts by origin and re-indexes
			// 1..N (see sortMonitors); these IDs just need to be
			// stable and unique within one call.
			ID:      i + 1,
			X:       os[i].Rect.X,
			Y:       os[i].Rect.Y,
			Width:   os[i].Rect.Width,
			Height:  os[i].Rect.Height,
			Primary: i == primaryOut,
			Focused: i == focusedOut,
			Active:  false, // sway IPC has no cursor query (sway-mechanism.md §10)
		})
	}
	return ms, nil
}

// outputs returns the active outputs from get_outputs.
func (b *Backend) outputs() ([]output, error) {
	payload, err := runQuery("get_outputs")
	if err != nil {
		return nil, err
	}
	os, err := parseOutputs(payload)
	if err != nil {
		return nil, err
	}
	active := os[:0]
	for _, o := range os {
		if o.Active {
			active = append(active, o)
		}
	}
	return active, nil
}

// focusedWindow returns the single tree node with focused=true, or
// nil when nothing is focused (e.g. an empty workspace).
func (b *Backend) focusedWindow() *node {
	payload, err := runQuery("get_tree")
	if err != nil {
		return nil
	}
	t, err := parseTree(payload)
	if err != nil {
		return nil
	}
	for _, n := range walkWindows(t) {
		if n.Focused {
			return n
		}
	}
	return nil
}

// contains reports whether the window's centroid lies inside the
// output's rect.
func contains(o output, n *node) bool {
	cx := n.Rect.X + n.Rect.Width/2
	cy := n.Rect.Y + n.Rect.Height/2
	return cx >= o.Rect.X && cx < o.Rect.X+o.Rect.Width &&
		cy >= o.Rect.Y && cy < o.Rect.Y+o.Rect.Height
}

// CursorPosition is unavailable on sway: the IPC exposes no cursor
// query at all (get_tree/get_outputs/get_seats verified —
// sway-mechanism.md §10). ydotool injects input but cannot read the
// pointer back.
func (b *Backend) CursorPosition() (int, int, error) {
	return 0, 0, fmt.Errorf("sway: cursor position unavailable (sway IPC exposes no cursor query): %w",
		core.ErrNotImplemented)
}

// Move repositions and resizes the window to bounds (absolute screen
// points, same space ListWindows reports).
//
// Tiled windows cannot be absolutely positioned in sway (layout
// semantics); they are floated first — the decided adapter policy,
// documented as changing the user's tiling layout.
//
// Geometry compensation (sway-mechanism.md §6): sway's move/resize
// commands target the container *including* the SSD title bar, while
// the reported rect excludes it. With deco_h = deco_rect.height:
//
//	resize set width W px height (H + deco_h) px  → rect.w ≈ W, rect.h ≈ H
//	move absolute position X px (Y - deco_h) px   → rect.x = X, rect.y = Y
//
// Resize runs before the final move: client-side size negotiation
// can shift the window a pixel or two, and the move must land last.
func (b *Backend) Move(windowID string, bounds core.Rect) error {
	id, err := parseID(windowID)
	if err != nil {
		return err
	}
	t, err := loadTree()
	if err != nil {
		return err
	}
	n := findNode(t, id)
	if n == nil {
		return fmt.Errorf("sway: window %s not found (may have been closed): %w", windowID, errNoMatch)
	}
	if !n.isFloating() {
		// Floated windows gain a title bar (SSD), which changes
		// deco_rect — re-read the node after toggling.
		if err := runCommand("["+fmt.Sprintf("con_id=%d", id)+"]", "floating", "enable"); err != nil {
			return err
		}
		t, err = loadTree()
		if err != nil {
			return err
		}
		n = findNode(t, id)
		if n == nil {
			return fmt.Errorf("sway: window %s not found (may have been closed): %w", windowID, errNoMatch)
		}
	}
	decoH := n.DecoRect.Height
	criteria := "[" + fmt.Sprintf("con_id=%d", id) + "]"
	if err := runCommand(criteria, "resize", "set", "width",
		strconv.Itoa(bounds.W), "px", "height",
		strconv.Itoa(bounds.H+decoH), "px"); err != nil {
		return err
	}
	return runCommand(criteria, "move", "absolute", "position",
		strconv.Itoa(bounds.X), "px",
		strconv.Itoa(bounds.Y-decoH), "px")
}

// Focus raises and activates the window.
func (b *Backend) Focus(windowID string) error {
	id, err := parseID(windowID)
	if err != nil {
		return err
	}
	return runCommand("["+fmt.Sprintf("con_id=%d", id)+"]", "focus")
}

func loadTree() (*tree, error) {
	payload, err := runQuery("get_tree")
	if err != nil {
		return nil, err
	}
	return parseTree(payload)
}
