//go:build linux

package hyprland

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os/exec"
	"strconv"
	"strings"

	"github.com/muthuishere/windowctl/internal/core"
)

// errNoMatch reports a window ID that no longer resolves — wrapped as
// core.ErrNoMatch so the public layer's match semantics hold.
var errNoMatch = core.ErrNoMatch

// noSuchWindow is hyprctl's criteria-miss verdict (printed on stdout
// with exit code 0 — hyprland-mechanism.md).
const noSuchWindow = "No such window found"

// client is one element of `hyprctl -j clients`. at/size are the
// window's logical box (absolute layout coordinates in points — the
// same space core.Rect uses), verified on Hyprland 0.55.2 with a
// scale-2 monitor.
type client struct {
	Address    string `json:"address"`
	Class      string `json:"class"`
	Title      string `json:"title"`
	PID        int    `json:"pid"`
	At         [2]int `json:"at"`
	Size       [2]int `json:"size"`
	Floating   bool   `json:"floating"`
	Fullscreen int    `json:"fullscreen"`
	Mapped     bool   `json:"mapped"`
	Hidden     bool   `json:"hidden"`
	Workspace  struct {
		ID   int    `json:"id"`
		Name string `json:"name"`
	} `json:"workspace"`
	Monitor int `json:"monitor"`
}

// monitor is one element of `hyprctl -j monitors`. width/height are
// the OUTPUT MODE's pixels (physical), not logical — the logical
// desktop is width/scale × height/scale; window at/size are logical.
// There is no primary flag on 0.55.2; the focused monitor is
// promoted instead (same fallback as the sway backend).
type monitor struct {
	ID       int     `json:"id"`
	Name     string  `json:"name"`
	X        int     `json:"x"`
	Y        int     `json:"y"`
	Width    int     `json:"width"`
	Height   int     `json:"height"`
	Scale    float64 `json:"scale"`
	Focused  bool    `json:"focused"`
	Disabled bool    `json:"disabled"`
}

func parseClients(payload string) ([]client, error) {
	var cs []client
	if err := json.Unmarshal([]byte(payload), &cs); err != nil {
		return nil, fmt.Errorf("parse hyprctl clients: %w", err)
	}
	return cs, nil
}

func parseMonitors(payload string) ([]monitor, error) {
	var ms []monitor
	if err := json.Unmarshal([]byte(payload), &ms); err != nil {
		return nil, fmt.Errorf("parse hyprctl monitors: %w", err)
	}
	return ms, nil
}

// isWindow filters to real application windows: mapped, not hidden,
// with a class (e.g. stray empty-class clients of special surfaces).
// Layer-shell surfaces (bars/docks) never appear in hyprctl clients.
func (c *client) isWindow() bool {
	return c.Mapped && !c.Hidden && c.Class != ""
}

// ListWindows enumerates application windows in Hyprland's own list
// order (most-recently-focused first — focusHistoryID order).
// Bounds are the client's logical box: at + size.
func (b *Backend) ListWindows() ([]core.Window, error) {
	payload, err := runQuery("clients")
	if err != nil {
		return nil, err
	}
	cs, err := parseClients(payload)
	if err != nil {
		return nil, err
	}
	ws := make([]core.Window, 0, len(cs))
	for i := range cs {
		c := &cs[i]
		if !c.isWindow() {
			continue
		}
		ws = append(ws, core.Window{
			ID:     c.Address,
			Title:  c.Title,
			App:    c.Class,
			PID:    c.PID,
			Bounds: core.Rect{X: c.At[0], Y: c.At[1], W: c.Size[0], H: c.Size[1]},
		})
	}
	return ws, nil
}

// findClient resolves a window ID (Hyprland's address string, e.g.
// "0x5631bd3f6d30") among current clients.
func (b *Backend) findClient(windowID string) (*client, error) {
	payload, err := runQuery("clients")
	if err != nil {
		return nil, err
	}
	cs, err := parseClients(payload)
	if err != nil {
		return nil, err
	}
	for i := range cs {
		if cs[i].Address == windowID {
			return &cs[i], nil
		}
	}
	return nil, fmt.Errorf("hyprland: window %s not found (may have been closed): %w", windowID, errNoMatch)
}

// Move repositions and resizes the window to bounds (absolute layout
// points, the same space ListWindows reports).
//
// Tiled windows cannot take exact pixel geometry in Hyprland
// (movewindowpixel exact on a tiled client re-arranges within the
// layout instead); they are floated first — the same decided adapter
// policy as sway. No title-bar compensation is needed: exact
// dispatches land at/size precisely on the default border-only
// decoration (verified at scale 2).
//
// resizewindowpixel recenters the window around its midpoint, so the
// resize runs before the final move.
func (b *Backend) Move(windowID string, bounds core.Rect) error {
	c, err := b.findClient(windowID)
	if err != nil {
		return err
	}
	target := "address:" + c.Address
	if !c.Floating {
		if err := runDispatch("togglefloating", target); err != nil {
			return err
		}
	}
	if err := runDispatch("resizewindowpixel",
		fmt.Sprintf("exact %d %d,%s", bounds.W, bounds.H, target)); err != nil {
		return err
	}
	return runDispatch("movewindowpixel",
		fmt.Sprintf("exact %d %d,%s", bounds.X, bounds.Y, target))
}

// Focus raises and activates the window.
func (b *Backend) Focus(windowID string) error {
	c, err := b.findClient(windowID)
	if err != nil {
		return err
	}
	return runDispatch("focuswindow", "address:"+c.Address)
}

// MoveCursor positions the OS cursor at the exact logical point via
// Hyprland's own movecursor dispatcher. ydotool's --absolute is NOT
// usable as a positioning primitive here: its absolute emulation
// (an INT32_MIN relative slam + offset, per Client/tool_mousemove.c)
// runs through libinput's pointer acceleration, which on a default
// config lands at 2× the request — verified: with adaptive accel
// `--absolute 100,100` puts the cursor at (200,200) regardless of
// monitor scale, and only becomes 1:1 with acceleration disabled.
// movecursor has no such dependence (exact at any config, verified).
func (b *Backend) MoveCursor(x, y int) error {
	return runDispatch("movecursor", strconv.Itoa(x), strconv.Itoa(y))
}

// ListMonitors reports outputs. Active is the monitor under the OS
// cursor (real signal — hyprctl cursorpos); Focused is the monitor
// containing the focused window's centroid. Primary uses the focused
// monitor when no output declares one (0.55.2 has no primary flag).
//
// Bounds are logical: the reported width/height are the output
// mode's pixels, so they are divided by scale.
func (b *Backend) ListMonitors() ([]core.Monitor, error) {
	payload, err := runQuery("monitors")
	if err != nil {
		return nil, err
	}
	ms, err := parseMonitors(payload)
	if err != nil {
		return nil, err
	}

	// Logical geometry per output.
	type lbox struct {
		x, y, w, h int
	}
	boxes := make([]lbox, len(ms))
	for i := range ms {
		scale := ms[i].Scale
		if scale < 1 {
			scale = 1
		}
		boxes[i] = lbox{
			x: ms[i].X, y: ms[i].Y,
			w: int(float64(ms[i].Width) / scale),
			h: int(float64(ms[i].Height) / scale),
		}
	}

	// Active: monitor under the cursor.
	cx, cy, err := b.CursorPosition()
	var activeIdx = -1
	if err == nil {
		for i := range boxes {
			if cx >= boxes[i].x && cx < boxes[i].x+boxes[i].w &&
				cy >= boxes[i].y && cy < boxes[i].y+boxes[i].h {
				activeIdx = i
				break
			}
		}
	}

	// Focused: monitor containing the active window's centroid.
	focusedIdx := -1
	if av, err := runQuery("activewindow"); err == nil {
		var c client
		if json.Unmarshal([]byte(av), &c) == nil && c.Address != "" {
			cxc := c.At[0] + c.Size[0]/2
			cyc := c.At[1] + c.Size[1]/2
			for i := range boxes {
				if cxc >= boxes[i].x && cxc < boxes[i].x+boxes[i].w &&
					cyc >= boxes[i].y && cyc < boxes[i].y+boxes[i].h {
					focusedIdx = i
					break
				}
			}
		}
	}

	primaryIdx := -1
	if focusedIdx != -1 {
		primaryIdx = focusedIdx
	}

	out := make([]core.Monitor, 0, len(ms))
	for i := range ms {
		if ms[i].Disabled {
			continue
		}
		out = append(out, core.Monitor{
			// The public layer re-sorts by origin and re-indexes
			// 1..N (see sortMonitors); these IDs just need to be
			// stable and unique within one call.
			ID:      i + 1,
			X:       boxes[i].x,
			Y:       boxes[i].y,
			Width:   boxes[i].w,
			Height:  boxes[i].h,
			Primary: i == primaryIdx,
			Focused: i == focusedIdx,
			Active:  i == activeIdx,
		})
	}
	return out, nil
}

// CursorPosition reads the pointer position (logical points) from
// `hyprctl cursorpos` — "x, y".
func (b *Backend) CursorPosition() (int, int, error) {
	if _, err := exec.LookPath("hyprctl"); err != nil {
		return 0, 0, fmt.Errorf("hyprctl not found on PATH — required for window management on Hyprland. "+
			"Install it (it ships with Hyprland): %w", err)
	}
	cmd := exec.Command("hyprctl", "cursorpos")
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return 0, 0, fmt.Errorf("hyprland: hyprctl cursorpos: %w (stderr: %s)", err, strings.TrimSpace(stderr.String()))
	}
	x, y, err := parseCursorPos(stdout.String())
	if err != nil {
		return 0, 0, err
	}
	return x, y, nil
}

// parseCursorPos parses hyprctl cursorpos output ("300, 200").
func parseCursorPos(out string) (int, int, error) {
	out = strings.TrimSpace(out)
	parts := strings.Split(out, ",")
	if len(parts) != 2 {
		return 0, 0, fmt.Errorf("hyprland: hyprctl cursorpos: unexpected output %q", out)
	}
	x, err := strconv.Atoi(strings.TrimSpace(parts[0]))
	if err != nil {
		return 0, 0, fmt.Errorf("hyprland: hyprctl cursorpos: unexpected output %q", out)
	}
	y, err := strconv.Atoi(strings.TrimSpace(parts[1]))
	if err != nil {
		return 0, 0, fmt.Errorf("hyprland: hyprctl cursorpos: unexpected output %q", out)
	}
	return x, y, nil
}
