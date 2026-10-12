package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/muthuishere/windowctl"
)

// drawCmd: `windowctl draw` — a big arrow with a sign over every window.
// The CLI is transport only: flag parsing, the detach/stop process
// bookkeeping, and output. Placement, geometry and pixels live in the
// library (windowctl.Draw / RenderDraw).
func drawCmd(args []string) {
	if len(args) > 0 {
		switch args[0] {
		case "stop", "clear":
			drawStopCmd(args[1:])
			return
		case "config":
			drawConfigCmd(args[1:])
			return
		case "fonts":
			for _, f := range windowctl.ListFonts() {
				fmt.Println(f)
			}
			return
		}
	}
	fs := flag.NewFlagSet("draw", flag.ExitOnError)
	fs.Usage = func() {
		fmt.Fprint(os.Stderr, `usage: windowctl draw <target> --text "..." [look] [--duration S] [--detach] [--preview out.png] [--json]
       windowctl draw stop            remove every detached drawing
       windowctl draw config [--init] show / create ~/.config/windowctl/fontconfig.json
       windowctl draw fonts           list font names usable with --font

targets (exactly one):
  --at X,Y            point            --rect X,Y,W,H     rect (boxed)
  --title T / --app A the window       --mouse            the cursor
  --monitor N         makes --at / --rect relative to monitor N
  --title/--app + --find TEXT   circle that text inside the window (see also: windowctl find)

`)
		fs.PrintDefaults()
	}
	at := fs.String("at", "", "target point X,Y")
	rect := fs.String("rect", "", "target rect X,Y,W,H")
	title := fs.String("title", "", "target the window matched by title (case-insensitive substring)")
	app := fs.String("app", "", "target the window matched by app name (case-insensitive)")
	mouse := fs.Bool("mouse", false, "target the cursor position")
	find := fs.String("find", "", "with --title/--app: point at this text in the window (button, menu, link, label; OCR fallback); boxed + arrow, --style ring to circle it")
	findSource := fs.String("source", "", "for --find: auto (accessibility, then OCR) | accessibility | ocr")
	findRole := fs.String("role", "", "for --find: button | menuitem | link | tab | checkbox | radio | textfield | text | image")
	findExact := fs.Bool("exact", false, "for --find: the whole label must equal the text")
	findNth := fs.Int("nth", 0, "for --find: use the n-th best match (1-based)")
	monitor := fs.Int("monitor", 0, "monitor (>=1) that --at / --rect are relative to")
	text := fs.String("text", "", "the sign's text (required)")
	style := fs.String("style", "", "arrow | ring | box")
	shape := fs.String("shape", "", "bend | straight | zigzag | spiral")
	size := fs.String("size", "", "S | M | L")
	border := fs.String("border", "", "shadow | white-black | black")
	corners := fs.String("corners", "", "round | sharp")
	from := fs.String("from", "", "side the sign sits on: auto | top-left | top | top-right | left | right | bottom-left | bottom | bottom-right")
	color := fs.String("color", "", "red amber orange yellow green teal blue purple pink black white, or #RRGGBB")
	textColor := fs.String("text-color", "", "sign text colour (default: readable contrast)")
	borderColor := fs.String("border-color", "", "contrast border colour")
	edgeColor := fs.String("edge-color", "", "thin edge colour (white-black / black borders)")
	font := fs.String("font", "", "font file path or system font name (see 'windowctl draw fonts')")
	fontScale := fs.Float64("font-scale", 0, "multiply the font size")
	noConfig := fs.Bool("no-config", false, "ignore ~/.config/windowctl/fontconfig.json")
	duration := fs.Float64("duration", -1, "seconds to show (default 8, or 300 with --detach; 0 = until 'draw stop')")
	detach := fs.Bool("detach", false, "return at once; the drawing stays until --duration or 'windowctl draw stop'")
	preview := fs.String("preview", "", "render to this PNG instead of showing (same pixels, nothing on screen)")
	asJSON := fs.Bool("json", false, "print where the drawing landed as JSON")
	_ = fs.Parse(args)

	fail := func(code int, err error) {
		if errors.Is(err, windowctl.ErrNoMatch) {
			fmt.Fprintln(os.Stderr, "windowctl:", formatNoMatchFilter(*title, *app))
		} else {
			fmt.Fprintln(os.Stderr, "windowctl draw:", err)
		}
		os.Exit(code)
	}
	if err := rejectEmptyFilterFlags(fs, title, app); err != nil {
		fail(2, err)
	}
	opts := windowctl.DrawOptions{
		Text: *text, Mouse: *mouse,
		Style: windowctl.DrawStyle(*style), Shape: windowctl.DrawShape(*shape), Size: windowctl.DrawSize(*size),
		Border: windowctl.DrawBorder(*border), Corners: windowctl.DrawCorners(*corners), From: windowctl.DrawFrom(*from),
		Font: *font, FontScale: *fontScale, NoConfig: *noConfig,
		Find: *find, FindSource: windowctl.FindSource(*findSource), FindRole: *findRole, FindExact: *findExact, FindNth: *findNth,
	}
	if *at != "" {
		n, err := parseInts(*at, 2, "--at X,Y")
		if err != nil {
			fail(2, err)
		}
		opts.Point = &windowctl.Point{X: n[0], Y: n[1]}
	}
	if *rect != "" {
		n, err := parseInts(*rect, 4, "--rect X,Y,W,H")
		if err != nil {
			fail(2, err)
		}
		opts.Region = &windowctl.Rect{X: n[0], Y: n[1], W: n[2], H: n[3]}
	}
	if *title != "" || *app != "" {
		opts.Match = &windowctl.Match{Title: *title, App: *app}
	}
	monitorID, err := monitorIDFromFlag(fs, monitor)
	if err != nil {
		fail(2, err)
	}
	opts.Monitor = monitorID
	for _, c := range []struct {
		raw string
		dst **windowctl.Color
	}{{*color, &opts.Color}, {*textColor, &opts.TextColor}, {*borderColor, &opts.BorderColor}, {*edgeColor, &opts.EdgeColor}} {
		if c.raw == "" {
			continue
		}
		col, err := windowctl.ParseColor(c.raw)
		if err != nil {
			fail(2, err)
		}
		*c.dst = &col
	}
	secs := *duration
	if secs < 0 {
		secs = 8
		if *detach {
			secs = 300
		}
	}
	opts.Duration = time.Duration(secs * float64(time.Second))
	opts.Avoid = windowctl.LiveDrawingSigns()

	// Resolve --find exactly once, here, and draw at that rect from then
	// on: a second search (the detached child, or the place-then-show
	// pass below) can see a changed window and land somewhere else.
	var found *windowctl.FoundText
	if opts.Find != "" {
		if opts.Match == nil {
			fail(2, errors.New("--find needs the window to search: add --title or --app"))
		}
		hits, err := windowctl.FindText(windowctl.FindOptions{
			Match: *opts.Match, Text: opts.Find, Source: opts.FindSource, Role: opts.FindRole, Exact: opts.FindExact,
		})
		if err != nil {
			fail(3, err)
		}
		i := max(opts.FindNth, 1) - 1
		if i >= len(hits) {
			fail(3, fmt.Errorf("--nth %d but only %d match(es) for %q", opts.FindNth, len(hits), opts.Find))
		}
		found = &hits[i]
		r := found.Bounds
		opts.Region, opts.Match, opts.Monitor, opts.Find = &r, nil, nil, ""
		args = append(stripFlags(args, map[string]bool{"find": true, "title": true, "app": true, "source": true, "role": true, "nth": true, "monitor": true, "style": true}, map[string]bool{"exact": true}),
			"--rect", fmt.Sprintf("%d,%d,%d,%d", r.X, r.Y, r.W, r.H))
		if opts.Style != "" {
			args = append(args, "--style", string(opts.Style))
		}
	}

	emit := func(res windowctl.DrawResult) {
		if found != nil {
			res.Found = found
		}
		if *asJSON {
			_ = json.NewEncoder(os.Stdout).Encode(res)
		}
	}

	if *preview != "" {
		res, err := windowctl.RenderDraw(opts, *preview)
		if err != nil {
			fail(1, err)
		}
		emit(res)
		if !*asJSON {
			fmt.Println(*preview)
		}
		return
	}

	if *detach {
		res, err := windowctl.SpawnDetachedDraw(append([]string{"draw"}, withoutDetach(args)...))
		if err != nil {
			fail(1, err)
		}
		emit(res)
		return
	}

	// Foreground (or the detached child): show until the duration ends,
	// `draw stop`, or Ctrl-C.
	stop := make(chan struct{})
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
	go func() { <-sig; close(stop) }()
	opts.Stop = stop
	child := windowctl.IsDrawChild()
	if child || *asJSON {
		// Place first so the registry / JSON say where it is before it shows.
		res, err := windowctl.RenderDraw(opts, os.DevNull)
		if err != nil {
			fail(1, err)
		}
		if child {
			windowctl.RegisterDrawing(res)
			defer windowctl.UnregisterDrawing()
		} else {
			emit(res)
		}
	}
	if _, err := windowctl.Draw(opts); err != nil {
		if child {
			windowctl.UnregisterDrawing()
		}
		fail(1, err)
	}
}

func parseInts(s string, n int, what string) ([]int, error) {
	parts := strings.Split(s, ",")
	if len(parts) != n {
		return nil, fmt.Errorf("%s: expected %d comma-separated integers, got %q", what, n, s)
	}
	out := make([]int, n)
	for i, p := range parts {
		v, err := strconv.Atoi(strings.TrimSpace(p))
		if err != nil {
			return nil, fmt.Errorf("%s: %q is not an integer", what, p)
		}
		out[i] = v
	}
	return out, nil
}

// ---- detached drawings: one child process per drawing, tracked in a
// small registry so `draw stop` can end them and new drawings can keep
// their signs clear of live ones.

func drawStopCmd(args []string) {
	fs := flag.NewFlagSet("draw stop", flag.ExitOnError)
	asJSON := fs.Bool("json", false, "print how many drawings were stopped as JSON")
	_ = fs.Parse(args)
	n := windowctl.StopDrawings()
	if *asJSON {
		_ = json.NewEncoder(os.Stdout).Encode(map[string]int{"Stopped": n})
		return
	}
	fmt.Printf("stopped %d drawing(s)\n", n)
}

func drawConfigCmd(args []string) {
	fs := flag.NewFlagSet("draw config", flag.ExitOnError)
	initCfg := fs.Bool("init", false, "write a default config file")
	force := fs.Bool("force", false, "with --init: overwrite an existing file")
	_ = fs.Parse(args)
	if *initCfg {
		p, err := windowctl.InitDrawConfig(*force)
		if err != nil {
			fmt.Fprintln(os.Stderr, "windowctl draw config:", err)
			os.Exit(1)
		}
		fmt.Println("wrote", p)
		return
	}
	cfg, err := windowctl.LoadDrawConfig()
	if err != nil {
		fmt.Fprintln(os.Stderr, "windowctl draw config:", err)
		os.Exit(1)
	}
	p := windowctl.DrawConfigPath()
	if _, err := os.Stat(p); err != nil {
		fmt.Fprintf(os.Stderr, "%s does not exist (built-in defaults apply); create it with `windowctl draw config --init`\n", p)
		cfg = windowctl.DefaultDrawConfig()
	} else {
		fmt.Fprintln(os.Stderr, p)
	}
	b, _ := json.MarshalIndent(cfg, "", "  ")
	fmt.Println(string(b))
}

// stripFlags removes the named flags (and, for valued flags given as
// "--name value", their value) from a raw argument list.
func stripFlags(args []string, valued, boolean map[string]bool) []string {
	var out []string
	for i := 0; i < len(args); i++ {
		a := args[i]
		name := strings.TrimLeft(a, "-")
		if !strings.HasPrefix(a, "-") {
			out = append(out, a)
			continue
		}
		if eq := strings.IndexByte(name, '='); eq >= 0 {
			if valued[name[:eq]] || boolean[name[:eq]] {
				continue
			}
		} else if valued[name] {
			i++ // skip its value
			continue
		} else if boolean[name] {
			continue
		}
		out = append(out, a)
	}
	return out
}

func withoutDetach(args []string) []string {
	var out []string
	for _, a := range args {
		if a == "--detach" || a == "-detach" || strings.HasPrefix(a, "--detach=") || strings.HasPrefix(a, "-detach=") {
			continue
		}
		out = append(out, a)
	}
	return out
}
