//go:build linux

package gnome

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/Jiafei-Queen/windowctl/internal/core"
)

// Reply shapes mirror the KDE backend's script reports: every
// extension method answers {"ok":true,...} or {"ok":false,"error"}.

type report struct {
	OK    bool   `json:"ok"`
	Error string `json:"error"`
}

type windowEntry struct {
	ID     string `json:"id"`
	Title  string `json:"title"`
	App    string `json:"app"`
	PID    int    `json:"pid"`
	X      int    `json:"x"`
	Y      int    `json:"y"`
	Width  int    `json:"width"`
	Height int    `json:"height"`
	// WorkspaceIndex is the window's workspace index. A pointer so an
	// extension predating the field decodes as nil (reported -1 is a
	// legitimate "not yet placed" value, distinct from absent).
	WorkspaceIndex *int `json:"workspaceIndex"`
	// OnAllWorkspaces marks pinned windows, which always count as
	// current-desktop. OnAllWorkspaces/Minimized default to false for
	// old extensions, matching their semantics.
	OnAllWorkspaces bool `json:"onAllWorkspaces"`
	Minimized       bool `json:"minimized"`
}

type monitorEntry struct {
	X       int     `json:"x"`
	Y       int     `json:"y"`
	Width   int     `json:"width"`
	Height  int     `json:"height"`
	Scale   float64 `json:"scale"`
	Primary bool    `json:"primary"`
	Focused bool    `json:"focused"`
	Active  bool    `json:"active"`
}

type cursorEntry struct {
	X int `json:"x"`
	Y int `json:"y"`
}

type scaleEntry struct {
	Scale float64 `json:"scale"`
}

// listWindowEntries decodes the raw ListWindows reply, including the
// desktop metadata the plain parseWindows discards. The second return
// is the extension-reported active workspace index (nil when the
// running extension predates the field).
func listWindowEntries(payload string) ([]windowEntry, *int, error) {
	var rep struct {
		report
		Windows               []windowEntry `json:"windows"`
		CurrentWorkspaceIndex *int          `json:"currentWorkspaceIndex"`
	}
	if err := json.Unmarshal([]byte(payload), &rep); err != nil {
		return nil, nil, fmt.Errorf("gnome: parse window report: %w", err)
	}
	if !rep.OK {
		return nil, nil, fmt.Errorf("gnome: list windows: %s", rep.Error)
	}
	return rep.Windows, rep.CurrentWorkspaceIndex, nil
}

// parseWindows decodes a ListWindows reply into core.Windows.
func parseWindows(payload string) ([]core.Window, error) {
	entries, _, err := listWindowEntries(payload)
	if err != nil {
		return nil, err
	}
	return windowsFromEntries(entries), nil
}

// windowsFromEntries maps extension entries to core.Windows. Kept
// separate from parsing so the filtered path shares the mapping
// (mirroring hyprland's windowsFromClients pattern).
func windowsFromEntries(entries []windowEntry) []core.Window {
	ws := make([]core.Window, 0, len(entries))
	for _, w := range entries {
		ws = append(ws, core.Window{
			ID:     w.ID,
			Title:  w.Title,
			App:    w.App,
			PID:    w.PID,
			Bounds: core.Rect{X: w.X, Y: w.Y, W: w.Width, H: w.Height},
		})
	}
	return ws
}

// parseMonitors decodes a ListMonitors reply into core.Monitors.
func parseMonitors(payload string) ([]core.Monitor, error) {
	var rep struct {
		report
		Monitors []monitorEntry `json:"monitors"`
	}
	if err := json.Unmarshal([]byte(payload), &rep); err != nil {
		return nil, fmt.Errorf("gnome: parse monitor report: %w", err)
	}
	if !rep.OK {
		return nil, fmt.Errorf("gnome: list monitors: %s", rep.Error)
	}
	ms := make([]core.Monitor, 0, len(rep.Monitors))
	for i, m := range rep.Monitors {
		ms = append(ms, core.Monitor{
			// Same sequential-ID convention as the kde backend:
			// the public layer re-sorts and re-indexes 1..N.
			ID:      i + 1,
			X:       m.X,
			Y:       m.Y,
			Width:   m.Width,
			Height:  m.Height,
			Primary: m.Primary,
			Focused: m.Focused,
			Active:  m.Active,
		})
	}
	return ms, nil
}

// parseScale decodes a Screenshot reply's scale factor (the factor the
// extension itself applied when capturing), defaulting to 1.
func parseScale(payload string) (float64, error) {
	var rep struct {
		report
		scaleEntry
	}
	if err := json.Unmarshal([]byte(payload), &rep); err != nil {
		return 0, fmt.Errorf("gnome: parse screenshot report: %w", err)
	}
	if !rep.OK {
		return 0, fmt.Errorf("gnome: screenshot: %s", rep.Error)
	}
	if rep.Scale <= 0 {
		return 1, nil
	}
	return rep.Scale, nil
}

func (b *Backend) ListWindows() ([]core.Window, error) {
	payload, err := b.call("ListWindows")
	if err != nil {
		return nil, err
	}
	return parseWindows(payload)
}

// ListWindowsFiltered implements the optional desktop-scope capability
// (core.FilteredWindowLister). Only CurrentDesktopOnly / SkipMinimized
// are honored here — Title / App remain the public layer's job.
//
// Backward compatibility: when the running extension predates the
// workspace fields (no currentWorkspaceIndex in the reply), the
// desktop filter cannot be honored and degrades to returning
// everything — the same best-effort contract the Filter docs promise.
// ensureInstalled copies the newer embedded sources over the old ones,
// so the degrade window is one session (see install.go).
func (b *Backend) ListWindowsFiltered(filter core.Filter) ([]core.Window, error) {
	payload, err := b.call("ListWindows")
	if err != nil {
		return nil, err
	}
	entries, current, err := listWindowEntries(payload)
	if err != nil {
		return nil, err
	}
	if filter.CurrentDesktopOnly {
		entries = keepCurrentWorkspace(entries, current)
	}
	if filter.SkipMinimized {
		entries = dropMinimized(entries)
	}
	return windowsFromEntries(entries), nil
}

// keepCurrentWorkspace keeps sticky (on-all-workspaces) windows and
// windows whose workspace index matches the extension-reported active
// one. "Unknown" entries — per-window index absent, current absent —
// are kept: the desktop scope is best-effort, never a hard failure.
func keepCurrentWorkspace(entries []windowEntry, current *int) []windowEntry {
	if current == nil {
		return entries
	}
	kept := make([]windowEntry, 0, len(entries))
	for _, w := range entries {
		if w.OnAllWorkspaces || w.WorkspaceIndex == nil || *w.WorkspaceIndex == *current {
			kept = append(kept, w)
		}
	}
	return kept
}

// dropMinimized drops entries the extension flagged minimized.
func dropMinimized(entries []windowEntry) []windowEntry {
	kept := make([]windowEntry, 0, len(entries))
	for _, w := range entries {
		if !w.Minimized {
			kept = append(kept, w)
		}
	}
	return kept
}

func (b *Backend) ListMonitors() ([]core.Monitor, error) {
	payload, err := b.call("ListMonitors")
	if err != nil {
		return nil, err
	}
	return parseMonitors(payload)
}

func (b *Backend) Move(windowID string, bounds core.Rect) error {
	payload, err := b.call("Move", windowID,
		int32(bounds.X), int32(bounds.Y), int32(bounds.W), int32(bounds.H))
	if err != nil {
		return err
	}
	var rep report
	if err := json.Unmarshal([]byte(payload), &rep); err != nil {
		return fmt.Errorf("gnome: parse move report: %w", err)
	}
	if !rep.OK {
		if isNotFound(rep.Error) {
			return fmt.Errorf("gnome: window %s not found (may have been closed): %w",
				windowID, core.ErrNoMatch)
		}
		return fmt.Errorf("gnome: move: %s", rep.Error)
	}
	return nil
}

func (b *Backend) Focus(windowID string) error {
	payload, err := b.call("Focus", windowID)
	if err != nil {
		return err
	}
	var rep report
	if err := json.Unmarshal([]byte(payload), &rep); err != nil {
		return fmt.Errorf("gnome: parse focus report: %w", err)
	}
	if !rep.OK {
		if isNotFound(rep.Error) {
			return fmt.Errorf("gnome: window %s not found (may have been closed): %w",
				windowID, core.ErrNoMatch)
		}
		return fmt.Errorf("gnome: focus: %s", rep.Error)
	}
	return nil
}

func (b *Backend) CursorPosition() (int, int, error) {
	payload, err := b.call("CursorPos")
	if err != nil {
		return 0, 0, err
	}
	var rep struct {
		report
		cursorEntry
	}
	if err := json.Unmarshal([]byte(payload), &rep); err != nil {
		return 0, 0, fmt.Errorf("gnome: parse cursor report: %w", err)
	}
	if !rep.OK {
		return 0, 0, fmt.Errorf("gnome: cursor position: %s", rep.Error)
	}
	return rep.X, rep.Y, nil
}

// isNotFound recognizes the extension's "window not found" message so
// the adapter can return core.ErrNoMatch, matching the darwin and kde
// contracts for windows that vanished between list and act.
func isNotFound(msg string) bool {
	return strings.HasPrefix(msg, "window not found")
}
