---
title: Find on screen
description: Locate a button, menu item or label inside a window by its text — accessibility tree first, OCR as fallback.
---

```sh
windowctl find --app "System Settings" --text "Privacy"
windowctl find --app TextEdit --text "Format" --role menuitem
windowctl find --title "Save" --text "Cancel" --role button --exact --json
windowctl find --app "Google Chrome" --text "Sign in" --all
```

Returns where the text is, in the same global coordinates `mouse click` and
`draw` take — so *find it → point at it → click it* is three calls with no
pixel maths.

## How it searches

`--source auto` (the default) asks the **accessibility tree** first — the
labels an app exposes to screen readers, which are exact and cheap. When the
app exposes nothing useful (canvas apps, games, some Electron UIs) it falls
back to **OCR** on a screenshot of the window. On macOS the accessibility pass
includes the app's menu bar — the app is focused first, and the match is
reported on the monitor the window is on, so `draw --find Format` boxes the
menu bar you are looking at.

Ranking: exact label > whole word > prefix > substring, then interactive roles
(button, menu item, link, tab…) over plain text, then top to bottom. A partial
match inside a big element (a text area, a web view) is narrowed to the exact
words with OCR. Nothing found exits 3 and lists the closest labels it saw.

```sh
windowctl find --app TextEdit --text "Untitled" --source accessibility
windowctl find --app TextEdit --text "Untitled" --source ocr
```

- `--role button|menuitem|link|tab|checkbox|radio|textfield|text|image` narrows the match.
- `--exact` requires the whole label to equal the text (default: case-insensitive contains).
- `--all` prints every match, not just the best.
- `--no-raise` skips focusing the window before OCR.

## Point at it

```sh
windowctl draw --app "System Settings" --find "Privacy" --text "Click this"
```

`draw --find` runs the same search once and boxes the match tightly (8pt
padding) with a short arrow; add `--style ring` for an oval instead. `--nth`,
`--role`, `--exact` and `--source` work there too.

## Platforms

macOS: Accessibility (AX) API + Vision OCR. Windows: UI Automation + Windows
`Windows.Media.Ocr` (in-box since Windows 10). Linux: not yet.
