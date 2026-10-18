package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"strings"
	"testing"

	windowctl "github.com/Jiafei-Queen/windowctl"
)

func TestPrintWindowsTableHasSpecHeaderColumns(t *testing.T) {
	var buf bytes.Buffer
	printWindowsTable(&buf, []windowctl.Window{
		{ID: "1", Title: "Inbox", App: "Chrome", PID: 42, Monitor: 0, Bounds: windowctl.Rect{X: 0, Y: 0, W: 800, H: 600}},
	})
	out := buf.String()
	for _, col := range []string{"ID", "TITLE", "APP", "PID", "MONITOR", "BOUNDS"} {
		if !strings.Contains(out, col) {
			t.Errorf("expected column %q in table header, got:\n%s", col, out)
		}
	}
	if !strings.Contains(out, "800x600+0+0") {
		t.Errorf("expected bounds %q in row, got:\n%s", "800x600+0+0", out)
	}
}

func TestPrintWindowsJSONIsArrayOfObjects(t *testing.T) {
	var buf bytes.Buffer
	if err := printWindowsJSON(&buf, []windowctl.Window{
		{ID: "1", Title: "Inbox", App: "Chrome"},
	}); err != nil {
		t.Fatal(err)
	}
	var got []map[string]any
	if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
		t.Fatalf("output is not a JSON array: %v\n%s", err, buf.String())
	}
	if len(got) != 1 || got[0]["Title"] != "Inbox" {
		t.Fatalf("unexpected JSON shape: %s", buf.String())
	}
}

func TestPrintMonitorsTableHasSpecHeaderColumns(t *testing.T) {
	var buf bytes.Buffer
	printMonitorsTable(&buf, []windowctl.Monitor{
		{ID: 0, X: 0, Y: 0, Width: 1920, Height: 1080, Primary: true},
	})
	out := buf.String()
	for _, col := range []string{"ID", "X", "Y", "WIDTH", "HEIGHT", "PRIMARY"} {
		if !strings.Contains(out, col) {
			t.Errorf("expected column %q in table header, got:\n%s", col, out)
		}
	}
}

func TestPrintMonitorsJSONIsArrayOfObjects(t *testing.T) {
	var buf bytes.Buffer
	if err := printMonitorsJSON(&buf, []windowctl.Monitor{
		{ID: 0, Width: 1920, Height: 1080, Primary: true},
	}); err != nil {
		t.Fatal(err)
	}
	var got []map[string]any
	if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
		t.Fatalf("output is not a JSON array: %v\n%s", err, buf.String())
	}
	if len(got) != 1 || got[0]["Primary"] != true {
		t.Fatalf("unexpected JSON shape: %s", buf.String())
	}
}

// TestUsageMentionsPermissionsSubcommand pins ac-1: the help / usage
// text MUST advertise the new permissions subcommand alongside
// windows / monitors / move / focus.
func TestUsageMentionsPermissionsSubcommand(t *testing.T) {
	var buf bytes.Buffer
	usage(&buf)
	if !strings.Contains(buf.String(), "windowctl permissions") {
		t.Fatalf("usage text missing 'windowctl permissions':\n%s", buf.String())
	}
}

func TestUsageMentionsInstallSkillsCommand(t *testing.T) {
	var buf bytes.Buffer
	usage(&buf)
	if !strings.Contains(buf.String(), "windowctl install --skills") {
		t.Fatalf("usage text missing 'windowctl install --skills':\n%s", buf.String())
	}
}

// alwaysGranted / alwaysDenied are CheckAccessibility test stubs that
// also record whether they were invoked, so individual tests can pin
// "the --status path called check, not request".
type checkStub struct {
	result bool
	called int
}

func (s *checkStub) fn() bool {
	s.called++
	return s.result
}

type requestStub struct {
	err    error
	called int
}

func (s *requestStub) fn() error {
	s.called++
	return s.err
}

// TestRunPermissionsDarwinGranted exercises the macOS happy path: when
// the adapter's RequestAccessibility returns nil the command prints a
// "granted" line to stdout and exits 0. Per spec slice 11.
func TestRunPermissionsDarwinGranted(t *testing.T) {
	var stdout, stderr bytes.Buffer
	rc := runPermissions(&stdout, &stderr, "darwin", false, false, func() error { return nil }, func() bool { return true })
	if rc != 0 {
		t.Fatalf("expected exit 0 on AX granted, got %d (stderr=%q)", rc, stderr.String())
	}
	if !strings.Contains(stdout.String(), "granted") {
		t.Fatalf("expected stdout to mention 'granted', got %q", stdout.String())
	}
}

// TestRunPermissionsDarwinDenied exercises the macOS unhappy path: when
// the adapter returns ErrAccessibilityDenied the command MUST exit
// non-zero and the stderr message MUST mention System Settings so the
// user has the manual route as a fallback.
func TestRunPermissionsDarwinDenied(t *testing.T) {
	var stdout, stderr bytes.Buffer
	rc := runPermissions(&stdout, &stderr, "darwin", false, false, func() error {
		return windowctl.ErrAccessibilityDenied
	}, func() bool { return false })
	if rc == 0 {
		t.Fatalf("expected non-zero exit on AX denied, got 0 (stdout=%q)", stdout.String())
	}
	if !strings.Contains(stderr.String(), "System Settings") {
		t.Fatalf("expected stderr to mention 'System Settings', got %q", stderr.String())
	}
}

// TestRunPermissionsLinuxIsNoOp pins ac-3: on linux the subcommand
// prints a "not required" line and exits 0. Same for windows below.
func TestRunPermissionsLinuxIsNoOp(t *testing.T) {
	var stdout, stderr bytes.Buffer
	rc := runPermissions(&stdout, &stderr, "linux", false, false, func() error { return nil }, func() bool { return true })
	if rc != 0 {
		t.Fatalf("expected exit 0 on linux no-op, got %d", rc)
	}
	if !strings.Contains(stdout.String(), "not required") || !strings.Contains(stdout.String(), "linux") {
		t.Fatalf("expected stdout to mention 'not required on linux', got %q", stdout.String())
	}
}

func TestRunPermissionsWindowsIsNoOp(t *testing.T) {
	var stdout, stderr bytes.Buffer
	rc := runPermissions(&stdout, &stderr, "windows", false, false, func() error { return nil }, func() bool { return true })
	if rc != 0 {
		t.Fatalf("expected exit 0 on windows no-op, got %d", rc)
	}
	if !strings.Contains(stdout.String(), "not required") || !strings.Contains(stdout.String(), "windows") {
		t.Fatalf("expected stdout to mention 'not required on windows', got %q", stdout.String())
	}
}

// TestRunPermissionsUnexpectedError pins behaviour when the adapter
// returns a non-sentinel error (e.g. an internal CFDictionary failure
// surfaced as a generic error). The CLI MUST exit non-zero and surface
// the error rather than swallow it.
func TestRunPermissionsUnexpectedError(t *testing.T) {
	var stdout, stderr bytes.Buffer
	boom := errors.New("ax check exploded")
	rc := runPermissions(&stdout, &stderr, "darwin", false, false, func() error { return boom }, func() bool { return false })
	if rc == 0 {
		t.Fatalf("expected non-zero exit on unexpected error, got 0")
	}
	if !strings.Contains(stderr.String(), "ax check exploded") {
		t.Fatalf("expected stderr to surface the underlying error, got %q", stderr.String())
	}
}

// TestRunPermissionsStatusDarwinGranted pins the read-only happy path:
// --status on darwin with AX trust granted prints "granted" and exits 0
// WITHOUT invoking RequestAccessibility (the prompting one).
func TestRunPermissionsStatusDarwinGranted(t *testing.T) {
	var stdout, stderr bytes.Buffer
	req := &requestStub{}
	chk := &checkStub{result: true}
	rc := runPermissions(&stdout, &stderr, "darwin", true, false, req.fn, chk.fn)
	if rc != 0 {
		t.Fatalf("expected exit 0 on AX granted, got %d (stderr=%q)", rc, stderr.String())
	}
	if !strings.Contains(stdout.String(), "granted") {
		t.Fatalf("expected stdout to mention 'granted', got %q", stdout.String())
	}
	if req.called != 0 {
		t.Fatalf("--status MUST NOT call RequestAccessibility (prompting); got %d calls", req.called)
	}
	if chk.called != 1 {
		t.Fatalf("--status MUST call CheckAccessibility exactly once; got %d", chk.called)
	}
}

// TestRunPermissionsStatusDarwinDenied pins the read-only unhappy path:
// --status on darwin with AX trust denied prints the actionable denial
// message to stderr (mentioning the manual route) and exits non-zero —
// again without invoking RequestAccessibility.
func TestRunPermissionsStatusDarwinDenied(t *testing.T) {
	var stdout, stderr bytes.Buffer
	req := &requestStub{}
	chk := &checkStub{result: false}
	rc := runPermissions(&stdout, &stderr, "darwin", true, false, req.fn, chk.fn)
	if rc == 0 {
		t.Fatalf("expected non-zero exit on AX denied, got 0 (stdout=%q)", stdout.String())
	}
	if !strings.Contains(stderr.String(), "denied") {
		t.Fatalf("expected stderr to mention 'denied', got %q", stderr.String())
	}
	if !strings.Contains(stderr.String(), "System Settings") {
		t.Fatalf("expected stderr to mention 'System Settings', got %q", stderr.String())
	}
	if !strings.Contains(stderr.String(), "windowctl permissions") {
		t.Fatalf("expected stderr to point at 'windowctl permissions' for remediation, got %q", stderr.String())
	}
	if req.called != 0 {
		t.Fatalf("--status MUST NOT call RequestAccessibility (prompting); got %d calls", req.called)
	}
	if chk.called != 1 {
		t.Fatalf("--status MUST call CheckAccessibility exactly once; got %d", chk.called)
	}
}

// TestRunPermissionsStatusLinuxIsNoOp pins ac-4: on linux --status
// prints a "not required" line, exits 0, and does NOT call either
// adapter method (the message is platform-derived, not adapter-derived).
func TestRunPermissionsStatusLinuxIsNoOp(t *testing.T) {
	var stdout, stderr bytes.Buffer
	req := &requestStub{}
	chk := &checkStub{result: true}
	rc := runPermissions(&stdout, &stderr, "linux", true, false, req.fn, chk.fn)
	if rc != 0 {
		t.Fatalf("expected exit 0 on linux --status, got %d", rc)
	}
	if !strings.Contains(stdout.String(), "not required") || !strings.Contains(stdout.String(), "linux") {
		t.Fatalf("expected stdout to mention 'not required on linux', got %q", stdout.String())
	}
	if req.called != 0 {
		t.Fatalf("--status on linux MUST NOT call RequestAccessibility; got %d calls", req.called)
	}
}

// TestRunPermissionsStatusWindowsIsNoOp — same as linux, for windows.
func TestRunPermissionsStatusWindowsIsNoOp(t *testing.T) {
	var stdout, stderr bytes.Buffer
	req := &requestStub{}
	chk := &checkStub{result: true}
	rc := runPermissions(&stdout, &stderr, "windows", true, false, req.fn, chk.fn)
	if rc != 0 {
		t.Fatalf("expected exit 0 on windows --status, got %d", rc)
	}
	if !strings.Contains(stdout.String(), "not required") || !strings.Contains(stdout.String(), "windows") {
		t.Fatalf("expected stdout to mention 'not required on windows', got %q", stdout.String())
	}
	if req.called != 0 {
		t.Fatalf("--status on windows MUST NOT call RequestAccessibility; got %d calls", req.called)
	}
}

// TestRunPermissionsNoStatusDarwinDoesNotCallCheck pins the symmetric
// guarantee: when --status is NOT set, the prompting RequestAccessibility
// path is taken and CheckAccessibility is not invoked.
func TestRunPermissionsNoStatusDarwinDoesNotCallCheck(t *testing.T) {
	var stdout, stderr bytes.Buffer
	req := &requestStub{}
	chk := &checkStub{result: true}
	rc := runPermissions(&stdout, &stderr, "darwin", false, false, req.fn, chk.fn)
	if rc != 0 {
		t.Fatalf("expected exit 0 on AX granted (no --status), got %d", rc)
	}
	if req.called != 1 {
		t.Fatalf("no-status path MUST call RequestAccessibility exactly once; got %d", req.called)
	}
	if chk.called != 0 {
		t.Fatalf("no-status path MUST NOT call CheckAccessibility; got %d", chk.called)
	}
}

func TestRunInstallRequiresSkillsFlag(t *testing.T) {
	var stdout, stderr bytes.Buffer
	rc := runInstall(&stdout, &stderr, nil, func(string) (string, error) {
		return "", errors.New("codex not found")
	}, func(windowctl.SkillInstallOptions) ([]windowctl.SkillInstallResult, error) {
		t.Fatal("install action should not run without --skills")
		return nil, nil
	})
	if rc != 2 {
		t.Fatalf("expected exit 2, got %d", rc)
	}
	if !strings.Contains(stderr.String(), "--skills is required") {
		t.Fatalf("expected --skills requirement in stderr, got %q", stderr.String())
	}
}

func TestRunInstallSkillsAutoIncludesAgentsWhenCodexIsPresent(t *testing.T) {
	var stdout, stderr bytes.Buffer
	var got windowctl.SkillInstallOptions
	install := func(opts windowctl.SkillInstallOptions) ([]windowctl.SkillInstallResult, error) {
		got = opts
		return []windowctl.SkillInstallResult{
			{Host: "claude", Path: "/tmp/claude/window-ctl-skill", Action: "installed"},
			{Host: "agents", Path: "/tmp/agents/window-ctl-skill", Action: "installed"},
		}, nil
	}
	rc := runInstall(&stdout, &stderr, []string{"--skills"}, func(name string) (string, error) {
		if name == "codex" {
			return "/usr/local/bin/codex", nil
		}
		return "", errors.New("unexpected binary lookup")
	}, install)
	if rc != 0 {
		t.Fatalf("expected exit 0, got %d (stderr=%q)", rc, stderr.String())
	}
	if !got.IncludeAgents {
		t.Fatalf("expected codex detection to enable agents install, got %+v", got)
	}
	if !strings.Contains(stdout.String(), "agents: installed") {
		t.Fatalf("expected agents install line, got %q", stdout.String())
	}
}

// parseFilterFlags is a tiny harness used by the BUG-3 / BUG-5 tests:
// it sets up a flag.FlagSet shaped like the action subcommands and
// returns it post-Parse so the validation helpers can be exercised in
// isolation (the real subcommands call os.Exit on bad input, so we
// can't drive them end-to-end from a unit test).
func parseFilterFlags(t *testing.T, args []string) (*flag.FlagSet, *string, *string, *int) {
	t.Helper()
	fs := flag.NewFlagSet("test", flag.ContinueOnError)
	fs.SetOutput(&bytes.Buffer{}) // swallow flag's own usage output
	title := fs.String("title", "", "")
	app := fs.String("app", "", "")
	monitor := fs.Int("monitor", 0, "")
	if err := fs.Parse(args); err != nil {
		t.Fatalf("flag parse failed: %v", err)
	}
	return fs, title, app, monitor
}

// TestRejectEmptyFilterFlagsExplicitTitleEmpty pins BUG-3: passing
// --title "" explicitly (typically from an unset shell variable like
// --title "$VAR") MUST be a hard error rather than silently
// equivalent to "no filter set".
func TestRejectEmptyFilterFlagsExplicitTitleEmpty(t *testing.T) {
	fs, title, app, _ := parseFilterFlags(t, []string{"--title", ""})
	err := rejectEmptyFilterFlags(fs, title, app)
	if err == nil || !strings.Contains(err.Error(), "--title cannot be empty") {
		t.Fatalf("expected --title cannot be empty error, got %v", err)
	}
}

func TestRejectEmptyFilterFlagsExplicitAppEmpty(t *testing.T) {
	fs, title, app, _ := parseFilterFlags(t, []string{"--app", ""})
	err := rejectEmptyFilterFlags(fs, title, app)
	if err == nil || !strings.Contains(err.Error(), "--app cannot be empty") {
		t.Fatalf("expected --app cannot be empty error, got %v", err)
	}
}

// TestRejectEmptyFilterFlagsOmittedIsOK confirms the symmetric
// guarantee: when neither flag is passed, the helper returns nil so
// the caller's "at least one of --title / --app required" check still
// runs (rather than this helper masking it).
func TestRejectEmptyFilterFlagsOmittedIsOK(t *testing.T) {
	fs, title, app, _ := parseFilterFlags(t, []string{})
	if err := rejectEmptyFilterFlags(fs, title, app); err != nil {
		t.Fatalf("expected nil for omitted flags, got %v", err)
	}
}

func TestRejectEmptyFilterFlagsNonEmptyValueIsOK(t *testing.T) {
	fs, title, app, _ := parseFilterFlags(t, []string{"--title", "Chrome"})
	if err := rejectEmptyFilterFlags(fs, title, app); err != nil {
		t.Fatalf("expected nil for non-empty title, got %v", err)
	}
}

// TestMonitorIDFromFlagOmittedReturnsNil pins BUG-5: when --monitor
// is not passed at all, the helper returns nil so the caller
// auto-resolves.
func TestMonitorIDFromFlagOmittedReturnsNil(t *testing.T) {
	fs, _, _, monitor := parseFilterFlags(t, []string{})
	id, err := monitorIDFromFlag(fs, monitor)
	if err != nil {
		t.Fatalf("unexpected error for omitted --monitor: %v", err)
	}
	if id != nil {
		t.Fatalf("expected nil ID for omitted --monitor, got %d", *id)
	}
}

// TestMonitorIDFromFlagNegativeIsRejected pins BUG-5: --monitor -1
// passed explicitly used to be silently treated as "auto-resolve"
// because -1 was the flag's sentinel default. Now any value < 1 is a
// hard error.
func TestMonitorIDFromFlagNegativeIsRejected(t *testing.T) {
	fs, _, _, monitor := parseFilterFlags(t, []string{"--monitor", "-1"})
	id, err := monitorIDFromFlag(fs, monitor)
	if err == nil {
		t.Fatalf("expected error for --monitor -1, got id=%v", id)
	}
	if !strings.Contains(err.Error(), ">= 1") {
		t.Fatalf("expected error to explain >=1 requirement, got %q", err.Error())
	}
}

func TestMonitorIDFromFlagZeroIsRejected(t *testing.T) {
	fs, _, _, monitor := parseFilterFlags(t, []string{"--monitor", "0"})
	if _, err := monitorIDFromFlag(fs, monitor); err == nil {
		t.Fatalf("expected error for --monitor 0")
	}
}

func TestMonitorIDFromFlagValidIsReturned(t *testing.T) {
	fs, _, _, monitor := parseFilterFlags(t, []string{"--monitor", "2"})
	id, err := monitorIDFromFlag(fs, monitor)
	if err != nil {
		t.Fatalf("unexpected error for --monitor 2: %v", err)
	}
	if id == nil || *id != 2 {
		t.Fatalf("expected id=2, got %v", id)
	}
}

// TestFormatNoMatchFilterTitleOnly / AppOnly / Both pin BUG-12: the
// "no window matched" stderr line MUST echo the filter values so
// users can see which input failed without re-running with -v.
func TestFormatNoMatchFilterTitleOnly(t *testing.T) {
	got := formatNoMatchFilter("Inbox", "")
	if !strings.Contains(got, `--title="Inbox"`) {
		t.Fatalf("expected title in error, got %q", got)
	}
}

func TestFormatNoMatchFilterAppOnly(t *testing.T) {
	got := formatNoMatchFilter("", "Cromne")
	if !strings.Contains(got, `--app="Cromne"`) {
		t.Fatalf("expected app in error, got %q", got)
	}
}

func TestFormatNoMatchFilterBoth(t *testing.T) {
	got := formatNoMatchFilter("Inbox", "Google Chrome")
	if !strings.Contains(got, `--title="Inbox"`) || !strings.Contains(got, `--app="Google Chrome"`) {
		t.Fatalf("expected both filters in error, got %q", got)
	}
}

// TestRunPermissionsStatusJSONGranted pins BUG-11: --status --json
// emits {"trusted": true} on stdout when AX is granted, exit 0.
func TestRunPermissionsStatusJSONGranted(t *testing.T) {
	var stdout, stderr bytes.Buffer
	rc := runPermissions(&stdout, &stderr, "darwin", true, true, func() error { return nil }, func() bool { return true })
	if rc != 0 {
		t.Fatalf("expected exit 0, got %d (stderr=%q)", rc, stderr.String())
	}
	var got map[string]bool
	if err := json.Unmarshal(stdout.Bytes(), &got); err != nil {
		t.Fatalf("stdout is not JSON: %v\n%s", err, stdout.String())
	}
	if got["trusted"] != true {
		t.Fatalf(`expected {"trusted": true}, got %v`, got)
	}
}

// TestRunPermissionsStatusJSONDenied pins BUG-11: --status --json
// emits {"trusted": false} on stdout when AX is denied, exit 0
// (read-only state inspection — both states are valid).
func TestRunPermissionsStatusJSONDenied(t *testing.T) {
	var stdout, stderr bytes.Buffer
	rc := runPermissions(&stdout, &stderr, "darwin", true, true, func() error { return nil }, func() bool { return false })
	if rc != 0 {
		t.Fatalf("expected exit 0 even when denied (read-only), got %d (stderr=%q)", rc, stderr.String())
	}
	var got map[string]bool
	if err := json.Unmarshal(stdout.Bytes(), &got); err != nil {
		t.Fatalf("stdout is not JSON: %v\n%s", err, stdout.String())
	}
	if got["trusted"] != false {
		t.Fatalf(`expected {"trusted": false}, got %v`, got)
	}
}

// TestRunPermissionsStatusJSONLinux pins the platform-symmetric
// behaviour: on non-darwin --status --json emits {"trusted": true}
// (no per-process AX gate exists, so trust is implicit) instead of a
// "not required on linux" prose line that would break script
// consumers.
func TestRunPermissionsStatusJSONLinux(t *testing.T) {
	var stdout, stderr bytes.Buffer
	rc := runPermissions(&stdout, &stderr, "linux", true, true, func() error { return nil }, func() bool { return true })
	if rc != 0 {
		t.Fatalf("expected exit 0, got %d", rc)
	}
	var got map[string]bool
	if err := json.Unmarshal(stdout.Bytes(), &got); err != nil {
		t.Fatalf("stdout is not JSON on linux: %v\n%s", err, stdout.String())
	}
	if got["trusted"] != true {
		t.Fatalf(`expected {"trusted": true} on linux, got %v`, got)
	}
}

// TestRunBatchTextOutputMixedSuccessFailure pins the per-line text
// output: success goes to stdout as "[N] <label>: ok", failure goes to
// stderr as "[N] <label>: <err>", and the exit code is 1 when any
// entry failed (even though others succeeded).
func TestRunBatchTextOutputMixedSuccessFailure(t *testing.T) {
	stdin := strings.NewReader(`[
		{"app":"Ghostty","x":0,"y":0,"w":100,"h":100},
		{"app":"Missing","zone":"1A"}
	]`)
	var stdout, stderr bytes.Buffer
	apply := func(entries []windowctl.BatchEntry) []windowctl.BatchResult {
		return []windowctl.BatchResult{
			{Entry: entries[0], Err: nil},
			{Entry: entries[1], Err: windowctl.ErrNoMatch},
		}
	}
	rc := runBatch(stdin, &stdout, &stderr, "", false, apply)
	if rc != 1 {
		t.Fatalf("expected exit 1 when any entry fails, got %d", rc)
	}
	if !strings.Contains(stdout.String(), "[1] Ghostty: ok") {
		t.Errorf("stdout missing success line, got %q", stdout.String())
	}
	if !strings.Contains(stderr.String(), "[2] Missing:") {
		t.Errorf("stderr missing failure line, got %q", stderr.String())
	}
}

// TestRunBatchAllOkExitsZero pins the symmetric guarantee: when every
// entry succeeds the exit code is 0.
func TestRunBatchAllOkExitsZero(t *testing.T) {
	stdin := strings.NewReader(`[{"app":"Ghostty","zone":"1A"}]`)
	var stdout, stderr bytes.Buffer
	apply := func(entries []windowctl.BatchEntry) []windowctl.BatchResult {
		return []windowctl.BatchResult{{Entry: entries[0], Err: nil}}
	}
	rc := runBatch(stdin, &stdout, &stderr, "", false, apply)
	if rc != 0 {
		t.Fatalf("expected exit 0 when all entries succeed, got %d (stderr=%q)", rc, stderr.String())
	}
}

// TestRunBatchInvalidJSONExitsTwo pins that input-level errors (parse
// failure) exit 2 — distinct from per-entry failures (1).
func TestRunBatchInvalidJSONExitsTwo(t *testing.T) {
	stdin := strings.NewReader(`{not json}`)
	var stdout, stderr bytes.Buffer
	apply := func(entries []windowctl.BatchEntry) []windowctl.BatchResult { return nil }
	rc := runBatch(stdin, &stdout, &stderr, "", false, apply)
	if rc != 2 {
		t.Fatalf("expected exit 2 on invalid JSON input, got %d", rc)
	}
	if !strings.Contains(stderr.String(), "parsing JSON") {
		t.Errorf("stderr missing parse-error context, got %q", stderr.String())
	}
}

// TestRunBatchJSONOutputShape pins the --json wire format: a single
// stdout array with one element per entry, each {"entry": {...},
// "ok": true} or {"entry": {...}, "error": "..."}.
func TestRunBatchJSONOutputShape(t *testing.T) {
	stdin := strings.NewReader(`[
		{"app":"Ghostty","zone":"1A"},
		{"app":"Missing","zone":"1A"}
	]`)
	var stdout, stderr bytes.Buffer
	apply := func(entries []windowctl.BatchEntry) []windowctl.BatchResult {
		return []windowctl.BatchResult{
			{Entry: entries[0], Err: nil},
			{Entry: entries[1], Err: windowctl.ErrNoMatch},
		}
	}
	rc := runBatch(stdin, &stdout, &stderr, "", true, apply)
	if rc != 1 {
		t.Fatalf("expected exit 1, got %d", rc)
	}
	var got []map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &got); err != nil {
		t.Fatalf("stdout is not a JSON array: %v\n%s", err, stdout.String())
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 result records, got %d: %s", len(got), stdout.String())
	}
	if got[0]["ok"] != true {
		t.Errorf("entry 0 expected ok=true, got %v", got[0])
	}
	if _, hasOk := got[1]["ok"]; hasOk {
		t.Errorf("entry 1 must NOT have ok set when it errored, got %v", got[1])
	}
	if _, hasErr := got[1]["error"]; !hasErr {
		t.Errorf("entry 1 expected error field, got %v", got[1])
	}
}

// TestErrAccessibilityDeniedNotPrefixedWithCommandName pins BUG-4:
// the sentinel's message MUST NOT contain the leading "windowctl: "
// prefix — the CLI adds it once at print time, so embedding it in the
// sentinel doubles up.
func TestErrAccessibilityDeniedNotPrefixedWithCommandName(t *testing.T) {
	if strings.HasPrefix(windowctl.ErrAccessibilityDenied.Error(), "windowctl:") {
		t.Fatalf("ErrAccessibilityDenied must not start with 'windowctl:' (the CLI adds that); got %q",
			windowctl.ErrAccessibilityDenied.Error())
	}
}
