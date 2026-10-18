//go:build linux

package hyprland

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Jiafei-Queen/windowctl/internal/core"
)

// The integration tests exercise the backend's full pipeline (query →
// parse → command → verdict) with a fake hyprctl on PATH that answers
// the recorded clients/monitors payloads and logs every dispatch.
// No Hyprland, no sockets — CI-safe.

// setupFakeHyprctl installs a stub hyprctl answering queries from
// fixture files and logging dispatches. Returns the temp dir.
func setupFakeHyprctl(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	write := func(name, content string) {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}
	write("clients.json", clientsFixture)
	write("monitors.json", monitorsFixture)
	write("activewindow.json", activeWindowFixture)

	binDir := t.TempDir()
	stub := "#!/bin/bash\n" +
		// The hijacked PATH only holds the stub; restore coreutils.
		`export PATH="/usr/bin:/bin:$PATH"
case "$1 $2" in
  "-j clients") cat "$WCTL_FAKE_HYPR_DIR/clients.json" ;;
  "-j monitors") cat "$WCTL_FAKE_HYPR_DIR/monitors.json" ;;
  "-j activewindow") cat "$WCTL_FAKE_HYPR_DIR/activewindow.json" ;;
  "cursorpos ") echo "300, 200" ;;
  "dispatch "*) echo "ok" ;;
  *) echo "unknown: $*" >&2; exit 1 ;;
esac
case "$1" in
  dispatch) echo "$*" >> "$WCTL_FAKE_HYPR_DIR/dispatches.log" ;;
esac
`
	if err := os.WriteFile(filepath.Join(binDir, "hyprctl"), []byte(stub), 0o755); err != nil {
		t.Fatalf("write stub: %v", err)
	}
	t.Setenv("PATH", binDir)
	t.Setenv("WCTL_FAKE_HYPR_DIR", dir)
	return dir
}

// activeWindowFixture mirrors `hyprctl -j activewindow` on the VM: the
// focused client is the foot terminal (its centroid sits on monitor 0).
const activeWindowFixture = `{
  "address": "0x5631bd3f6d30", "class": "foot", "title": "jiafei114@debian:~",
  "pid": 14307, "xwayland": false,
  "at": [21, 21], "size": [1126, 589],
  "floating": false, "mapped": true, "monitor": 0,
  "workspace": {"id": 1, "name": "1"}
}`

// dispatchesLog returns the recorded dispatch lines.
func dispatchesLog(t *testing.T, dir string) []string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(dir, "dispatches.log"))
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		t.Fatalf("read dispatches.log: %v", err)
	}
	var out []string
	for _, line := range strings.Split(strings.TrimSpace(string(raw)), "\n") {
		if line != "" {
			out = append(out, line)
		}
	}
	return out
}

func TestIntegrationBackendListWindows(t *testing.T) {
	setupFakeHyprctl(t)
	b := New()
	ws, err := b.ListWindows()
	if err != nil {
		t.Fatalf("ListWindows: %v", err)
	}
	if len(ws) != 2 {
		t.Fatalf("got %d windows, want 2 (hidden client filtered)", len(ws))
	}
	if ws[0].ID != "0x5631bd3f6d30" || ws[0].App != "foot" {
		t.Errorf("ws[0] = %+v", ws[0])
	}
}

func TestIntegrationBackendListMonitors(t *testing.T) {
	setupFakeHyprctl(t)
	b := New()
	ms, err := b.ListMonitors()
	if err != nil {
		t.Fatalf("ListMonitors: %v", err)
	}
	if len(ms) != 1 {
		t.Fatalf("got %d monitors, want 1", len(ms))
	}
	m := ms[0]
	// Logical box: 2336x1261 at scale 2 → 1168x630.
	if m.Width != 1168 || m.Height != 630 {
		t.Errorf("monitor box = %dx%d, want 1168x630", m.Width, m.Height)
	}
	// cursorpos (300,200) → monitor 0 active; focused foot client's
	// centroid (584,315) → monitor 0 focused; focused promoted to
	// primary (no primary flag on 0.55.2).
	if !m.Active || !m.Focused || !m.Primary {
		t.Errorf("Active/Focused/Primary = %v/%v/%v, want true/true/true", m.Active, m.Focused, m.Primary)
	}
}

// TestIntegrationBackendFocusAndMove pins the dispatch spellings: the
// address target, the float-first policy for tiled windows, and the
// resize-before-move ordering.
func TestIntegrationBackendFocusAndMove(t *testing.T) {
	dir := setupFakeHyprctl(t)
	b := New()

	if err := b.Focus("0x5631bd3f6d30"); err != nil {
		t.Fatalf("Focus: %v", err)
	}
	log := dispatchesLog(t, dir)
	if len(log) != 1 || !strings.Contains(log[0], "focuswindow") ||
		!strings.Contains(log[0], "address:0x5631bd3f6d30") {
		t.Fatalf("Focus dispatch = %v, want focuswindow address:0x5631bd3f6d30", log)
	}

	if err := os.Remove(filepath.Join(dir, "dispatches.log")); err != nil && !os.IsNotExist(err) {
		t.Fatalf("reset log: %v", err)
	}

	// The foot client is tiled (floating=false) → togglefloating
	// first; resize recenters, so it must run before the final move.
	if err := b.Move("0x5631bd3f6d30", core.Rect{X: 100, Y: 200, W: 300, H: 400}); err != nil {
		t.Fatalf("Move: %v", err)
	}
	log = dispatchesLog(t, dir)
	if len(log) != 3 {
		t.Fatalf("Move issued %d dispatches, want 3: %v", len(log), log)
	}
	if !strings.Contains(log[0], "togglefloating address:0x5631bd3f6d30") {
		t.Errorf("first dispatch = %q, want togglefloating", log[0])
	}
	if !strings.Contains(log[1], "resizewindowpixel exact 300 400,address:0x5631bd3f6d30") {
		t.Errorf("resize = %q", log[1])
	}
	if !strings.Contains(log[2], "movewindowpixel exact 100 200,address:0x5631bd3f6d30") {
		t.Errorf("move = %q", log[2])
	}
}
