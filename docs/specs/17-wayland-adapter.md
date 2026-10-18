# Spec: `wayland-adapter`

> Part of the [windowctl OpenSpec capability set](./README.md).

- **Status:** Implemented (KDE Plasma, GNOME/Mutter, sway, Hyprland)
- **Source:** §7, §7.1, §8.2, §10.3, OI-04
- **Summary:** Linux Wayland support for the window/monitor adapter via a
  runtime-dispatched, per-compositor backend architecture. KDE Plasma
  (KWin) is implemented through KWin's D-Bus scripting interface;
  GNOME (Mutter) through a resident Shell extension exposing a D-Bus
  service; sway and Hyprland (wlroots) through their native IPC CLIs
  (`swaymsg` / `hyprctl`). Design details and the verified mechanics
  are documented in the adapter package comments
  (`internal/adapter/linux/wayland/`).

## Requirement: Runtime session dispatch *(Implemented)*

### Scenario: Wayland session routes to the Wayland adapter

- **WHEN** the Linux adapter is constructed on a machine whose session
  environment declares Wayland (`XDG_SESSION_TYPE=wayland`, or
  `WAYLAND_DISPLAY` set without an X11 declaration)
- **THEN** the adapter returns the Wayland implementation
- **AND** `XDG_SESSION_TYPE=x11` (or no Wayland signal at all) keeps
  routing to the pre-existing X11 implementation with **zero behavior
  change** — the historical `wmctrl`/`xdotool` code is untouched, just
  relocated into an `x11` subpackage
- **AND** when both `DISPLAY` (XWayland) and `WAYLAND_DISPLAY` are set,
  `XDG_SESSION_TYPE` wins — XWayland alone must not flip the choice

### Scenario: Detection is environment-only

- **WHEN** the adapter is constructed (which happens at package init,
  before any command runs)
- **THEN** dispatch performs no D-Bus connection and no subprocess —
  only environment reads — so construction stays free

## Requirement: KDE Plasma backend *(Implemented)*

### Scenario: Window management through KWin scripting

- **WHEN** the Wayland adapter runs on KDE Plasma and a window
  operation (list / move / focus) is requested
- **THEN** the backend injects a JavaScript file into KWin via the
  `org.kde.kwin.Scripting` D-Bus interface, executes it, and receives
  the result through a `callDBus` callback to a session-bus name the
  windowctl process owns (per-process unique name)
- **AND** the window ID reported by `windows list` is KWin's
  `internalId` UUID string; move/focus resolve it by matching
  `String(internalId)` inside the script
- **AND** a script that fails to report back within 5 seconds produces
  a descriptive timeout error (never a silent success)
- **AND** a window that vanished between list and act produces
  `core.ErrNoMatch`, matching the darwin contract

### Scenario: Move uses whole-literal geometry assignment

- **WHEN** the backend moves a window
- **THEN** the injected script assigns `frameGeometry` a **whole
  literal object** (`{x, y, width, height}`) — mutating a read-back
  snapshot or writing single fields is a verified no-op on KWin 6
- **AND** because Wayland geometry changes are asynchronous (the
  client acknowledges the compositor's configure event), the adapter
  does not verify the result inline; the smoke test waits for the
  geometry to settle before asserting

### Scenario: Monitors and cursor position

- **WHEN** `monitors list` or `mouse position` runs on KDE
- **THEN** outputs come from `workspace.screens` with
  `devicePixelRatio` per output (logical point geometry — the same
  coordinate space as window bounds)
- **AND** `Primary` is proxied by `workspace.activeScreen` (KWin
  scripting exposes no primary-output concept); `Active`/`Focused`
  follow the same cursor / active-window-centroid semantics the
  darwin adapter stamps
- **AND** the cursor position comes from `workspace.cursorPos`

## Requirement: Screenshot capture on KDE *(Implemented)*

### Scenario: Region capture via full-desktop grab + crop

- **WHEN** `screenshot` captures a rect on KDE Plasma
- **THEN** the backend grabs the full virtual desktop with
  `spectacle -b -n -f`, crops the device-pixel region for the
  requested point rect, and — on HiDPI outputs — resamples the crop
  down to exactly the requested point size
- **AND** the written PNG upholds the global point-normalization
  invariant: 1 image pixel == 1 coordinate point (same contract as
  macOS/Windows), so an agent can read a pixel off the screenshot and
  feed it straight into `mouse click`
- **AND** missing `spectacle` produces an actionable error naming the
  tool and how to install it

## Requirement: Input injection on Wayland *(Implemented)*

### Scenario: Mouse and keyboard through ydotool

- **WHEN** `mouse move` / `mouse click` / `type` / `key` runs on
  Wayland
- **THEN** input is injected through the `ydotool` CLI (uinput virtual
  device), which is compositor-independent
- **AND** chords are rendered as evdev keycode sequences (US-layout
  mapping); keys without a keycode fail with a descriptive error
  rather than typing nothing
- **AND** missing `ydotool` (or its `ydotoold` daemon) produces an
  actionable error including the udev/input-group setup steps

## Requirement: Unsupported compositors fail clearly *(Implemented)*

### Scenario: Recognized compositor without a backend

- **WHEN** the session is Wayland on a compositor with no
  implementation (COSMIC, unrecognized)
- **THEN** backend-dependent operations fail with an error naming the
  detected compositor and which ones are implemented — never silent
  no-ops or cryptic D-Bus failures

## Requirement: sway backend *(Implemented)*

### Scenario: Window management through swaymsg IPC

- **WHEN** the Wayland adapter runs on sway and a window operation
  (list / move / focus) is requested
- **THEN** the backend shells out to `swaymsg`: `get_tree` for
  enumeration (filtering to nodes with a Wayland `app_id` or X11
  WM_CLASS — the normalWindow equivalent; swaybar never appears in
  the tree) and `[con_id=N]` criteria commands for acting on one
  window; the stable window ID is the tree node's numeric `id`
- **AND** the list is walked through each container's `focus` array
  (children in focus-recency order), so it is frontmost-first like
  the darwin/windows adapters — the public layer's Focused stamp
  depends on it; the plain nodes/floating_nodes list order is
  creation order, not stacking order (verified: the focused floating
  window is not first in floating_nodes)
- **AND** tiled windows cannot be absolutely positioned in sway
  (verified error "Only floating containers can be moved to an
  absolute position"), so Move floats the window first — changing
  the user's tiling layout, as documented
- **AND** sway's geometry commands target the container *including*
  the SSD title bar while the reported `rect` excludes it, so Move
  compensates with `deco_rect.height`
  (`resize set width W px height H+deco_h px`, then
  `move absolute position X px Y-deco_h px`, move last)
- **AND** a failed swaymsg command (exit code 2, JSON
  `{"success": false}`) surfaces verbatim; a criteria miss ("No
  matching node.") maps to `core.ErrNoMatch`, matching the darwin
  contract

### Scenario: Monitors and cursor position on sway

- **WHEN** `monitors list` runs on sway
- **THEN** outputs come from `get_outputs` with logical `rect`
  geometry, a real `primary` flag, and the focused output promoted
  to Primary when no output declares one
- **AND** `mouse position` is unavailable: sway's IPC exposes no
  cursor query at all (verified — get_tree/get_outputs/get_seats),
  so CursorPosition fails with a descriptive ErrNotImplemented

### Scenario: Cursor positioning must bypass ydotool on sway

- **WHEN** `mouse move` / `mouse click` runs on sway
- **THEN** positioning goes through sway's own
  `seat seat0 cursor set X Y` (absolute layout coordinates, seat
  "seat0" by default), and button presses go through ydotool
- **AND** ydotool's `--absolute` is NOT used as a positioning
  primitive: verified on a default sway 1.10.1 config its landing is
  non-deterministic (the same request landed inside the target
  window, in the gap before it, and past its far edge on successive
  calls — the INT32_MIN slam's residual runs through libinput
  pointer acceleration), while `cursor set` is exact (verified
  against 10px gaps and window edges)

### Scenario: Region capture via grim

- **WHEN** `screenshot` captures a rect on sway or Hyprland
- **THEN** the backend runs `grim -g "X,Y WxH"` (logical points —
  the same space as window rects) and resamples the result down to
  exactly the requested point size whenever the written PNG is
  larger (device pixels = logical × output scale; verified at
  scale 2 on Hyprland), upholding the 1 image pixel == 1 coordinate
  point invariant
- **AND** missing `grim` produces an actionable error naming the
  tool and how to install it

## Requirement: Hyprland backend *(Implemented)*

### Scenario: Window management through hyprctl

- **WHEN** the Wayland adapter runs on Hyprland and a window
  operation (list / move / focus) is requested
- **THEN** the backend shells out to `hyprctl -j clients` /
  `monitors` for queries and `hyprctl dispatch` for actions; the
  stable window ID is Hyprland's `address` string (`address:0x…`
  criteria)
- **AND** hyprctl reports dispatch failures on **stdout with exit
  code 0** (verified: "No such window found", "Invalid
  dispatcher"), so verdicts are parsed from the output text — "ok"
  means success, a criteria miss maps to `core.ErrNoMatch`
- **AND** tiled windows are floated first (Hyprland's exact
  move on a tiled client re-arranges the layout instead);
  `resizewindowpixel exact` recenters the window, so the final
  `movewindowpixel exact X Y` runs last and lands the logical box
  precisely (no title-bar compensation on Hyprland's default
  border-only decoration, verified at scale 2)
- **AND** `monitors list` divides the reported mode-pixel
  width/height by `scale` (verified: hyprctl reports physical mode
  pixels while window geometry is logical; the `physicalWidth`
  field is unreliable) with the focused monitor promoted to
  Primary when no output declares one; `Active` comes from the
  real cursor position (`hyprctl cursorpos`)

### Scenario: Cursor positioning must bypass ydotool on Hyprland

- **WHEN** `mouse move` / `mouse click` runs on Hyprland
- **THEN** positioning goes through Hyprland's own
  `hyprctl dispatch movecursor` (exact, config-independent), and
  button presses go through ydotool
- **AND** ydotool's `--absolute` is NOT used as a positioning
  primitive: it is an INT32_MIN relative slam + offset whose landing
  runs through libinput pointer acceleration — verified on a default
  Hyprland 0.55.2 config to land at 2× the request, independent of
  monitor scale, and only 1:1 with acceleration disabled
  (`force_no_accel` / `accel_profile flat`)

## Requirement: Testing constraints *(Implemented)*

### Scenario: Wayland validation is local-only

- **WHEN** the Wayland backend is validated
- **THEN** unit tests cover the environment-only pure functions
  (session detection, compositor detection, script-template
  rendering, keycode mapping) and run in the normal CI matrix
- **AND** fake-bin integration tests replay recorded IPC payloads
  through a stubbed `swaymsg`/`hyprctl`/`grim` on PATH (sway and
  Hyprland backends), exercising the full query → parse → command →
  verdict pipeline — including the verdict-to-error mapping on
  non-zero swaymsg exits — with no compositor and no sockets, so
  they run in CI
- **AND** the GNOME backend's embedded Shell extension is gated in
  CI too: `node --check` on `extension.js`, and a Go test asserting
  the `metadata.json` schema (uuid matches the install flow,
  unlock-dialog session mode, shell-version declared)
- **AND** real-window validation runs only on a developer's
  compositor session via the per-backend `task smoke-wayland*`
  scripts (launch → detect → move → settle → assert bounds →
  focus → verify → monitors → screenshot), because CI runners have
  no compositor and Docker is forbidden for window tests (§10.3)

### Test matrix

| Layer | KDE | GNOME | sway | Hyprland |
|-------|-----|-------|------|----------|
| Unit: parser fixtures (happy + malformed payloads) | ✅ script shape/escaping | ✅ report parsing | ✅ tree/outputs/verdicts | ✅ clients/monitors/cursorpos |
| Unit: detection & dispatch | ✅ session/compositor detection, unsupported error contract | | | |
| Unit: input (ydotool argv, pointerMover precedence) | ✅ shared, fake-seam | | | |
| Unit: capture (resample, grim geometry, roundtrip) | ✅ shared `wayland/capture` | | | |
| Integration: fake-bin full pipeline (CI-runnable) | ❌ (D-Bus scripting needs KWin) | ❌ (D-Bus service needs the Shell) | ✅ swaymsg/grim stubs | ✅ hyprctl/grim stubs |
| Extension gating | — | ✅ `node --check` + metadata schema | — | — |
| Real-window smoke (local/VM only) | ✅ verified KWin 6.7.4 | ✅ verified GNOME 48 | ✅ verified sway 1.10.1 | ✅ verified Hyprland 0.55.2 |

## Tasks

| Task | Purpose |
|------|---------|
| `task build` | Builds the CLI; the Wayland adapter compiles into the linux binary |
| `task test` | Runs the detection/template/keycode/fixture unit tests in CI |
| `task test:cover` | Runs all tests with coverage (45% statement floor) |
| `task smoke-wayland` | KDE real-window smoke (local-only; needs KDE Wayland + spectacle) |
| `task smoke-wayland-gnome` | GNOME real-window smoke (local or VM via `WCTL_SMOKE_SSH`) |
| `task smoke-wayland-sway` | sway real-window smoke (local or VM via `WCTL_SMOKE_SSH`) |
| `task smoke-wayland-hyprland` | Hyprland real-window smoke (local or VM via `WCTL_SMOKE_SSH`) |
| `task lint` | `go vet` + shellcheck on the smoke scripts + `node --check` on the GNOME extension |

> Smoke scripts do not run in CI (no compositor on runners, §10.3
> forbids Docker for window tests). Each backend's smoke lands with
> it; all four share the launch → list → move → settle → assert →
> focus → monitors → screenshot body (the shared build/push dispatch
> lives in `scripts/smoke-lib.sh`), and the GNOME/sway/Hyprland
> scripts support the `WCTL_SMOKE_SSH` remote-push mode used for VM
> validation. The sway/Hyprland fake-bin integration tests and the
> GNOME extension gates close the CI gap for everything that does not
> need a live compositor.
