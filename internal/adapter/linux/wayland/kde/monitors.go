//go:build linux

package kde

import (
	"encoding/json"
	"fmt"

	"github.com/Jiafei-Queen/windowctl/internal/core"
)

// screenInfo is one output as reported by the monitors script. DPR is
// the output's devicePixelRatio; the geometry is in logical points
// (already scale-adjusted), the same space as window bounds.
type screenInfo struct {
	Name    string
	X       int
	Y       int
	Width   int
	Height  int
	DPR     float64
	Primary bool
	Focused bool
	Active  bool
}

func (b *Backend) ListMonitors() ([]core.Monitor, error) {
	screens, err := b.screens()
	if err != nil {
		return nil, err
	}
	ms := make([]core.Monitor, 0, len(screens))
	for i, s := range screens {
		ms = append(ms, core.Monitor{
			// The public layer re-sorts by origin and re-indexes
			// 1..N (see sortMonitors); these IDs just need to be
			// stable and unique within one call, matching the x11
			// adapter's sequential convention.
			ID:      i + 1,
			X:       s.X,
			Y:       s.Y,
			Width:   s.Width,
			Height:  s.Height,
			Primary: s.Primary,
			Focused: s.Focused,
			Active:  s.Active,
		})
	}
	return ms, nil
}

// screens runs the monitors script and returns the raw per-output data.
// CaptureRect reuses it for the HiDPI scale factor.
func (b *Backend) screens() ([]screenInfo, error) {
	payload, err := b.runScript(monitorsBody)
	if err != nil {
		return nil, err
	}
	var rep monitorReport
	if err := json.Unmarshal([]byte(payload), &rep); err != nil {
		return nil, fmt.Errorf("kde: parse monitor report: %w", err)
	}
	if !rep.OK {
		return nil, fmt.Errorf("kde: list monitors: %s", rep.Error)
	}
	out := make([]screenInfo, 0, len(rep.Monitors))
	for _, m := range rep.Monitors {
		out = append(out, screenInfo{
			Name:    m.Name,
			X:       m.X,
			Y:       m.Y,
			Width:   m.Width,
			Height:  m.Height,
			DPR:     m.DPR,
			Primary: m.Primary,
			Focused: m.Focused,
			Active:  m.Active,
		})
	}
	return out, nil
}

func (b *Backend) CursorPosition() (int, int, error) {
	payload, err := b.runScript(cursorBody)
	if err != nil {
		return 0, 0, err
	}
	var rep cursorReport
	if err := json.Unmarshal([]byte(payload), &rep); err != nil {
		return 0, 0, fmt.Errorf("kde: parse cursor report: %w", err)
	}
	if !rep.OK {
		return 0, 0, fmt.Errorf("kde: cursor position: %s", rep.Error)
	}
	return rep.X, rep.Y, nil
}
