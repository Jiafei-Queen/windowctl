---
name: window-ctl-skill
---

# Automation — screenshot, mouse, keyboard, launch/wait

Covers the desktop-automation surface that lets an agent *see* the
screen and *act* on it: `screenshot`, `mouse`, `type`, `key`,
`launch`, `wait`. Together these close the **visual loop**:

> `screenshot` → (you read the PNG) → `mouse click` / `type` → `screenshot` again to confirm.

Everything here is a single native `windowctl` invocation (CGO /
CoreGraphics on macOS, Win32 on Windows, xdotool/ImageMagick on Linux).
The skill never touches Quartz, Win32, or X11 directly.

---

## The point-normalization guarantee (why the loop works)

`windowctl screenshot` writes a PNG whose **pixel grid equals the
global coordinate grid** — 1 image pixel == 1 coordinate point, even
on a retina/HiDPI display. The command reports the captured rect's
origin. So to click something you spotted in the image:

```
globalX = capturedRect.X + pixelXInImage
globalY = capturedRect.Y + pixelYInImage
```

Then `windowctl mouse click --x globalX --y globalY`. No scale factor,
ever. This is the single most important fact in this file.

---

## screenshot

```sh
windowctl screenshot [--out <path>] [--json] \
  ( --monitor <n> | --x <n> --y <n> --w <n> --h <n> | --title <s> | --app <s> )
```

- **No target** → the FOCUSED monitor (where the user is working),
  falling back to primary. Use this for "screenshot my screen".
- `--monitor <n>` → that whole display.
- `--x --y --w --h` → a region. Monitor-relative when `--monitor` is
  also given, absolute virtual-desktop points otherwise (same rule as
  `move`).
- `--title` / `--app` → the matched window's bounds (discover first,
  see Core Rules — exactly one window must match).
- `--out` defaults to `screenshot-<unixtime>.png` in the cwd.

`--json` shape (feed the origin back into `mouse`):
```json
{"Path":"shot.png","X":1920,"Y":0,"Width":2560,"Height":1440}
```

**macOS permission:** capture needs the **Screen Recording** TCC grant,
which is SEPARATE from Accessibility. If a capture returns
`Screen Recording permission denied`, route the user to
`windowctl permissions --screen` (check silently with
`windowctl permissions --screen --status`). See `permissions.md`.

---

## mouse

```sh
windowctl mouse move  --x <n> --y <n> [--monitor <n>]
windowctl mouse click [--x <n> --y <n>] [--monitor <n>] [--right|--middle] [--double]
windowctl mouse position [--json]
```

- Coords follow the monitor-relative rule (relative iff `--monitor`).
- `click` with no `--x/--y` clicks at the CURRENT cursor position.
- `--double` for double-click; `--right` / `--middle` for the other
  buttons.
- `mouse position` reports the cursor's global point — handy to record
  a spot the user points at, or to restore the cursor after acting.

**macOS:** mouse + keyboard synthesis needs Accessibility (same grant
as `move`/`focus`). A `mouse`/`type`/`key` call returning
`Accessibility permission denied` → `windowctl permissions`.

---

## type and key — and the focus guard

```sh
windowctl type --text "hello world" [--title <s> | --app <s>]
windowctl key  --combo "cmd+shift+s" [--title <s> | --app <s>]
```

`type` injects literal text (unicode, IME-safe). `key` presses ONE
chord: `+`-separated modifiers then a key.

- Modifiers: `cmd` (= Command on mac, Win/Super elsewhere — aliases
  `command`/`meta`/`super`/`win`), `ctrl`, `alt` (aliases `opt`/
  `option`), `shift`.
- Keys: any single character, `f1`..`f12`, or a named key: `enter`,
  `tab`, `esc`, `space`, `up`/`down`/`left`/`right`, `delete`,
  `backspace`, `home`, `end`, `pageup`, `pagedown`.
- Shift must be explicit: `cmd+shift+s`, not `cmd+S`.

### ALWAYS pass `--title`/`--app` when you know the target window.

Without a filter, `type`/`key` hit whatever holds the keyboard *at the
instant the event is delivered*. Right after a `launch`, a `focus`, or
a window-spawning chord like `cmd+n`, that is very often the PREVIOUS
window — so your keystrokes silently corrupt an unrelated document.

With `--title`/`--app`, the command focuses the match, **verifies**
the window actually became focused (polls up to 2s), and only then
types. If focus never lands it errors with *"input NOT sent"* instead
of typing into the wrong place. Treat the filtered form as the default
and the bare form as the rare exception (e.g. you JUST clicked into a
field yourself).

---

## launch and wait

```sh
windowctl launch --app "Google Chrome"
windowctl wait (--title <s> | --app <s>) [--timeout <ms>] [--json]
```

`launch` starts/foregrounds an app and returns immediately — it does
NOT wait for a window. `wait` polls until a window matches (default
timeout 10000ms).

**Standard "open then act" recipe:**
```sh
windowctl launch --app TextEdit
windowctl wait --app TextEdit --timeout 8000 --json   # blocks until the window exists
windowctl type --app TextEdit --text "now safe to type"
```

**Gotcha — `wait` matches PRE-EXISTING windows too.** If the app is
already running with a window, `wait` returns instantly on the old
one. When you specifically need the NEW window: snapshot
`windows list --json` before `launch`, then diff IDs after, instead of
relying on `wait`.

---

## Putting it together — a full visual loop

```sh
# 1. See the current screen
windowctl screenshot --out /tmp/before.png --json
#    → {"Path":"/tmp/before.png","X":0,"Y":0,"Width":1920,"Height":1080}

# 2. (agent reads /tmp/before.png, finds a button at pixel 840,470)

# 3. Click it — origin was (0,0) so global == pixel here
windowctl mouse click --x 840 --y 470

# 4. Type into the field that opened, targeting the window by name
windowctl type --app "Google Chrome" --text "search query"
windowctl key --combo enter --app "Google Chrome"

# 5. Confirm
windowctl screenshot --app "Google Chrome" --out /tmp/after.png
```

---

## Open a terminal and run a command in it

When the human should SEE a command run (a demo, a long build they will
watch, a REPL, an interactive tool). If you only need the output, run the
command yourself instead — windowctl cannot read text back except via
`screenshot` / `find --source ocr`.

```sh
# macOS (Terminal; Ghostty / iTerm / "Visual Studio Code" work the same by app name)
windowctl launch --app Terminal
windowctl wait   --app Terminal                       # --timeout is milliseconds
windowctl key    --app Terminal --combo cmd+n         # optional: fresh window (launch reuses a running app)
windowctl type   --app Terminal --text 'npm test'
windowctl key    --app Terminal --combo enter

# Windows
windowctl launch --app WindowsTerminal                # or cmd / powershell
windowctl wait   --app WindowsTerminal
windowctl type   --app WindowsTerminal --text 'dir'
windowctl key    --app WindowsTerminal --combo enter
```

Check it ran: `windowctl find --app Terminal --text "<expected word>" --source ocr`
(exit 0 = on screen) or `windowctl screenshot --app Terminal`.

Rules:
- Always pass `--app` to `type`/`key` — the focus guard stops keystrokes
  landing in the wrong window.
- `launch` reuses an existing window, which may sit in some other
  directory: `cd` first (`--text 'cd ~/proj && npm test'`) or open a fresh
  window with `cmd+n` (macOS) / `ctrl+shift+n` (Windows Terminal).
- Never type secrets; never run destructive commands the human did not ask for.

## remote — hand the live screen + control to a browser

```sh
windowctl remote [--monitor <n>] [--port <n>] [--fps <n>] [--tunnel]
```

Starts a local web server and prints a URL (with an access token baked
in) that opens a viewer: it streams PNG frames of the chosen monitor
and forwards clicks / double-clicks / right-clicks / keystrokes back to
the machine — full remote control from any browser on the LAN. Ctrl-C
revokes the link and stops everything.

- The URL **is** the key: it carries a per-run random token; requests
  without it get 403. Share it like a house key, Ctrl-C to revoke.
- The stream is live **MJPEG** (continuous, ~10fps default), not a
  slideshow — `--fps` (1–30) and the `?q=` quality knob tune it.
- Default binds the **LAN IP** so devices on the same network reach it
  directly. `--tunnel` also exposes a public `*.trycloudflare.com` URL
  via cloudflared (must be installed).

> **⚠️ Confirmation gate — ALWAYS ask before a `--tunnel` (public)
> stream.** A cloudflare tunnel puts full mouse + keyboard control of
> the machine behind a public URL that anyone who gets the link can
> drive. Before running `windowctl remote --tunnel`, state plainly that
> this exposes desktop control on a public internet URL and get an
> explicit "yes" from the user. The LAN-only form (no `--tunnel`) does
> **not** need this confirmation. This gate holds even when the user has
> a standing "always use cloudflare" preference — confirm the specific
> act of going live each time, then proceed.
- `--monitor` picks the shared display (default: focused); the viewer
  also has a monitor dropdown. `--fps` caps the poll rate (1–10,
  default 2).
- The viewer maps a click on the image back to the true screen point
  using the frame's origin + the point-normalization guarantee, so
  clicks land where you'd expect even on a scaled/retina display.
- On this path the human watching IS the focus guard — input goes to
  whatever is focused on screen, which is what a live operator wants.

Use it for "let me drive this machine from my phone/laptop", pairing,
or handing a session to someone. It needs the same macOS grants as the
rest of automation (Accessibility for control, Screen Recording for the
frames).

## Cross-platform notes (Parallels / multi-OS)

The same six commands exist on macOS, Windows, and Linux. Differences
to expect:

- **App names in `launch`:** macOS takes the display name (`"Google
  Chrome"`); Windows takes a name/path `start` can resolve; Linux
  takes a binary on `PATH` (`google-chrome`).
- **Permissions:** the macOS Accessibility + Screen Recording grants
  have no equivalent on Windows/Linux — `permissions` reports "not
  required" there.
- **`key` punctuation** is mapped by US-layout keycode position; on a
  non-US layout the produced glyph for punctuation chords may differ
  (letters, digits, named keys are stable).
- **Linux** needs `xdotool` (input) and `import`/`scrot` (capture)
  installed, and is X11-first (Wayland best-effort), mirroring the
  existing `move`/`focus` story.
