package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"

	"github.com/Jiafei-Queen/windowctl"
)

// findCmd: `windowctl find` — locate text (a button, menu item, link,
// label, or OCR'd screen text) inside a window and print its bounds.
func findCmd(args []string) {
	fs := flag.NewFlagSet("find", flag.ExitOnError)
	title := fs.String("title", "", "window to search, by title (case-insensitive substring)")
	app := fs.String("app", "", "window to search, by app name (case-insensitive)")
	text := fs.String("text", "", "text to find (case-insensitive; required)")
	source := fs.String("source", "", "auto (accessibility, then OCR) | accessibility | ocr")
	role := fs.String("role", "", "button | menuitem | link | tab | checkbox | radio | textfield | text | image")
	exact := fs.Bool("exact", false, "the whole label must equal the text")
	all := fs.Bool("all", false, "print every match, not just the best")
	noRaise := fs.Bool("no-raise", false, "do not focus the window before OCR")
	asJSON := fs.Bool("json", false, "print matches as JSON")
	_ = fs.Parse(args)
	if err := rejectEmptyFilterFlags(fs, title, app); err != nil {
		fmt.Fprintln(os.Stderr, "windowctl find:", err)
		os.Exit(2)
	}
	hits, err := windowctl.FindText(windowctl.FindOptions{
		Match: windowctl.Match{Title: *title, App: *app}, Text: *text,
		Source: windowctl.FindSource(*source), Role: *role, Exact: *exact, NoRaise: *noRaise,
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, "windowctl find:", err)
		if errors.Is(err, windowctl.ErrNoMatch) {
			os.Exit(3)
		}
		os.Exit(1)
	}
	if !*all {
		hits = hits[:1]
	}
	if *asJSON {
		_ = json.NewEncoder(os.Stdout).Encode(hits)
		return
	}
	fmt.Printf("%-4s %-14s %-10s %-22s %s\n", "#", "SOURCE", "ROLE", "BOUNDS", "TEXT")
	for i, h := range hits {
		b := h.Bounds
		fmt.Printf("%-4d %-14s %-10s %-22s %s\n", i+1, h.Source, h.Role, fmt.Sprintf("%dx%d+%d+%d", b.W, b.H, b.X, b.Y), h.Text)
	}
}
