//go:build linux

package sway

import (
	"image"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/muthuishere/windowctl/internal/core"
)

// The integration tests exercise the backend's full command pipeline
// (query → parse → command → verdict) with a fake swaymsg on PATH that
// answers the recorded get_tree/get_outputs payloads and logs every
// command it receives. No sway, no compositor, no sockets — the same
// tests run in CI.

// setupFakeSwaymsg installs a stub swaymsg that answers queries from
// fixture files in a temp dir and appends every command line to
// commands.log. Returns the temp dir path.
func setupFakeSwaymsg(t *testing.T, getTree, getOutputs string) string {
	t.Helper()
	dir := t.TempDir()
	write := func(name, content string) {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}
	write("get_tree.json", getTree)
	write("get_outputs.json", getOutputs)

	binDir := t.TempDir()
	stub := "#!/bin/bash\n" +
		// The hijacked PATH only holds the stubs; queries and logging
		// need coreutils, so restore the system paths first.
		`export PATH="/usr/bin:/bin:$PATH"
case "$1 $2" in
  "-t get_tree") cat "$WCTL_FAKE_SWAY_DIR/get_tree.json" ;;
  "-t get_outputs") cat "$WCTL_FAKE_SWAY_DIR/get_outputs.json" ;;
  *) echo '[{"success":true}]'; echo "$*" >> "$WCTL_FAKE_SWAY_DIR/commands.log" ;;
esac
`
	if err := os.WriteFile(filepath.Join(binDir, "swaymsg"), []byte(stub), 0o755); err != nil {
		t.Fatalf("write stub: %v", err)
	}
	// grim is invoked as `grim -g X,Y WxH outPath` → the output path
	// is $3; the stub hands out the fixture PNG regardless of the
	// geometry (the scale simulation lives in the fixture's size).
	if err := os.WriteFile(filepath.Join(binDir, "grim"), []byte("#!/bin/bash\nexport PATH=\"/usr/bin:/bin:$PATH\"\ncp \"$WCTL_FAKE_GRIM_PNG\" \"$3\"\n"), 0o755); err != nil {
		t.Fatalf("write grim stub: %v", err)
	}
	t.Setenv("PATH", binDir)
	t.Setenv("WCTL_FAKE_SWAY_DIR", dir)
	return dir
}

// commandsLog returns the swaymsg invocations recorded so far.
func commandsLog(t *testing.T, dir string) []string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(dir, "commands.log"))
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		t.Fatalf("read commands.log: %v", err)
	}
	var out []string
	for _, line := range strings.Split(strings.TrimSpace(string(raw)), "\n") {
		if line != "" {
			out = append(out, line)
		}
	}
	return out
}

// TestIntegrationBackendListWindows runs the real ListWindows against
// the fake swaymsg: focus-recency order (the stampFocused contract),
// app fallback, chrome exclusion.
func TestIntegrationBackendListWindows(t *testing.T) {
	setupFakeSwaymsg(t, treeFixture, outputsFixture)
	b := New()
	ws, err := b.ListWindows()
	if err != nil {
		t.Fatalf("ListWindows: %v", err)
	}
	var ids []string
	for _, w := range ws {
		ids = append(ids, w.ID)
	}
	if got, want := strings.Join(ids, ","), "16,18,17,7"; got != want {
		t.Fatalf("order = %v, want %v (frontmost first)", got, want)
	}
	if ws[0].App != "foot" {
		t.Errorf("ws[0].App = %q, want foot", ws[0].App)
	}
	if ws[3].App != "XClock" {
		t.Errorf("xwayland App = %q, want XClock", ws[3].App)
	}
}

// TestIntegrationBackendListMonitors runs the real ListMonitors: the
// single active output, no primary flag → the focused output (which
// contains the focused window's centroid) is promoted.
func TestIntegrationBackendListMonitors(t *testing.T) {
	setupFakeSwaymsg(t, treeFixture, outputsFixture)
	b := New()
	ms, err := b.ListMonitors()
	if err != nil {
		t.Fatalf("ListMonitors: %v", err)
	}
	if len(ms) != 1 {
		t.Fatalf("got %d monitors, want 1", len(ms))
	}
	m := ms[0]
	if m.Width != 2336 || m.Height != 1261 {
		t.Errorf("monitor box = %+v, want 2336x1261", m)
	}
	if !m.Primary || !m.Focused || m.Active {
		t.Errorf("Primary/Focused/Active = %v/%v/%v, want true/true/false (no cursor query on sway)",
			m.Primary, m.Focused, m.Active)
	}
}

// TestIntegrationBackendFocusAndMove pins the command spellings the
// real swaymsg receives: criteria commands with verdict checking, the
// float-enable path for tiled windows, and the deco compensation.
func TestIntegrationBackendFocusAndMove(t *testing.T) {
	dir := setupFakeSwaymsg(t, treeFixture, outputsFixture)
	b := New()

	if err := b.Focus("7"); err != nil {
		t.Fatalf("Focus: %v", err)
	}
	log := commandsLog(t, dir)
	if len(log) != 1 || !strings.Contains(log[0], "[con_id=7]") || !strings.Contains(log[0], "focus") {
		t.Fatalf("Focus command = %v, want [con_id=7] focus", log)
	}

	// Node 17 is tiled: Move must float it first, then resize with the
	// deco compensation (H + deco_h) and move with (Y - deco_h). Only
	// command branches land in the log (queries log nothing); reset it
	// so the Focus call above doesn't pollute the count.
	if err := os.Remove(filepath.Join(dir, "commands.log")); err != nil && !os.IsNotExist(err) {
		t.Fatalf("reset log: %v", err)
	}
	if err := b.Move("17", core.Rect{X: 100, Y: 200, W: 300, H: 400}); err != nil {
		t.Fatalf("Move: %v", err)
	}
	log = commandsLog(t, dir)
	if len(log) != 3 {
		t.Fatalf("Move issued %d commands, want 3 (float enable, resize, move): %v", len(log), log)
	}
	if !strings.Contains(log[0], "floating enable") {
		t.Errorf("first command = %q, want floating enable", log[0])
	}
	// deco_rect.height of node 17 is 22.
	if !strings.Contains(log[1], "resize set width 300 px height 422 px") {
		t.Errorf("resize = %q, want height 400+22 compensation", log[1])
	}
	if !strings.Contains(log[2], "move absolute position 100 px 178 px") {
		t.Errorf("move = %q, want Y 200-22 compensation", log[2])
	}
}

// TestIntegrationBackendMoveCursorAndCapture pins the exact cursor
// channel (`seat seat0 cursor set`) and the grim capture pipeline
// (scale-2 device pixels resampled back to point size).
func TestIntegrationBackendMoveCursorAndCapture(t *testing.T) {
	dir := setupFakeSwaymsg(t, treeFixture, outputsFixture)
	b := New()

	if err := b.MoveCursor(799, 698); err != nil {
		t.Fatalf("MoveCursor: %v", err)
	}
	log := commandsLog(t, dir)
	if len(log) != 1 || !strings.Contains(log[0], "seat0") ||
		!strings.Contains(log[0], "cursor set 799 698") {
		t.Fatalf("MoveCursor command = %v, want seat cursor set 799 698", log)
	}

	// CaptureRect: the fake grim hands out a 20×20 PNG for a 10×10
	// request (scale 2) → resampled to exactly 10×10.
	srcPath := filepath.Join(dir, "in.png")
	f, err := os.Create(srcPath)
	if err != nil {
		t.Fatalf("create fixture png: %v", err)
	}
	img := image.NewRGBA(image.Rect(0, 0, 20, 20))
	if err := png.Encode(f, img); err != nil {
		_ = f.Close()
		t.Fatalf("encode fixture png: %v", err)
	}
	if err := f.Close(); err != nil {
		t.Fatalf("close fixture png: %v", err)
	}
	t.Setenv("WCTL_FAKE_GRIM_PNG", srcPath)

	out := filepath.Join(dir, "out.png")
	if err := b.CaptureRect(core.Rect{X: 0, Y: 0, W: 10, H: 10}, out); err != nil {
		t.Fatalf("CaptureRect: %v", err)
	}
	got, err := os.Open(out)
	if err != nil {
		t.Fatalf("open capture: %v", err)
	}
	defer got.Close()
	cfg, err := png.DecodeConfig(got)
	if err != nil {
		t.Fatalf("decode capture: %v", err)
	}
	if cfg.Width != 10 || cfg.Height != 10 {
		t.Errorf("capture = %dx%d, want 10x10 (1px==1pt broken)", cfg.Width, cfg.Height)
	}
}

// TestIntegrationBackendCommandFailure pins the verdict-to-error path:
// a failed sway command surfaces the CLI's own error text.
func TestIntegrationBackendCommandFailure(t *testing.T) {
	dir := t.TempDir()
	write := func(name, content string) {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}
	write("get_tree.json", treeFixture)
	write("get_outputs.json", outputsFixture)

	binDir := t.TempDir()
	// A stub whose commands always fail with sway's criteria-miss
	// verdict, wrapped exactly as swaymsg prints it.
	stub := `#!/bin/bash
echo '[{"success":false,"parse_error":false,"error":"No matching node."}]'
exit 2
`
	if err := os.WriteFile(filepath.Join(binDir, "swaymsg"), []byte(stub), 0o755); err != nil {
		t.Fatalf("write stub: %v", err)
	}
	t.Setenv("PATH", binDir)
	t.Setenv("WCTL_FAKE_SWAY_DIR", dir)

	b := New()
	err := b.Focus("7")
	if err == nil {
		t.Fatal("Focus should fail on the criteria-miss verdict")
	}
	if !strings.Contains(err.Error(), "window not found") {
		t.Errorf("err = %v, want the window-not-found mapping", err)
	}
}
