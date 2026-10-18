// automation.go — CLI entry points for the desktop-automation surface:
// screenshot, mouse (move/click/position), type, key, launch, wait.
// Thin flag-parsing shells over the public package, same as main.go.
package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"time"

	windowctl "github.com/Jiafei-Queen/windowctl"
)

// screenshotResult is the --json wire shape. The region fields are the
// captured virtual-desktop rect — consumers translate image pixels to
// global coordinates as (X + imageX, Y + imageY); the image is already
// normalized to 1 pixel == 1 point.
type screenshotResult struct {
	Path   string `json:"Path"`
	X      int    `json:"X"`
	Y      int    `json:"Y"`
	Width  int    `json:"Width"`
	Height int    `json:"Height"`
}

func screenshotCmd(args []string) {
	fs := flag.NewFlagSet("screenshot", flag.ExitOnError)
	title := fs.String("title", "", "capture the window matched by title (case-insensitive substring)")
	app := fs.String("app", "", "capture the window matched by app name (case-insensitive)")
	monitor := fs.Int("monitor", 0, "capture this monitor (>=1), or base for a relative region")
	x := fs.Int("x", 0, "region x (with --monitor: relative; otherwise: absolute)")
	y := fs.Int("y", 0, "region y (with --monitor: relative; otherwise: absolute)")
	w := fs.Int("w", 0, "region width")
	h := fs.Int("h", 0, "region height")
	out := fs.String("out", "", "output PNG path (default screenshot-<timestamp>.png)")
	asJSON := fs.Bool("json", false, "emit the written path + captured region as JSON")
	_ = fs.Parse(args)

	if err := rejectEmptyFilterFlags(fs, title, app); err != nil {
		fmt.Fprintln(os.Stderr, "windowctl screenshot:", err)
		os.Exit(2)
	}

	regionSet := false
	fs.Visit(func(f *flag.Flag) {
		switch f.Name {
		case "x", "y", "w", "h":
			regionSet = true
		}
	})

	opts := windowctl.ScreenshotOptions{OutPath: *out}
	if opts.OutPath == "" {
		opts.OutPath = fmt.Sprintf("screenshot-%d.png", time.Now().Unix())
	}
	if *title != "" || *app != "" {
		opts.Match = &windowctl.Match{Title: *title, App: *app}
	}
	if regionSet {
		if *w <= 0 || *h <= 0 {
			fmt.Fprintln(os.Stderr, "windowctl screenshot: --w and --h must be > 0 for a region capture")
			os.Exit(2)
		}
		opts.Region = &windowctl.Rect{X: *x, Y: *y, W: *w, H: *h}
	}
	monitorID, err := monitorIDFromFlag(fs, monitor)
	if err != nil {
		fmt.Fprintln(os.Stderr, "windowctl screenshot:", err)
		os.Exit(2)
	}
	opts.Monitor = monitorID

	rect, err := windowctl.Screenshot(opts)
	if err != nil {
		if errors.Is(err, windowctl.ErrNoMatch) {
			fmt.Fprintln(os.Stderr, "windowctl:", formatNoMatchFilter(*title, *app))
		} else {
			fmt.Fprintln(os.Stderr, "windowctl:", err)
		}
		os.Exit(1)
	}
	if *asJSON {
		_ = json.NewEncoder(os.Stdout).Encode(screenshotResult{
			Path: opts.OutPath, X: rect.X, Y: rect.Y, Width: rect.W, Height: rect.H,
		})
		return
	}
	fmt.Printf("wrote %s (region %dx%d at %d,%d — image pixel (px,py) = screen point (%d+px, %d+py))\n",
		opts.OutPath, rect.W, rect.H, rect.X, rect.Y, rect.X, rect.Y)
}

func mouseCmd(args []string) {
	if len(args) < 1 {
		fmt.Fprintln(os.Stderr, "usage: windowctl mouse (move --x <n> --y <n> [--monitor <n>] | click [--x <n> --y <n>] [--monitor <n>] [--right|--middle] [--double] | position [--json])")
		os.Exit(2)
	}
	switch args[0] {
	case "move":
		mouseMoveCmd(args[1:])
	case "click":
		mouseClickCmd(args[1:])
	case "position":
		mousePositionCmd(args[1:])
	default:
		fmt.Fprintf(os.Stderr, "windowctl mouse: unknown subcommand %q (want move, click or position)\n", args[0])
		os.Exit(2)
	}
}

func mouseMoveCmd(args []string) {
	fs := flag.NewFlagSet("mouse move", flag.ExitOnError)
	x := fs.Int("x", 0, "target x (with --monitor: relative; otherwise: absolute)")
	y := fs.Int("y", 0, "target y (with --monitor: relative; otherwise: absolute)")
	monitor := fs.Int("monitor", 0, "interpret --x/--y relative to this monitor (>=1)")
	_ = fs.Parse(args)

	xSet, ySet := false, false
	fs.Visit(func(f *flag.Flag) {
		switch f.Name {
		case "x":
			xSet = true
		case "y":
			ySet = true
		}
	})
	if !xSet || !ySet {
		fmt.Fprintln(os.Stderr, "windowctl mouse move: --x and --y are required")
		os.Exit(2)
	}
	monitorID, err := monitorIDFromFlag(fs, monitor)
	if err != nil {
		fmt.Fprintln(os.Stderr, "windowctl mouse move:", err)
		os.Exit(2)
	}
	if err := windowctl.MouseMove(monitorID, *x, *y); err != nil {
		fmt.Fprintln(os.Stderr, "windowctl:", err)
		os.Exit(1)
	}
}

func mouseClickCmd(args []string) {
	fs := flag.NewFlagSet("mouse click", flag.ExitOnError)
	x := fs.Int("x", 0, "click x (with --monitor: relative; otherwise: absolute; omit both to click in place)")
	y := fs.Int("y", 0, "click y (with --monitor: relative; otherwise: absolute; omit both to click in place)")
	monitor := fs.Int("monitor", 0, "interpret --x/--y relative to this monitor (>=1)")
	right := fs.Bool("right", false, "right-click")
	middle := fs.Bool("middle", false, "middle-click")
	double := fs.Bool("double", false, "double-click")
	_ = fs.Parse(args)

	if *right && *middle {
		fmt.Fprintln(os.Stderr, "windowctl mouse click: --right and --middle are mutually exclusive")
		os.Exit(2)
	}
	xSet, ySet := false, false
	fs.Visit(func(f *flag.Flag) {
		switch f.Name {
		case "x":
			xSet = true
		case "y":
			ySet = true
		}
	})
	monitorID, err := monitorIDFromFlag(fs, monitor)
	if err != nil {
		fmt.Fprintln(os.Stderr, "windowctl mouse click:", err)
		os.Exit(2)
	}

	opts := windowctl.ClickOptions{Monitor: monitorID, Double: *double}
	if *right {
		opts.Button = windowctl.MouseRight
	}
	if *middle {
		opts.Button = windowctl.MouseMiddle
	}
	if xSet {
		opts.X = x
	}
	if ySet {
		opts.Y = y
	}
	if err := windowctl.MouseClick(opts); err != nil {
		fmt.Fprintln(os.Stderr, "windowctl:", err)
		os.Exit(1)
	}
}

func mousePositionCmd(args []string) {
	fs := flag.NewFlagSet("mouse position", flag.ExitOnError)
	asJSON := fs.Bool("json", false, "emit JSON instead of \"x y\"")
	_ = fs.Parse(args)

	x, y, err := windowctl.CursorPosition()
	if err != nil {
		fmt.Fprintln(os.Stderr, "windowctl:", err)
		os.Exit(1)
	}
	if *asJSON {
		_ = json.NewEncoder(os.Stdout).Encode(map[string]int{"X": x, "Y": y})
		return
	}
	fmt.Printf("%d %d\n", x, y)
}

func typeCmd(args []string) {
	fs := flag.NewFlagSet("type", flag.ExitOnError)
	text := fs.String("text", "", "the literal text to type")
	title := fs.String("title", "", "focus-and-verify this window first (case-insensitive substring)")
	app := fs.String("app", "", "focus-and-verify this window first (case-insensitive app name)")
	_ = fs.Parse(args)

	if *text == "" {
		fmt.Fprintln(os.Stderr, "windowctl type: --text is required")
		os.Exit(2)
	}
	if err := rejectEmptyFilterFlags(fs, title, app); err != nil {
		fmt.Fprintln(os.Stderr, "windowctl type:", err)
		os.Exit(2)
	}
	var err error
	if *title != "" || *app != "" {
		err = windowctl.TypeInto(windowctl.Match{Title: *title, App: *app}, *text)
	} else {
		err = windowctl.TypeText(*text)
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

func keyCmd(args []string) {
	fs := flag.NewFlagSet("key", flag.ExitOnError)
	combo := fs.String("combo", "", "key combo to press, e.g. \"cmd+shift+s\", \"ctrl+c\", \"enter\"")
	title := fs.String("title", "", "focus-and-verify this window first (case-insensitive substring)")
	app := fs.String("app", "", "focus-and-verify this window first (case-insensitive app name)")
	_ = fs.Parse(args)

	if *combo == "" {
		fmt.Fprintln(os.Stderr, "windowctl key: --combo is required")
		os.Exit(2)
	}
	if err := rejectEmptyFilterFlags(fs, title, app); err != nil {
		fmt.Fprintln(os.Stderr, "windowctl key:", err)
		os.Exit(2)
	}
	var err error
	if *title != "" || *app != "" {
		err = windowctl.PressKeyInto(windowctl.Match{Title: *title, App: *app}, *combo)
	} else {
		err = windowctl.PressKey(*combo)
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

func launchCmd(args []string) {
	fs := flag.NewFlagSet("launch", flag.ExitOnError)
	app := fs.String("app", "", "application to launch (macOS: app name; Windows: name/path; Linux: binary on PATH)")
	_ = fs.Parse(args)

	if *app == "" {
		fmt.Fprintln(os.Stderr, "windowctl launch: --app is required")
		os.Exit(2)
	}
	if err := windowctl.Launch(*app); err != nil {
		fmt.Fprintln(os.Stderr, "windowctl:", err)
		os.Exit(1)
	}
}

func waitCmd(args []string) {
	fs := flag.NewFlagSet("wait", flag.ExitOnError)
	title := fs.String("title", "", "wait for a window matching title (case-insensitive substring)")
	app := fs.String("app", "", "wait for a window matching app name (case-insensitive)")
	timeout := fs.Int("timeout", 10000, "give up after this many milliseconds")
	asJSON := fs.Bool("json", false, "emit the matched window as JSON")
	_ = fs.Parse(args)

	if err := rejectEmptyFilterFlags(fs, title, app); err != nil {
		fmt.Fprintln(os.Stderr, "windowctl wait:", err)
		os.Exit(2)
	}
	if *title == "" && *app == "" {
		fmt.Fprintln(os.Stderr, "windowctl wait: --title or --app is required")
		os.Exit(2)
	}
	w, err := windowctl.WaitForWindow(windowctl.Filter{Title: *title, App: *app}, *timeout)
	if err != nil {
		fmt.Fprintln(os.Stderr, "windowctl:", err)
		os.Exit(1)
	}
	if *asJSON {
		_ = json.NewEncoder(os.Stdout).Encode(w)
		return
	}
	fmt.Printf("%s\t%s\t%s\t%d\t%dx%d+%d+%d\n",
		w.ID, w.Title, w.App, w.Monitor, w.Bounds.W, w.Bounds.H, w.Bounds.X, w.Bounds.Y)
}
