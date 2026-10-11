package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"runtime"
	"text/tabwriter"

	windowctl "github.com/muthuishere/windowctl"
)

func main() {
	if len(os.Args) < 2 {
		usage(os.Stderr)
		os.Exit(2)
	}

	autoInstallSkill(os.Args[1])

	switch os.Args[1] {
	case "windows":
		windowsCmd(os.Args[2:])
	case "monitors":
		monitorsCmd(os.Args[2:])
	case "move":
		moveCmd(os.Args[2:])
	case "focus":
		focusCmd(os.Args[2:])
	case "resize":
		resizeCmd(os.Args[2:])
	case "permissions":
		permissionsCmd(os.Args[2:])
	case "batch":
		batchCmd(os.Args[2:])
	case "screenshot":
		screenshotCmd(os.Args[2:])
	case "mouse":
		mouseCmd(os.Args[2:])
	case "type":
		typeCmd(os.Args[2:])
	case "key":
		keyCmd(os.Args[2:])
	case "launch":
		launchCmd(os.Args[2:])
	case "wait":
		waitCmd(os.Args[2:])
	case "find":
		findCmd(os.Args[2:])
	case "draw":
		drawCmd(os.Args[2:])
	case "remote":
		remoteCmd(os.Args[2:])
	case "install":
		installCmd(os.Args[2:])
	case "uninstall":
		uninstallCmd(os.Args[2:])
	case "-h", "--help", "help":
		usage(os.Stdout)
	default:
		fmt.Fprintf(os.Stderr, "unknown command %q\n\n", os.Args[1])
		usage(os.Stderr)
		os.Exit(2)
	}
}

func usage(w io.Writer) {
	fmt.Fprintln(w, `windowctl — cross-platform window management

Usage:
  windowctl windows list [--title <s>] [--app <s>] [--json]
  windowctl monitors list [--json]
  windowctl move (--title <s> | --app <s>) [--monitor <n>] (--zone <z> | --x <n> --y <n> --w <n> --h <n>)
  windowctl focus (--title <s> | --app <s>)
  windowctl resize (--title <s> | --app <s>) --w <n> --h <n>
  windowctl permissions [--status] [--json] [--screen]
  windowctl batch [--file <path>] [--json]   (reads JSON array of entries from stdin or --file)
  windowctl screenshot [--monitor <n>] [--x <n> --y <n> --w <n> --h <n>] [--title <s> | --app <s>] [--out <path>] [--json]
  windowctl mouse move --x <n> --y <n> [--monitor <n>]
  windowctl mouse click [--x <n> --y <n>] [--monitor <n>] [--right|--middle] [--double]
  windowctl mouse position [--json]
  windowctl type --text <s> [--title <s> | --app <s>]   (with a filter: focus + verify before typing)
  windowctl key --combo <s> [--title <s> | --app <s>]   (e.g. "cmd+shift+s", "ctrl+c", "enter")
  windowctl launch --app <s>
  windowctl wait (--title <s> | --app <s>) [--timeout <ms>] [--json]
  windowctl draw (--at X,Y | --rect X,Y,W,H | --title <s> | --app <s> | --mouse) --text <s> [--style arrow|ring|box] [--shape bend|straight|zigzag|spiral] [--color <c>] [--detach] [--preview <png>] [--json]
  windowctl find (--title <s> | --app <s>) --text <s> [--source auto|accessibility|ocr] [--role button|menuitem|...] [--all] [--json]
  windowctl draw (--title <s> | --app <s>) --find <s> --text <s>   (circle + arrow at that text)
  windowctl draw stop | draw config [--init] | draw fonts
  windowctl remote [--monitor <n>] [--port <n>] [--fps <n>] [--tunnel]   (browser screen-share + control; Ctrl-C to stop)
  windowctl install --skills [--agents]
  windowctl uninstall --skills [--agents]`)
}

func windowsCmd(args []string) {
	if len(args) < 1 || args[0] != "list" {
		fmt.Fprintln(os.Stderr, "usage: windowctl windows list [--title <s>] [--app <s>] [--json]")
		os.Exit(2)
	}
	fs := flag.NewFlagSet("windows list", flag.ExitOnError)
	title := fs.String("title", "", "filter by window title (case-insensitive substring)")
	app := fs.String("app", "", "filter by application name (case-insensitive)")
	asJSON := fs.Bool("json", false, "emit JSON instead of a table")
	_ = fs.Parse(args[1:])

	if err := rejectEmptyFilterFlags(fs, title, app); err != nil {
		fmt.Fprintln(os.Stderr, "windowctl windows list:", err)
		os.Exit(2)
	}

	ws, err := windowctl.ListWindows(windowctl.Filter{Title: *title, App: *app})
	if err != nil {
		fmt.Fprintln(os.Stderr, "windowctl:", err)
		os.Exit(1)
	}
	if *asJSON {
		_ = printWindowsJSON(os.Stdout, ws)
		return
	}
	printWindowsTable(os.Stdout, ws)
}

// rejectEmptyFilterFlags hardens the "$VAR was unset" footgun: an
// explicitly-passed --title="" or --app="" must be a hard error rather
// than silently equivalent to "no filter set". Returns nil when neither
// flag was passed empty (the caller decides whether at least one is
// required).
func rejectEmptyFilterFlags(fs *flag.FlagSet, title, app *string) error {
	var bad string
	fs.Visit(func(f *flag.Flag) {
		if bad != "" {
			return
		}
		switch f.Name {
		case "title":
			if *title == "" {
				bad = "--title cannot be empty"
			}
		case "app":
			if *app == "" {
				bad = "--app cannot be empty"
			}
		}
	})
	if bad != "" {
		return errors.New(bad)
	}
	return nil
}

// monitorIDFromFlag reads the --monitor flag respecting "explicitly
// passed" semantics. Returns (nil, nil) when --monitor was not passed
// (caller should auto-resolve), (ptr, nil) when it was passed with a
// valid ID >= 1, or (nil, err) when it was passed with a value < 1
// (the explicit-bad-input case BUG-5 was about).
func monitorIDFromFlag(fs *flag.FlagSet, monitor *int) (*int, error) {
	passed := false
	fs.Visit(func(f *flag.Flag) {
		if f.Name == "monitor" {
			passed = true
		}
	})
	if !passed {
		return nil, nil
	}
	if *monitor < 1 {
		return nil, errors.New("--monitor must be >= 1 (use 1, 2, 3, ...; omit to auto-resolve)")
	}
	return monitor, nil
}

// formatNoMatchFilter wraps core.ErrNoMatch with the actual filter
// values the user passed so the diagnostic loop is one round-trip
// shorter (BUG-12). Both filters are echoed when both were set; the
// caller should only invoke this when errors.Is(err, ErrNoMatch).
func formatNoMatchFilter(title, app string) string {
	switch {
	case title != "" && app != "":
		return fmt.Sprintf("no window matched filter --title=%q --app=%q", title, app)
	case app != "":
		return fmt.Sprintf("no window matched filter --app=%q", app)
	case title != "":
		return fmt.Sprintf("no window matched filter --title=%q", title)
	default:
		// Defensive — every action subcommand requires at least one
		// filter, so we shouldn't reach here. Fall back to the
		// sentinel's bare message.
		return windowctl.ErrNoMatch.Error()
	}
}

func monitorsCmd(args []string) {
	if len(args) < 1 || args[0] != "list" {
		fmt.Fprintln(os.Stderr, "usage: windowctl monitors list [--json]")
		os.Exit(2)
	}
	fs := flag.NewFlagSet("monitors list", flag.ExitOnError)
	asJSON := fs.Bool("json", false, "emit JSON instead of a table")
	_ = fs.Parse(args[1:])

	ms, err := windowctl.ListMonitors()
	if err != nil {
		fmt.Fprintln(os.Stderr, "windowctl:", err)
		os.Exit(1)
	}
	if *asJSON {
		_ = printMonitorsJSON(os.Stdout, ms)
		return
	}
	printMonitorsTable(os.Stdout, ms)
}

func printWindowsTable(out io.Writer, ws []windowctl.Window) {
	tw := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "ID\tTITLE\tAPP\tPID\tMONITOR\tBOUNDS")
	for _, w := range ws {
		fmt.Fprintf(tw, "%s\t%s\t%s\t%d\t%d\t%dx%d+%d+%d\n",
			w.ID, w.Title, w.App, w.PID, w.Monitor, w.Bounds.W, w.Bounds.H, w.Bounds.X, w.Bounds.Y)
	}
	_ = tw.Flush()
}

func printWindowsJSON(out io.Writer, ws []windowctl.Window) error {
	return json.NewEncoder(out).Encode(ws)
}

func printMonitorsTable(out io.Writer, ms []windowctl.Monitor) {
	tw := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "ID\tX\tY\tWIDTH\tHEIGHT\tPRIMARY\tACTIVE\tFOCUSED")
	for _, m := range ms {
		fmt.Fprintf(tw, "%d\t%d\t%d\t%d\t%d\t%t\t%t\t%t\n",
			m.ID, m.X, m.Y, m.Width, m.Height, m.Primary, m.Active, m.Focused)
	}
	_ = tw.Flush()
}

func printMonitorsJSON(out io.Writer, ms []windowctl.Monitor) error {
	return json.NewEncoder(out).Encode(ms)
}

func moveCmd(args []string) {
	fs := flag.NewFlagSet("move", flag.ExitOnError)
	title := fs.String("title", "", "match by window title (case-insensitive substring)")
	app := fs.String("app", "", "match by application name (case-insensitive)")
	monitor := fs.Int("monitor", 0, "target monitor ID (>=1; omit to auto-resolve)")
	zone := fs.String("zone", "", "target zone (1A,1B,2A..2D or N:M)")
	x := fs.Int("x", 0, "target x (with --monitor: relative; otherwise: absolute)")
	y := fs.Int("y", 0, "target y (with --monitor: relative; otherwise: absolute)")
	w := fs.Int("w", 0, "target width")
	h := fs.Int("h", 0, "target height")
	_ = fs.Parse(args)

	if err := rejectEmptyFilterFlags(fs, title, app); err != nil {
		fmt.Fprintln(os.Stderr, "windowctl move:", err)
		os.Exit(2)
	}
	if *title == "" && *app == "" {
		fmt.Fprintln(os.Stderr, "windowctl move: --title or --app is required")
		os.Exit(2)
	}

	coordSet := false
	fs.Visit(func(f *flag.Flag) {
		switch f.Name {
		case "x", "y", "w", "h":
			coordSet = true
		}
	})

	if *zone != "" && coordSet {
		fmt.Fprintln(os.Stderr, "windowctl move: --zone and --x/--y/--w/--h are mutually exclusive")
		os.Exit(2)
	}
	if *zone == "" && !coordSet {
		fmt.Fprintln(os.Stderr, "windowctl move: either --zone or --x/--y/--w/--h is required")
		os.Exit(2)
	}

	if *zone == "" {
		// Coord-mode: w/h must be > 0 just like resize requires.
		// Otherwise --w 0 silently accepts a OS-clamped geometry that
		// neither errors nor honors the requested rect.
		if *w <= 0 || *h <= 0 {
			fmt.Fprintln(os.Stderr, "windowctl move: --w and --h must be > 0 in coord mode")
			os.Exit(2)
		}
	}

	monitorID, err := monitorIDFromFlag(fs, monitor)
	if err != nil {
		fmt.Fprintln(os.Stderr, "windowctl move:", err)
		os.Exit(2)
	}

	match := windowctl.Match{Title: *title, App: *app}
	if *zone != "" {
		err = windowctl.MoveZone(match, monitorID, *zone)
	} else {
		err = windowctl.MoveCoords(match, monitorID, windowctl.Rect{X: *x, Y: *y, W: *w, H: *h})
	}
	if err != nil {
		if errors.Is(err, windowctl.ErrNoMatch) {
			fmt.Fprintln(os.Stderr, "windowctl:", formatNoMatchFilter(*title, *app))
		} else {
			fmt.Fprintln(os.Stderr, "windowctl:", err)
		}
		os.Exit(1)
	}
}

func focusCmd(args []string) {
	fs := flag.NewFlagSet("focus", flag.ExitOnError)
	title := fs.String("title", "", "match by window title (case-insensitive substring)")
	app := fs.String("app", "", "match by application name (case-insensitive)")
	_ = fs.Parse(args)

	if err := rejectEmptyFilterFlags(fs, title, app); err != nil {
		fmt.Fprintln(os.Stderr, "windowctl focus:", err)
		os.Exit(2)
	}
	if *title == "" && *app == "" {
		fmt.Fprintln(os.Stderr, "windowctl focus: --title or --app is required")
		os.Exit(2)
	}

	if err := windowctl.Focus(windowctl.Match{Title: *title, App: *app}); err != nil {
		if errors.Is(err, windowctl.ErrNoMatch) {
			fmt.Fprintln(os.Stderr, "windowctl:", formatNoMatchFilter(*title, *app))
		} else {
			fmt.Fprintln(os.Stderr, "windowctl:", err)
		}
		os.Exit(1)
	}
}

func resizeCmd(args []string) {
	fs := flag.NewFlagSet("resize", flag.ExitOnError)
	title := fs.String("title", "", "match by window title (case-insensitive substring)")
	app := fs.String("app", "", "match by application name (case-insensitive)")
	w := fs.Int("w", 0, "new width (required)")
	h := fs.Int("h", 0, "new height (required)")
	_ = fs.Parse(args)

	if err := rejectEmptyFilterFlags(fs, title, app); err != nil {
		fmt.Fprintln(os.Stderr, "windowctl resize:", err)
		os.Exit(2)
	}
	if *title == "" && *app == "" {
		fmt.Fprintln(os.Stderr, "windowctl resize: --title or --app is required")
		os.Exit(2)
	}
	if *w <= 0 || *h <= 0 {
		fmt.Fprintln(os.Stderr, "windowctl resize: --w and --h are required and must be > 0")
		os.Exit(2)
	}

	if err := windowctl.Resize(windowctl.Match{Title: *title, App: *app}, *w, *h); err != nil {
		if errors.Is(err, windowctl.ErrNoMatch) {
			fmt.Fprintln(os.Stderr, "windowctl:", formatNoMatchFilter(*title, *app))
		} else {
			fmt.Fprintln(os.Stderr, "windowctl:", err)
		}
		os.Exit(1)
	}
}

// permissionsCmd is the `windowctl permissions` entry point. Per
// docs/specs/11-permissions-subcommand.md it triggers the macOS AX
// trust prompt (the only place in windowctl that does so) and is a
// no-op on linux/windows. With --status it switches to read-only
// mode and inspects the trust state without prompting. Real work
// happens in runPermissions, which is split out so it is
// unit-testable without spawning the binary or touching the real
// adapter.
func permissionsCmd(args []string) {
	fs := flag.NewFlagSet("permissions", flag.ExitOnError)
	status := fs.Bool("status", false, "report current permission state without triggering the macOS system prompt (read-only)")
	asJSON := fs.Bool("json", false, "emit JSON instead of human-readable text (only valid with --status)")
	screen := fs.Bool("screen", false, "operate on the Screen Recording permission (needed by screenshot) instead of Accessibility")
	_ = fs.Parse(args)
	if *asJSON && !*status {
		fmt.Fprintln(os.Stderr, "windowctl permissions: --json requires --status")
		os.Exit(2)
	}
	var rc int
	if *screen {
		rc = runScreenPermissions(os.Stdout, os.Stderr, runtime.GOOS, *status, *asJSON, windowctl.RequestScreenCapture, windowctl.CheckScreenCapture)
	} else {
		rc = runPermissions(os.Stdout, os.Stderr, runtime.GOOS, *status, *asJSON, windowctl.RequestAccessibility, windowctl.CheckAccessibility)
	}
	if rc != 0 {
		os.Exit(rc)
	}
}

// runScreenPermissions is the Screen Recording twin of runPermissions,
// same platform-conditional shape: linux/windows have no comparable
// gate ("not required"), darwin either checks silently (--status) or
// requests with a possible one-time system prompt.
func runScreenPermissions(stdout, stderr io.Writer, goos string, status, asJSON bool, requestFn func() error, checkFn func() bool) int {
	if goos != "darwin" {
		if asJSON && status {
			_ = json.NewEncoder(stdout).Encode(map[string]bool{"granted": true})
			return 0
		}
		fmt.Fprintf(stdout, "Screen Recording permission: not required on %s\n", goos)
		return 0
	}
	if status {
		granted := checkFn()
		if asJSON {
			_ = json.NewEncoder(stdout).Encode(map[string]bool{"granted": granted})
			if granted {
				return 0
			}
			return 1
		}
		if granted {
			fmt.Fprintln(stdout, "Screen Recording permission: granted")
			return 0
		}
		fmt.Fprintln(stderr, windowctl.ErrScreenCaptureDenied.Error())
		return 1
	}
	if err := requestFn(); err != nil {
		fmt.Fprintln(stderr, err.Error())
		return 1
	}
	fmt.Fprintln(stdout, "Screen Recording permission: granted")
	return 0
}

// runPermissions encapsulates the platform-conditional logic for the
// permissions subcommand so the side-effecting bits (the actual AX
// call, os.Exit) can be injected by tests.
//
//   - When status=true: the read-only path. On darwin invokes checkFn
//     (no prompt). true → "granted" + exit 0. false → actionable
//     denial message on stderr + non-zero exit. requestFn is NEVER
//     called so the system dialog can't fire from this code path.
//     On linux / windows: prints "not required on {os}" and returns
//     0; neither adapter method is invoked (the message is
//     platform-derived).
//   - When status=false: the default prompting path (preserved from
//     Sreyash 002). On darwin invokes requestFn. nil → "granted" +
//     exit 0. ErrAccessibilityDenied → actionable denial message on
//     stderr + non-zero exit. Any other error → surfaces it on
//     stderr + non-zero exit. On linux / windows: invokes requestFn
//     for symmetry but ignores the result; prints "not required on
//     {os}" and returns 0.
func runPermissions(stdout, stderr io.Writer, goos string, status, asJSON bool, requestFn func() error, checkFn func() bool) int {
	if status {
		if goos != "darwin" {
			if asJSON {
				// Non-darwin trust is implicit (no per-process AX
				// gate). Emit `trusted: true` so script consumers
				// don't have to special-case the platform.
				_ = json.NewEncoder(stdout).Encode(map[string]bool{"trusted": true})
				return 0
			}
			fmt.Fprintf(stdout, "Accessibility permission: not required on %s\n", goos)
			return 0
		}
		trusted := checkFn()
		if asJSON {
			// --status is a read-only state inspection: both
			// granted and denied are valid answers, exit 0 either
			// way. Wrapper scripts branch on .trusted.
			_ = json.NewEncoder(stdout).Encode(map[string]bool{"trusted": trusted})
			return 0
		}
		if trusted {
			fmt.Fprintln(stdout, "Accessibility permission: granted")
			return 0
		}
		fmt.Fprintln(stderr, "Accessibility permission: denied — run 'windowctl permissions' to grant, or grant manually in System Settings → Privacy & Security → Accessibility")
		return 1
	}

	if goos != "darwin" {
		// Invoke for symmetry — adapters are no-op on non-darwin —
		// but the user-visible message is platform-derived.
		_ = requestFn()
		fmt.Fprintf(stdout, "Accessibility permission: not required on %s\n", goos)
		return 0
	}

	err := requestFn()
	if err == nil {
		fmt.Fprintln(stdout, "Accessibility permission: granted")
		return 0
	}
	if errors.Is(err, windowctl.ErrAccessibilityDenied) {
		fmt.Fprintln(stderr, "Accessibility permission: denied — grant access in System Settings → Privacy & Security → Accessibility, then re-run")
		return 1
	}
	fmt.Fprintln(stderr, "windowctl permissions:", err)
	return 1
}

// batchCmd is the `windowctl batch` entry point. It reads a JSON array
// of BatchEntry objects from stdin (or --file), applies each entry as
// a Move sequentially, and never aborts on a single failure. Per-entry
// success / failure is reported one line at a time (or as a JSON array
// with --json). Exit code: 0 if every entry succeeded, 1 if any entry
// failed, 2 if the input itself was invalid (parse error, file not
// found, etc.).
//
// Sample input:
//
//	[
//	  {"app": "Ghostty", "monitor": 2, "x": 0, "y": 25, "w": 1920, "h": 1055},
//	  {"app": "Activity Monitor", "monitor": 1, "zone": "2A"},
//	  {"title": "Inbox", "x": 100, "y": 100, "w": 800, "h": 600}
//	]
func batchCmd(args []string) {
	fs := flag.NewFlagSet("batch", flag.ExitOnError)
	file := fs.String("file", "", "read JSON entries from this path instead of stdin")
	asJSON := fs.Bool("json", false, "emit JSON instead of text")
	_ = fs.Parse(args)

	rc := runBatch(os.Stdin, os.Stdout, os.Stderr, *file, *asJSON, windowctl.Batch)
	if rc != 0 {
		os.Exit(rc)
	}
}

// runBatch encapsulates batchCmd's IO and dispatch so it can be unit-
// tested without spawning the binary or touching the real adapter.
// applyFn matches windowctl.Batch's signature so tests can inject a
// stub.
func runBatch(stdin io.Reader, stdout, stderr io.Writer, file string, asJSON bool, applyFn func([]windowctl.BatchEntry) []windowctl.BatchResult) int {
	src := stdin
	if file != "" {
		f, err := os.Open(file)
		if err != nil {
			fmt.Fprintln(stderr, "windowctl batch:", err)
			return 2
		}
		defer f.Close()
		src = f
	}

	data, err := io.ReadAll(src)
	if err != nil {
		fmt.Fprintln(stderr, "windowctl batch: reading input:", err)
		return 2
	}

	var entries []windowctl.BatchEntry
	if err := json.Unmarshal(data, &entries); err != nil {
		fmt.Fprintln(stderr, "windowctl batch: parsing JSON:", err)
		return 2
	}

	results := applyFn(entries)

	failed := false
	if asJSON {
		out := make([]map[string]any, 0, len(results))
		for _, r := range results {
			rec := map[string]any{"entry": r.Entry}
			if r.Err != nil {
				rec["error"] = r.Err.Error()
				failed = true
			} else {
				rec["ok"] = true
			}
			out = append(out, rec)
		}
		_ = json.NewEncoder(stdout).Encode(out)
	} else {
		for i, r := range results {
			label := batchEntryLabel(r.Entry)
			if r.Err != nil {
				fmt.Fprintf(stderr, "[%d] %s: %s\n", i+1, label, r.Err)
				failed = true
			} else {
				fmt.Fprintf(stdout, "[%d] %s: ok\n", i+1, label)
			}
		}
	}

	if failed {
		return 1
	}
	return 0
}

// batchEntryLabel picks the most informative one-token identifier for
// an entry: app first (typical case), then title, then "-" when the
// entry was so under-specified that neither was set (validation will
// have rejected it, but we still need something to print).
func batchEntryLabel(e windowctl.BatchEntry) string {
	switch {
	case e.App != "":
		return e.App
	case e.Title != "":
		return e.Title
	default:
		return "-"
	}
}
