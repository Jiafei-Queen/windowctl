---
title: Go library
description: The CLI is a thin shell over an importable package.
---

```go
import windowctl "github.com/muthuishere/windowctl"
```

```go
chrome := windowctl.Match{App: "Google Chrome"}
two := 2

ws, err := windowctl.ListWindows(windowctl.Filter{App: "Google Chrome"})
ms, err := windowctl.ListMonitors()

err = windowctl.MoveZone(chrome, &two, "2B")
err = windowctl.MoveCoords(windowctl.Match{Title: "Terminal"}, nil, windowctl.Rect{X: 0, Y: 0, W: 960, H: 1080})
err = windowctl.Resize(windowctl.Match{App: "Code"}, 900, 700)
err = windowctl.Focus(windowctl.Match{Title: "jira"})
results := windowctl.Batch(entries) // one BatchResult per entry

rect, err := windowctl.Screenshot(windowctl.ScreenshotOptions{Match: &chrome, OutPath: "chrome.png"})
err = windowctl.TypeInto(windowctl.Match{App: "TextEdit"}, "hello") // focus-guarded
err = windowctl.PressKey("cmd+shift+s")
w, err := windowctl.WaitForWindow(windowctl.Filter{App: "TextEdit"}, 10000)

hits, err := windowctl.FindText(windowctl.FindOptions{Match: chrome, Text: "Sign in"})
res, err := windowctl.Draw(windowctl.DrawOptions{Text: "Click here", Point: &windowctl.Point{X: 700, Y: 500}})

ok := windowctl.CheckAccessibility()   // never prompts
err = windowctl.RequestAccessibility() // prompts
```

Everything the CLI does, the library does — the command is argument parsing and
output formatting over these calls, nothing more. If you are writing Go, import
it; do not shell out to a binary you then have to ship and locate.

## Where the logic lives

The per-OS layer implements a small set of primitives: list windows, list
monitors, move, focus, permission checks, capture, mouse/keyboard injection,
launch, and show-overlay. Everything expressible as a composition of
those — zone maths, N:M splits, majority-overlap monitor resolution, filter
matching, the relative-vs-absolute coordinate rule — lives in the portable
package. So the semantics are identical on macOS, Windows and Linux, and they
are unit tested once rather than three times.

## Module

Go 1.24. Dependencies: `golang.org/x/sys` and `golang.org/x/image` (draw's
text rendering). macOS builds use CGO against the system frameworks; Linux and
Windows builds are CGO-free.
