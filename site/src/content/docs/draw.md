---
title: Point at things — draw
description: A big arrow with a text sign (or a ring / box) over every window, so an agent can show a human where to click.
---

```sh
windowctl draw --at 700,500 --text "Click here"
windowctl draw --app TextEdit --text "This window"                  # boxes the window
windowctl draw --app "System Settings" --find "Privacy" --text "Open this"
windowctl draw --mouse --text "You are here" --color blue
```

An agent that can only *describe* where to click ("the third button in the
second toolbar") is slow to follow. `draw` paints a **big arrow and a sign**
over everything — click-through, never takes focus, gone after a few seconds.
The look is ported from [bigarrow](https://github.com/franzenzenhofer/big-arrow-on-the-screen)
(MIT, macOS-only Swift); windowctl's version is plain Go on macOS and Windows.

## Targets (exactly one)

| Flag | Points at |
|---|---|
| `--at X,Y` | a point — the arrow tip lands exactly on it |
| `--rect X,Y,W,H` | a rectangle (boxed) |
| `--title T` / `--app A` | a window's bounds (boxed) |
| `--title/--app` + `--find TEXT` | a button / menu / label inside the window (ring) |
| `--mouse` | the cursor |

`--monitor N` makes `--at` / `--rect` relative to monitor N — the same
coordinate model as `screenshot` and `mouse`, so a point read off a screenshot
can be pointed at directly.

## Look

```sh
windowctl draw --at 700,500 --text "Ring"  --style ring
windowctl draw --at 700,500 --text "Boxed" --style box
windowctl draw --at 700,500 --text "Zig"   --shape zigzag --color purple
windowctl draw --at 700,500 --text "Spin"  --shape spiral --size L
windowctl draw --at 700,500 --text "Left"  --from left --border white-black --corners sharp
```

- `--style arrow|ring|box` · `--shape bend|straight|zigzag|spiral` · `--size S|M|L`
- `--color` red (default) · orange · amber · yellow · green · teal · blue · purple · pink · black · white · or `#RRGGBB`
- `--border shadow|white-black|black` · `--corners round|sharp`
- `--from auto|top-left|top|top-right|left|right|bottom-left|bottom|bottom-right` — a preference; it falls back to the roomiest side
- `--font <name or path>` (`windowctl draw fonts` lists them) · `--font-scale` · `--text-color` · `--border-color` · `--edge-color`

Colours are tuned so sign text reaches WCAG AA contrast where the hue allows.

## Lifetime

```sh
windowctl draw --at 700,500 --text "Wait" --duration 3   # blocks 3 s (default 8)
windowctl draw --at 700,500 --text "Stays" --detach      # returns now, stays up to 300 s
windowctl draw stop                                      # remove every detached drawing
```

A new drawing places its sign clear of the ones already on screen.

## Check before showing

```sh
windowctl draw --at 700,500 --text "Click here" --preview /tmp/arrow.png --json
```

`--preview` renders the exact same pixels to a PNG without showing anything;
`--json` prints where the tip and sign landed. Works on Linux too, which is how
the rendering is tested in CI.

## Your defaults: `~/.config/windowctl/fontconfig.json`

```sh
windowctl draw config --init     # write the default file
windowctl draw config            # print the effective config
```

Keys: `font`, `fontScale`, `color`, `textColor`, `borderColor`, `edgeColor`,
`size`, `style`, `shape`, `border`, `corners`, `from`. A flag beats the config,
the config beats the built-in. `--no-config` ignores the file. Same path on
macOS and Windows.

## Platforms

macOS: non-activating `NSPanel` above everything. Windows: a layered,
click-through, topmost window. No admin rights, no macOS permission. Linux:
`--preview` only.
