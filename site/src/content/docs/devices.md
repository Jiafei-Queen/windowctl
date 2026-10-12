---
title: Desktop automation
description: Screenshot, click, type, press keys, launch apps and wait for windows — the visual loop an agent needs.
---

Moving windows is half of it. The same binary also **looks** at the screen and
**acts** on it, so an agent (or a script) can run a full look → act → confirm
loop without any other tool.

```sh
windowctl launch --app TextEdit
windowctl wait --app TextEdit --timeout 10000
windowctl screenshot --app TextEdit --out /tmp/look.png         # look
windowctl type --app TextEdit --text "hello from windowctl"     # act (focus-guarded)
windowctl key  --app TextEdit --combo "cmd+a"
windowctl screenshot --app TextEdit --out /tmp/confirm.png      # confirm
```

## The one invariant: 1 pixel = 1 point

Every capture is resampled so **one image pixel equals one global screen
coordinate** — on Retina too. Read a pixel position off the PNG, add the
capture's origin, and that is the exact point to pass to `mouse click`. That is
what makes a screenshot actionable instead of decorative.

## Pages

- [Screenshots](/windowctl/capture/) — monitor, window or region to PNG.
- [Keyboard & mouse](/windowctl/input/) — `mouse`, `type`, `key`, `launch`, `wait`, and the focus guard.
- [Point at things](/windowctl/draw/) — draw a big arrow + sign on top of everything.
- [Find on screen](/windowctl/find/) — locate a button or label by text.
- [Remote control](/windowctl/remote/) — share the screen to a browser and drive it.

## Platform support

| Command | macOS | Windows | Linux |
|---|---|---|---|
| `screenshot` | CGWindowList, pure CGO | GDI `BitBlt` | `import` / `scrot` |
| `mouse`, `type`, `key` | CGEvent | `SendInput` | `xdotool` |
| `launch`, `wait` | yes | yes | yes |
| `draw` overlay | yes | yes | `--preview` only |
| `find` | AX tree + Vision OCR | UI Automation + Windows.Media.Ocr | not yet |
| `remote` | yes | yes | yes (via screenshot/xdotool) |

On macOS, input reuses the Accessibility grant; `screenshot` needs the separate
**Screen Recording** grant — see [Permissions](/windowctl/permissions/).
