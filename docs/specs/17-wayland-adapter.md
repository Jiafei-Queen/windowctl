# Spec: `wayland-adapter`

> Part of the [windowctl OpenSpec capability set](./README.md).

- **Status:** Partial (KDE Plasma implemented; wlroots compositors planned)
- **Source:** §7, §7.1, §8.2, §10.3, OI-04
- **Summary:** Linux Wayland support for the window/monitor adapter via a
  runtime-dispatched, per-compositor backend architecture. KDE Plasma
  (KWin) is implemented through KWin's D-Bus scripting interface;
  Sway/Hyprland (wlroots) are a planned follow-up. Design details and
  the verified KWin mechanics are documented in the adapter package
  comments (`internal/adapter/linux/wayland/`).

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
  implementation (GNOME, COSMIC, unrecognized)
- **THEN** backend-dependent operations fail with an error naming the
  detected compositor and which ones are implemented — never silent
  no-ops or cryptic D-Bus failures

### Scenario: wlroots compositors (planned)

- **WHEN** the session is Sway or Hyprland
- **THEN** the compositor is recognized but no backend exists yet; the
  same clear unsupported error applies
- **AND** the planned implementation is `swaymsg` / `hyprctl` for
  window management, `grim` for capture, `wtype` for keyboard input
  (wlroots compositor backends — not part of this slice yet)

## Requirement: Testing constraints *(Implemented)*

### Scenario: Wayland validation is local-only

- **WHEN** the Wayland backend is validated
- **THEN** unit tests cover the environment-only pure functions
  (session detection, compositor detection, script-template
  rendering, keycode mapping) and run in the normal CI matrix
- **AND** real-window validation runs only on a developer's KDE
  Wayland session via `task smoke-wayland` (launch → detect → move →
  settle → assert bounds → focus → verify via stacking order →
  monitors → screenshot), because CI runners have no compositor and
  Docker is forbidden for window tests (§10.3)

## Tasks

| Task | Purpose |
|------|---------|
| `task build` | Builds the CLI; the Wayland adapter compiles into the linux binary |
| `task test` | Runs the detection/template/keycode unit tests in CI |
| `task smoke-wayland` | KDE real-window smoke (local-only; needs KDE Wayland + spectacle) |
| `task lint` | `go vet` across the module, including the new packages |

> A wlroots smoke script (Sway/Hyprland) is not covered yet — it
> lands with the P2 backend.
