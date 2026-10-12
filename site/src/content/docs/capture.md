---
title: Screenshots
description: Capture a monitor, a window, or a region to PNG — point-normalized so pixels are clickable coordinates.
---

```sh
windowctl screenshot --out shot.png                         # the focused monitor
windowctl screenshot --monitor 2 --out external.png         # a whole display
windowctl screenshot --app "Google Chrome" --out chrome.png # one window's bounds
windowctl screenshot --title "jira" --out jira.png          # by title substring
windowctl screenshot --x 0 --y 0 --w 800 --h 600 --out region.png            # absolute region
windowctl screenshot --monitor 2 --x 100 --y 100 --w 400 --h 300 --out r.png # region relative to monitor 2
windowctl screenshot --app TextEdit --json                  # path + captured rect as JSON
```

Without `--out` the file is named `screenshot-<timestamp>.png`.

## Pixels are points

The image is resampled so one pixel equals one global coordinate point, even
on a Retina display. With `--json` you get the capture's origin, so

```
click point = origin + pixel position in the PNG
```

and `windowctl mouse click --x … --y …` lands exactly where you looked.

## Backends

- **macOS** — `CGWindowListCreateImage` → optional downscale → ImageIO PNG, all
  in-process (no `screencapture` subprocess). Needs the **Screen Recording**
  permission: `windowctl permissions --screen`.
- **Windows** — GDI `BitBlt` into a top-down DIB.
- **Linux** — shells out to `import` (ImageMagick) or `scrot`.
