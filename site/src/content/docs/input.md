---
title: Keyboard & mouse
description: mouse, type, key, launch and wait — with a focus guard so keystrokes never land in the wrong window.
---

## Mouse

```sh
windowctl mouse move  --x 960 --y 540
windowctl mouse click --x 812 --y 344                 # left click
windowctl mouse click --x 812 --y 344 --double
windowctl mouse click --x 812 --y 344 --right         # or --middle
windowctl mouse click --monitor 2 --x 100 --y 100     # relative to monitor 2
windowctl mouse click                                 # click where the cursor is
windowctl mouse position --json
```

## Typing and keys

```sh
windowctl type --text "hello world"                     # into whatever has focus
windowctl type --app TextEdit --text "meeting notes"    # focus-guarded
windowctl key  --combo "cmd+shift+s"
windowctl key  --title "jira" --combo "ctrl+c"
windowctl key  --combo enter
```

### The focus guard

With `--title` or `--app`, `type` and `key` **focus the window, then poll
until it actually reports focused**, and only then inject. Without the guard,
typing right after a launch or a focus can land in the window that *was*
frontmost a moment ago — your terminal, your editor. That is a data-corruption
hazard, so the guarded form is the one to script.

## Launch and wait

```sh
windowctl launch --app TextEdit           # macOS app name · Windows name/path · Linux binary
windowctl wait   --app TextEdit           # default timeout 10000 ms
windowctl wait   --title "Untitled" --timeout 5000 --json
```

`wait` polls `windows list` until a match appears, then prints it — so
`launch` → `wait` → `type --app` is a race-free sequence.

## Backends

macOS posts CGEvents to the HID tap (needs Accessibility). Windows uses
`SendInput` / `SetCursorPos`. Linux uses `xdotool`.
