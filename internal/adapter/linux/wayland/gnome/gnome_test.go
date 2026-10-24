//go:build linux

package gnome

import (
	"testing"

	"github.com/Jiafei-Queen/windowctl/internal/core"
)

func TestParseWindows(t *testing.T) {
	payload := `{"ok":true,"windows":[
		{"id":"3","title":"计算器","app":"org.gnome.Calculator","pid":9275,
		 "x":100,"y":100,"width":360,"height":616},
		{"id":"7","title":"Text Editor","app":"org.gnome.TextEditor","pid":42,
		 "x":0,"y":0,"width":100,"height":50}]}`

	ws, err := parseWindows(payload)
	if err != nil {
		t.Fatalf("parseWindows: %v", err)
	}
	if len(ws) != 2 {
		t.Fatalf("got %d windows, want 2", len(ws))
	}
	want := core.Window{
		ID: "3", Title: "计算器", App: "org.gnome.Calculator", PID: 9275,
		Bounds: core.Rect{X: 100, Y: 100, W: 360, H: 616},
	}
	if ws[0] != want {
		t.Errorf("window[0] = %+v, want %+v", ws[0], want)
	}
}

func TestParseWindowsError(t *testing.T) {
	if _, err := parseWindows(`{"ok":false,"error":"boom"}`); err == nil {
		t.Error("expected error for ok=false report")
	}
	if _, err := parseWindows(`not json`); err == nil {
		t.Error("expected error for invalid json")
	}
	// An ok=true report with no windows is an empty desktop, not an
	// error — and must be a non-nil empty slice.
	ws, err := parseWindows(`{"ok":true}`)
	if err != nil {
		t.Fatalf("empty report: %v", err)
	}
	if ws == nil || len(ws) != 0 {
		t.Errorf("empty report → %v, want non-nil empty slice", ws)
	}
}

// TestParseWindowsMissingFields pins the zero-value tolerance: entries
// with absent fields decode as 0/"" rather than failing (the
// extension always sends full records; this guards against a
// partial/unexpected shape turning a whole listing into an error).
func TestParseWindowsMissingFields(t *testing.T) {
	ws, err := parseWindows(`{"ok":true,"windows":[{"id":"9"}]}`)
	if err != nil {
		t.Fatalf("parseWindows: %v", err)
	}
	want := core.Window{ID: "9"}
	if ws[0] != want {
		t.Errorf("window = %+v, want %+v", ws[0], want)
	}
}

func TestParseMonitors(t *testing.T) {
	payload := `{"ok":true,"monitors":[
		{"x":0,"y":0,"width":2336,"height":1261,"scale":1.25,
		 "primary":true,"focused":true,"active":false},
		{"x":2336,"y":0,"width":1920,"height":1080,"scale":1,
		 "primary":false,"focused":false,"active":true}]}`

	ms, err := parseMonitors(payload)
	if err != nil {
		t.Fatalf("parseMonitors: %v", err)
	}
	if len(ms) != 2 {
		t.Fatalf("got %d monitors, want 2", len(ms))
	}
	want := core.Monitor{
		ID: 1, X: 0, Y: 0, Width: 2336, Height: 1261,
		Primary: true, Focused: true, Active: false,
	}
	if ms[0] != want {
		t.Errorf("monitor[0] = %+v, want %+v", ms[0], want)
	}
	if ms[1].Active != true {
		t.Errorf("monitor[1].Active = %v, want true", ms[1].Active)
	}
}

func TestParseScale(t *testing.T) {
	for _, tc := range []struct {
		payload string
		want    float64
		wantErr bool
	}{
		{`{"ok":true,"scale":2}`, 2, false},
		{`{"ok":true,"scale":1.25}`, 1.25, false},
		// Missing/zero scale falls back to 1 rather than failing.
		{`{"ok":true}`, 1, false},
		{`{"ok":false,"error":"denied"}`, 0, true},
	} {
		got, err := parseScale(tc.payload)
		if (err != nil) != tc.wantErr {
			t.Errorf("parseScale(%s) err = %v, wantErr %v", tc.payload, err, tc.wantErr)
			continue
		}
		if err == nil && got != tc.want {
			t.Errorf("parseScale(%s) = %v, want %v", tc.payload, got, tc.want)
		}
	}
}

func TestIsNotFound(t *testing.T) {
	cases := map[string]bool{
		"window not found: 3": true,
		"window not found: 9": true,
		"":                    false,
		"some other failure":  false,
	}
	for msg, want := range cases {
		if got := isNotFound(msg); got != want {
			t.Errorf("isNotFound(%q) = %v, want %v", msg, got, want)
		}
	}
}

// TestKeepCurrentWorkspace covers the desktop-scope predicate: the
// active workspace's windows are kept, other workspaces' dropped, and
// sticky / unknown entries always kept.
func TestKeepCurrentWorkspace(t *testing.T) {
	one, three := 1, 3
	entries := []windowEntry{
		{ID: "1", WorkspaceIndex: &one, OnAllWorkspaces: false},
		{ID: "2", WorkspaceIndex: &three, OnAllWorkspaces: false},
		{ID: "3", WorkspaceIndex: &three, OnAllWorkspaces: true},
		{ID: "4", WorkspaceIndex: &one, OnAllWorkspaces: true},
		{ID: "5", WorkspaceIndex: nil},
	}
	// current=1: keep 1 (match), 3 (sticky), 4 (sticky), 5 (unknown).
	got := keepCurrentWorkspace(entries, &one)
	if len(got) != 4 || got[0].ID != "1" || got[1].ID != "3" || got[2].ID != "4" || got[3].ID != "5" {
		t.Errorf("keepCurrentWorkspace(current=1) = %v, want [1 3 4 5]", ids(got))
	}
	// nil current (old extension) → unchanged, full list.
	got = keepCurrentWorkspace(entries, nil)
	if len(got) != 5 {
		t.Errorf("keepCurrentWorkspace(nil) kept %d of 5 entries, want 5", len(got))
	}
}

// TestDropMinimized covers SkipMinimized: flagged entries dropped,
// everything else kept in order.
func TestDropMinimized(t *testing.T) {
	entries := []windowEntry{
		{ID: "1", Minimized: true},
		{ID: "2"},
		{ID: "3", Minimized: true},
	}
	got := dropMinimized(entries)
	if len(got) != 1 || got[0].ID != "2" {
		t.Errorf("dropMinimized = %v, want [2]", ids(got))
	}
}

func ids(entries []windowEntry) []string {
	out := make([]string, 0, len(entries))
	for _, e := range entries {
		out = append(out, e.ID)
	}
	return out
}

// TestListWindowEntriesWorkspaceMetadata pins the desktop-metadata
// contract: currentWorkspaceIndex and per-window fields decode, and
// absent fields (old extension) decode as nil/false.
func TestListWindowEntriesWorkspaceMetadata(t *testing.T) {
	payload := `{"ok":true,"currentWorkspaceIndex":2,"windows":[
		{"id":"9","workspaceIndex":2,"onAllWorkspaces":true,"minimized":false},
		{"id":"8","workspaceIndex":1,"onAllWorkspaces":false,"minimized":true}]}`
	entries, current, err := listWindowEntries(payload)
	if err != nil {
		t.Fatalf("listWindowEntries: %v", err)
	}
	if current == nil || *current != 2 {
		t.Errorf("current = %v, want 2", current)
	}
	if entries[0].WorkspaceIndex == nil || *entries[0].WorkspaceIndex != 2 || !entries[0].OnAllWorkspaces {
		t.Errorf("entry[0] = %+v, want workspace 2 sticky", entries[0])
	}
	if !entries[1].Minimized || entries[1].OnAllWorkspaces {
		t.Errorf("entry[1] = %+v, want minimized, not sticky", entries[1])
	}
	// Old extension: neither field present.
	entries, current, err = listWindowEntries(`{"ok":true,"windows":[{"id":"9"}]}`)
	if err != nil {
		t.Fatalf("listWindowEntries legacy: %v", err)
	}
	if current != nil || entries[0].WorkspaceIndex != nil || entries[0].Minimized || entries[0].OnAllWorkspaces {
		t.Errorf("legacy decode = %v current=%v, want all-zero workspace metadata", entries, current)
	}
}

func TestParseStringVariant(t *testing.T) {
	cases := []struct {
		in   string
		want []string
	}{
		{"@as []", []string{}},
		{"[]", []string{}},
		{"['windowctl@local']", []string{"windowctl@local"}},
		{"['a@x', 'b@y']", []string{"a@x", "b@y"}},
		{"garbage", nil},
	}
	for _, tc := range cases {
		got := parseStringVariant(tc.in)
		if tc.want == nil {
			if got != nil {
				t.Errorf("parseStringVariant(%q) = %v, want nil", tc.in, got)
			}
			continue
		}
		if len(got) != len(tc.want) {
			t.Errorf("parseStringVariant(%q) = %v, want %v", tc.in, got, tc.want)
			continue
		}
		for i := range got {
			if got[i] != tc.want[i] {
				t.Errorf("parseStringVariant(%q)[%d] = %q, want %q", tc.in, i, got[i], tc.want[i])
			}
		}
	}
}
