//go:build linux

// Package x11 is the X11 platform adapter for windowctl.
//
// For the os-spikes tracer bullet this implementation shells out to wmctrl
// (and xdotool for focus); native X11 via Xlib is a planned follow-up.
// Selected at runtime by the parent linux package when the session is
// not Wayland; see internal/adapter/linux/adapter.go.
package x11

import (
	"bufio"
	"fmt"
	"os/exec"
	"strconv"
	"strings"

	"github.com/Jiafei-Queen/windowctl/internal/core"
)

type Adapter struct{}

func New() *Adapter { return &Adapter{} }

// listWindows enumerates windows via `wmctrl -lpGx` (-x adds the
// WM_CLASS column). Column order per wmctrl's list_windows source:
//
//	ID desktop pid X Y W H WM_CLASS client-machine title...
//
// with desktop -1 meaning "all desktops" (sticky). When current is
// non-nil the list is narrowed to that desktop, keeping sticky
// windows; nil reports every desktop (the Filter{} contract).
func (a *Adapter) listWindows(current *int) ([]core.Window, error) {
	out, err := exec.Command("wmctrl", "-lpGx").Output()
	if err != nil {
		return nil, fmt.Errorf("wmctrl -lpGx: %w", err)
	}
	var ws []core.Window
	scanner := bufio.NewScanner(strings.NewReader(string(out)))
	for scanner.Scan() {
		w, ok := parseWindowLine(scanner.Text(), current)
		if !ok {
			continue
		}
		ws = append(ws, w)
	}
	return ws, scanner.Err()
}

func (a *Adapter) ListWindows() ([]core.Window, error) {
	return a.listWindows(nil)
}

// ListWindowsFiltered implements the optional desktop-scope capability
// (core.FilteredWindowLister). Only CurrentDesktopOnly / SkipMinimized
// are honored here — Title / App remain the public layer's job.
//
// SkipMinimized is unsupported on this backend: reading a window's
// minimized state needs one `xprop` call per window, an enumeration-
// halting cost. Documented as a no-op (spec 02).
func (a *Adapter) ListWindowsFiltered(filter core.Filter) ([]core.Window, error) {
	if !filter.CurrentDesktopOnly {
		return a.ListWindows()
	}
	cur, err := currentDesktop()
	if err != nil {
		return nil, err
	}
	return a.listWindows(&cur)
}

// currentDesktop reads the active desktop number from `wmctrl -d`,
// whose second column carries '*' for the current desktop.
func currentDesktop() (int, error) {
	out, err := exec.Command("wmctrl", "-d").Output()
	if err != nil {
		return 0, fmt.Errorf("wmctrl -d: %w", err)
	}
	return parseCurrentDesktop(string(out))
}

// parseCurrentDesktop picks the '*'-marked record out of `wmctrl -d`
// output (lines look like "0 - DG: 1280x1024  VP: 0,0  WA: ... name").
func parseCurrentDesktop(out string) (int, error) {
	scanner := bufio.NewScanner(strings.NewReader(out))
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) >= 2 && fields[1] == "*" {
			n, err := strconv.Atoi(fields[0])
			if err != nil {
				break
			}
			return n, nil
		}
	}
	return 0, fmt.Errorf("wmctrl -d: no current desktop marked in %q", out)
}

// parseWindowLine parses one `wmctrl -lpGx` record into a Window. ok
// is false for malformed lines and — when current is non-nil — for
// windows outside the current desktop (sticky -1 windows stay).
func parseWindowLine(line string, current *int) (core.Window, bool) {
	fields := strings.Fields(line)
	// Columns: 0=id, 1=desktop, 2=pid, 3..6=geometry, 7=WM_CLASS,
	// 8=client machine, 9+=title.
	if len(fields) < 10 {
		return core.Window{}, false
	}
	pid, _ := strconv.Atoi(fields[2])
	x, _ := strconv.Atoi(fields[3])
	y, _ := strconv.Atoi(fields[4])
	w, _ := strconv.Atoi(fields[5])
	h, _ := strconv.Atoi(fields[6])
	// The desktop column, previously dropped, is the desktop-scope
	// filter's input. -1 means "all desktops" (sticky) — always kept.
	desktop, _ := strconv.Atoi(fields[1])
	if current != nil && desktop != *current && desktop != -1 {
		return core.Window{}, false
	}
	title := strings.Join(fields[9:], " ")
	return core.Window{
		ID:     fields[0],
		Title:  title,
		App:    appClass(fields[7]),
		PID:    pid,
		Bounds: core.Rect{X: x, Y: y, W: w, H: h},
	}, true
}

// appClass extracts the class component of wmctrl's WM_CLASS column
// ("instance.class"). The class is the application identity every
// other adapter reports as App (KWin resourceClass, GNOME wm_class),
// and what App-based matching expects. No dot → the whole column.
func appClass(s string) string {
	if i := strings.Index(s, "."); i >= 0 {
		return s[i+1:]
	}
	return s
}

func (a *Adapter) ListMonitors() ([]core.Monitor, error) {
	out, err := exec.Command("xrandr", "--listmonitors").Output()
	if err != nil {
		return nil, fmt.Errorf("xrandr --listmonitors: %w", err)
	}
	var ms []core.Monitor
	scanner := bufio.NewScanner(strings.NewReader(string(out)))
	id := 0
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if !strings.Contains(line, "/") || !strings.Contains(line, "+") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 3 {
			continue
		}
		geom := fields[2]
		parts := strings.Split(geom, "+")
		if len(parts) != 3 {
			continue
		}
		dim := strings.Split(parts[0], "x")
		if len(dim) != 2 {
			continue
		}
		w, _ := strconv.Atoi(stripUnit(dim[0]))
		h, _ := strconv.Atoi(stripUnit(dim[1]))
		x, _ := strconv.Atoi(parts[1])
		y, _ := strconv.Atoi(parts[2])
		ms = append(ms, core.Monitor{
			ID:      id,
			X:       x,
			Y:       y,
			Width:   w,
			Height:  h,
			Primary: strings.HasPrefix(fields[1], "+*"),
		})
		id++
	}
	return ms, scanner.Err()
}

func (a *Adapter) Move(id string, b core.Rect) error {
	arg := fmt.Sprintf("0,%d,%d,%d,%d", b.X, b.Y, b.W, b.H)
	if err := exec.Command("wmctrl", "-i", "-r", id, "-e", arg).Run(); err != nil {
		return fmt.Errorf("wmctrl move %s %s: %w", id, arg, err)
	}
	return nil
}

func (a *Adapter) Focus(id string) error {
	if err := exec.Command("wmctrl", "-i", "-a", id).Run(); err != nil {
		return fmt.Errorf("wmctrl focus %s: %w", id, err)
	}
	return nil
}

// RequestAccessibility is a no-op on Linux. X11 / wmctrl do not require
// a per-process trust grant comparable to macOS Accessibility — the
// permission model is the display-server connection itself, which
// either works or fails on the underlying calls. The CLI surface keeps
// the method on the interface so `windowctl permissions` remains
// available cross-platform; here it just returns nil and the CLI
// prints a "not required" message based on runtime.GOOS.
func (a *Adapter) RequestAccessibility() error { return nil }

// CheckAccessibility is a no-op on Linux for the same reason
// RequestAccessibility is — there's no macOS-style per-process AX
// trust gate on X11. Returns true so callers that branch on the result
// see the "no setup required" answer. The CLI's `permissions --status`
// path on linux ignores this value and prints a platform-derived
// "not required on linux" message regardless.
func (a *Adapter) CheckAccessibility() bool { return true }

func stripUnit(s string) string {
	for i, r := range s {
		if r < '0' || r > '9' {
			return s[:i]
		}
	}
	return s
}
