# draw — point at things on the human's screen

Use when the human must do something themselves (click Allow, pick a
menu item, find a field): draw a big arrow with a sign instead of
describing where to look. Click-through, never steals focus, ends by
itself, needs no admin rights or macOS permission. macOS + Windows.

## Commands

```sh
windowctl draw --at 760,500 --text "Click Allow"                  # point (blocks 8 s)
windowctl draw --rect 900,300,200,60 --style box --text "This field"
windowctl draw --app "System Settings" --text "This window"      # boxes the window
windowctl draw --mouse --style ring --text "You are here"
windowctl draw --at 400,300 --monitor 2 --text "..."              # relative to monitor 2
windowctl draw --at X,Y --text "..." --detach --json              # returns at once (300 s)
windowctl draw stop                                               # remove every detached drawing
windowctl draw --at X,Y --text "..." --preview out.png --json     # render only, show nothing
```

Point at a control by its text — no coordinates needed:

```sh
windowctl find --app "System Settings" --text "Allow" --role button --json   # bounds only
windowctl draw --app "System Settings" --find "Allow" --text "Click Allow"   # box + arrow
windowctl draw --app Notepad --find "File" --role menuitem --style ring --text "Menus"
```

`--find` reads the accessibility tree (buttons, menus, links, labels), then
OCR for anything drawn as pixels. Exit 3 = not found; the error lists the
closest labels it saw — retry with one of them.

Coordinates are the same as `screenshot` / `mouse`: read a pixel off a
screenshot (plus the capture origin) and draw at it.

## Look

`--style arrow|ring|box` · `--shape bend|straight|zigzag|spiral` ·
`--size S|M|L` · `--color red|orange|amber|yellow|green|teal|blue|purple|pink|black|white|#RRGGBB` ·
`--border shadow|white-black|black` · `--corners round|sharp` ·
`--from auto|top-left|top|top-right|left|right|bottom-left|bottom|bottom-right` ·
`--font <name|path>` · `--font-scale 1.2`.

zigzag = urgent; spiral = impossible to miss; green = "yes, this one";
several `--detach` drawings = numbered steps ("1. ...", "2. ...") — each
new sign keeps clear of the live ones.

## Defaults file

`~/.config/windowctl/fontconfig.json` (same on Windows). Create with
`windowctl draw config --init`; show with `windowctl draw config`; list
fonts with `windowctl draw fonts`. Flags override it.

## Recipe: verified pointing

1. `windowctl screenshot --json` → find the target pixel; global = origin + pixel.
2. `windowctl draw --at X,Y --text "..." --detach --json` → check `Tip` == X,Y.
3. Optional: `windowctl screenshot` again to confirm what the human sees.
4. When the human has done it: `windowctl draw stop`.

## Rules

- Always say what to do in the sign, in the human's words ("Click Allow
  so Terminal can control your Mac"), not just "here".
- Never draw over a permission or security prompt to steer the human
  toward a choice they did not ask for; explain the consequence instead.
- Clean up: `windowctl draw stop` once the step is done.
- Exit codes: 0 ok, 2 bad input, 1 failure (no window match, no monitor).
  Linux: only `--preview` works (overlay not implemented yet).
