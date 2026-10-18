---
name: window-ctl-skill
description: >
  Manage desktop windows and monitors on macOS, Windows, and Linux. List
  open windows, list monitors (with which one is ACTIVE / has the
  FOCUSED window), move a window into a predefined zone (1A, 1B,
  2A..2D), an N:M split, or absolute / monitor-relative coordinates,
  resize a window in place, focus a window, **bulk-apply a layout of
  many windows in one call (`windowctl batch`)**, **take a screenshot
  of any monitor / region / window and drive the mouse and keyboard
  to automate the UI (the visual loop: screenshot → click → type)**,
  and request the macOS Accessibility + Screen Recording permissions
  needed for move / resize / focus / batch / mouse / type /
  screenshot. Trigger on: list windows, show open windows, what
  windows do I have, list monitors, show displays, which monitor is
  active, where's my cursor, which monitor has focus, move chrome to
  the left half, snap terminal to the right half, put slack on
  monitor 2, place editor in the top-right quarter, three-way / N-way
  split, send to external display, resize chrome to 900x700, make
  this window smaller, focus jira, raise window, bring app to front,
  apply my layout, save my arrangement, restore work mode, batch
  place windows, take a screenshot, screenshot this monitor / window
  / region, capture the screen, move the mouse, click at x y,
  double-click, right-click, type this text, press cmd+shift+s / hit
  enter, send keystrokes, launch an app, open an app and wait for its
  window, wait for a window to appear, automate a UI / click-through,
  find a button / menu / text on screen, where is the Allow button,
  locate "File" menu, point at it, draw an arrow, show me where to
  click, highlight this, circle it, box it, numbered steps on screen,
  open a terminal and run a command, run npm test in a terminal,
  share / stream my screen, remote control,
  request / check accessibility or screen recording permission, AX
  permission, grant screen control, debug windowctl AX bridge. Uses
  `windowctl` (`npm install -g @muthuishere/windowctl`).
---

<!-- version: 0.2.0 -->

# window-ctl-skill

A natural-language front door for the [`windowctl`](https://github.com/muthuishere/windowctl)
cross-platform window manager CLI. The skill owns intent routing and
recipe knowledge; `windowctl` owns the OS-specific window operations.

## Core Rules

- **`windowctl` is the engine. This skill never reaches into Quartz, Win32,
  X11, or wmctrl directly.** Every operation is a single `windowctl`
  invocation. If a recipe cannot be expressed as one, it does not belong
  in this skill.
- **Always discover before acting.** Before any `move` / `focus` / `resize`,
  run `windowctl windows list --json` to confirm exactly one window
  matches the user's filter. `--title` is a case-insensitive substring;
  `--app` is a case-insensitive exact match. Empty strings are
  rejected (`--title cannot be empty`). When zero match, surface what
  was actually open and ask. When many match, list them with IDs +
  bounds and ask which — the CLI silently picks the first match,
  which is rarely what the user wants.
- **Monitors are 1-indexed by ascending (X, Y) origin.** The leftmost
  display is always `1`. IDs from `monitors list` and the per-window
  `Monitor` field share the same numbering and are stable across
  reboots / re-plugs. Use `monitors list --json` to map heuristics
  ("the laptop", "the external", "the active one", "where my cursor
  is") to an integer ID, then pass `--monitor <id>`. Never hardcode.
  `--monitor 0` is invalid (off-screen sentinel for windows).
- **Prefer the `Focused` field for "this window" intents.**
  `windows list --json` exposes a per-window `Focused: true` flag;
  `monitors list --json` exposes per-monitor `Active` (cursor) and
  `Focused` (frontmost-window centroid). For "snap THIS window" /
  "what's on my current screen" requests, read these instead of
  guessing — see `references/windows.md` WIN-L-6 and
  `references/monitors.md` MON-R-4 / MON-R-5.
- **Zone vs coords are mutually exclusive.** `--zone` and
  `--x/--y/--w/--h` cannot be combined. `--w` and `--h` must be
  `> 0` in coord mode; both `move` and `resize` enforce this.
- **`--monitor` changes coord interpretation.** With `--monitor`,
  raw `--x/--y` are RELATIVE to that monitor's origin. Without
  `--monitor`, they are ABSOLUTE in the global virtual desktop.
  Get this wrong and the window lands off-screen.
- **OS clamps surface as exit 1 with details.** When the OS or app
  refuses the requested geometry (Chrome's minimum size, sandbox
  restrictions, fullscreen-mode windows), `move` / `resize` exits
  non-zero with a message like
  `windowctl: requested 512x640 at (3840,30), OS clamped to 617x616 ...
  (likely a minimum-window-size constraint)`.
  Treat this as soft-failure: the window did move, but not where
  asked. Surface the message verbatim and stop — do NOT auto-retry.
  Note: the (X, Y) in that message can be wrong (BUG-13 upstream);
  trust the Width/Height and re-read with `windows list` for the
  authoritative position.
- **macOS needs Accessibility once per parent process.** On the
  first failed `move` / `resize` / `focus` / `mouse` / `type` /
  `key` with `Accessibility permission denied`, route to
  `references/permissions.md`. The only command that triggers the
  AX prompt is `windowctl permissions`; everything else returns the
  denied error without prompting. Re-run the original command after
  grant.
- **`screenshot` needs a SECOND macOS grant: Screen Recording.**
  It is separate from Accessibility. On `Screen Recording
  permission denied`, route to `windowctl permissions --screen`
  (check silently with `--screen --status`). See
  `references/automation.md` and `references/permissions.md`.
- **Automating the UI? Read `references/automation.md`.** The visual
  loop (screenshot → read pixels → click/type) hinges on the
  point-normalization guarantee (1 image pixel == 1 click point) and
  the focus guard (always pass `--title`/`--app` to `type`/`key` so
  keystrokes can't land in the wrong window).
- **A public (`--tunnel`) remote stream ALWAYS needs explicit
  confirmation.** `windowctl remote --tunnel` puts full mouse +
  keyboard control behind a public `*.trycloudflare.com` URL that
  anyone with the link can drive. Before running it, say plainly that
  it exposes desktop control on the public internet and get an explicit
  "yes" — every time, even under a standing "always use cloudflare"
  preference. The LAN-only form (no `--tunnel`) needs no confirmation.
  See `references/automation.md`.
- **`WCTL_AX_DEBUG=1` is the macOS triage knob.** When `move` /
  `focus` reports `window <id> is gone from the AX tree`, re-run
  with `WCTL_AX_DEBUG=1` and surface the per-PID AX dump from
  stderr — that names the match rule (title / geometry /
  single-window) the bridge tried and failed on.
- **Linux: X11 is best-effort, Wayland goes through compositor
  backends.** Under X11 the adapter shells out to `wmctrl` + `xrandr`;
  under Wayland it dispatches per compositor — KDE Plasma, GNOME
  (installs its Shell extension on first use, one relogin), sway, and
  Hyprland give exact geometry, anything else fails with a clear
  unsupported error. Don't promise pixel-exact placement on X11.
- **`windows list` is now filtered server-side.** Only real
  application windows (macOS `kCGWindowLayer == 0`) are returned —
  ~5–15 entries in a typical session. Menubar widgets, the Dock,
  Spotlight, Control Center, AltTab, status items are excluded by
  the CLI. The full list usually fits inline; group by app rather
  than truncating.
- **Prefer `batch` for any multi-window placement in one turn.**
  When the user names two-or-more placements at once ("split chrome
  left and slack right, terminal on monitor 2"), build a JSON
  layout array and pipe to `windowctl batch` (one call, one
  rendered summary, partial-failure tolerant) rather than firing
  N sequential `move` calls. See `references/batch.md`. Note: the
  `batch` JSON uses **lowercase** keys (`app`, `title`, `monitor`,
  `zone`, `x/y/w/h`) — different from the PascalCase returned by
  `windows list --json` and `monitors list --json`. Anything that
  pipes one into the other has to translate.
- **Stop after each successful action.** Don't auto-chain ("moved
  Chrome — would you like me to also move Slack?"). Wait for the
  next instruction.

## Session Context

Held in conversation memory only — no file writes.

```
monitors:        cached `windowctl monitors list --json` for the session
                 (refresh if user mentions plugging/unplugging a display)
last_match:      the {id, title, app} of the most recently moved/focused
                 window — useful for "move it to the other monitor"
                 follow-ups
```

If the session ends, the skill re-lists.

## Process

1. Confirm `windowctl` is on PATH. Missing → `npm install -g
   @muthuishere/windowctl`. STOP if missing; do not attempt fallbacks.
   (Every `windowctl` run keeps this skill installed and current in
   `~/.claude/skills` and `~/.agents/skills` — no `install --skills`
   needed; `WINDOWCTL_NO_SKILL_AUTOINSTALL=1` turns that off.)
2. Identify the intent and load the matching family file:
   - List windows → `references/windows.md`
   - List monitors (incl. ACTIVE / FOCUSED) → `references/monitors.md`
   - Move a window (zone, split, coords) → `references/move.md`
   - Resize a window in place → `references/resize.md`
   - Bulk-place / save / restore a layout (`windowctl batch`) → `references/batch.md`
   - Focus / raise a window → `references/focus.md`
   - Screenshot / mouse / type / key / launch / wait (automate the UI, the visual loop; open a terminal and run a command) → `references/automation.md`
   - Locate a button / menu / link / phrase by its text (`windowctl find`) → `references/find.md`
   - Point at something on the human's screen (arrow + sign, ring, box; `windowctl draw`, `draw --find`) → `references/draw.md`
   - macOS Accessibility prompts + AX-bridge debugging → `references/permissions.md`
   - Composite layouts ("split chrome + slack 50/50") → `references/recipes.md`
3. Need a zone refresher? → `references/zones.md` (cheatsheet for 1A..2D
   and N:M).
4. Dispatch the documented `windowctl` command verbatim. Render per the
   recipe's "User-visible formatting" block.
5. Stop and wait.

## Self-check

Before trusting the skill's routing, validate format integrity:

    sh tests/all.sh

Runs:
  - tests/validate.sh        recipe format + cross-references
  - tests/install-test.sh    install/uninstall idempotency

Zero-exit means the catalogue is internally consistent and installable.

## Families at a glance

| Family | Reference |
|---|---|
| Windows (list, filter, Focused field) | `references/windows.md` |
| Monitors (list, ACTIVE / FOCUSED resolution) | `references/monitors.md` |
| Move (zone / split / coords / multi-monitor / clamp handling) | `references/move.md` |
| Resize (in-place width/height change) | `references/resize.md` |
| Batch (bulk-apply / save / restore layouts) | `references/batch.md` |
| Focus (raise + activate, "this window" via Focused) | `references/focus.md` |
| Automation (screenshot, mouse, type, key, launch, wait — the visual loop) | `references/automation.md` |
| Find (button / menu / text → exact bounds; AX/UIA then OCR) | `references/find.md` |
| Draw (arrow + sign, box, ring over every window; `--find`, steps, `draw stop`) | `references/draw.md` |
| Permissions (macOS Accessibility + Screen Recording, `WCTL_AX_DEBUG`) | `references/permissions.md` |
| Zones (1A..2D + N:M cheatsheet) | `references/zones.md` |
| Composite layouts | `references/recipes.md` |
