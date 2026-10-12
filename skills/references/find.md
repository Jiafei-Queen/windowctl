# find — locate text, buttons and menus inside a window

Use when you need the exact on-screen position of something you can
name: a button ("Allow"), a menu ("File"), a link, a tab, a label, or a
phrase in a document. Returns global coordinates — the same space as
`screenshot`, `mouse` and `draw` — so the result can be clicked or
pointed at directly. macOS + Windows, no admin rights.

## Commands

```sh
windowctl find --app TextEdit --text "Format"                       # best match, table
windowctl find --app TextEdit --text "Format" --json                # {Text, Label, Role, Source, Bounds, Center}
windowctl find --app Notepad --text "File" --role menuitem --json   # only menu items
windowctl find --title "Settings" --text "Allow" --exact --json     # whole label must equal it
windowctl find --app Safari --text "Sign in" --all --json           # every match, best first
windowctl find --app Preview --text "Total" --source ocr --json     # pixels only (PDFs, images, canvas)
```

## How it searches (`--source auto`, the default)

1. **Accessibility tree** — macOS AX (window + the app's menu bar),
   Windows UI Automation. Exact bounds straight from the app, works even
   when the element is partly covered. Untitled buttons fall back to
   their role description ("close button").
2. **OCR** — macOS Vision, Windows `Windows.Media.Ocr`. Both on-device.
   Used when the tree has no match (canvas apps, games, remote desktops,
   images, Electron apps with poor accessibility). The window is focused
   first so it is visible; `--no-raise` skips that.
3. A partial hit inside a big element (a text area, a web view) is
   narrowed to the exact words with OCR of just that element
   (`Source: accessibility+ocr`).

macOS OCR needs the Screen Recording grant (`windowctl permissions
--screen`). The accessibility path needs Accessibility.

## Ranking

exact label > whole word > prefix > substring; then interactive roles
(button, menuitem, link, tab, checkbox, radio) > textfield > text; then
top-to-bottom, left-to-right. `--role` filters to one of: button,
menuitem, link, tab, checkbox, radio, textfield, text, image.

## Result

```json
[{"Text":"brown fox","Label":"The quick brown fox","Role":"text",
  "Source":"ocr","Bounds":{"X":412,"Y":233,"W":88,"H":17},"Center":{"X":456,"Y":241}}]
```

`Text` is the matched span; `Label` the whole element / OCR line.
Click it: `windowctl mouse click --x <Center.X> --y <Center.Y>`.
Point at it: `windowctl draw --app <App> --find "<text>" --text "..."`
(boxes it with 8pt padding and a short arrow; `--style ring` for an oval).

## Not found

Exit **3**, and the error lists up to 8 of the closest labels seen:

```
"Allwo" not found in the accessibility tree or screen text (OCR) of window "Settings"; closest: "Allow", "Always", ...
```

Retry with one of those spellings — do not guess coordinates instead.

## Rules

- Always name the window (`--app` exact / `--title` substring); the
  first matching window is searched. Check `windows list` if unsure.
- Prefer `--role` for controls ("File" the menu, not "File" in a
  sentence).
- Menus: top-level menu bar items are found directly; items inside a
  closed menu are not on screen — open the menu first (click the
  top-level item), then find again.
- Exit codes: 0 found, 2 bad input, 3 text not found, 1 other failure
  (no window, permission denied).
- Linux: not implemented yet.
