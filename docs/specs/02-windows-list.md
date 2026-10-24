# Spec: `windows-list`

> Part of the [windowctl OpenSpec capability set](./README.md).

- **Status:** Implemented
- **Source:** FR-WIN-01, FR-OUT-01, FR-OUT-02, §11
- **Summary:** List currently open windows, optionally filtered by title,
  application, desktop scope, or minimized state, in either a
  human-readable table or JSON.

## Requirement: List all windows

### Scenario: Default table output

- **WHEN** the user runs `windowctl windows list` with no flags
- **THEN** all currently open windows — on every virtual desktop /
  workspace / Space, minimized included — are printed as a
  column-aligned table
- **AND** the columns include `ID`, `Title`, `App`, `PID`, `Monitor`,
  `Bounds`

## Requirement: Desktop scope

The Go API's `Filter` carries an explicit desktop scope; the CLI's
default listing spans all desktops.

### Scenario: Current-desktop scope (Go API)

- **WHEN** a library caller invokes `ListWindows(Filter{...,
  CurrentDesktopOnly: true})`
- **THEN** only windows on the current virtual desktop /
  workspace / Space are returned
- **AND** windows pinned on all desktops (sticky / onAllDesktops /
  pinned) count as current and are always included

### Scenario: Platform capability differences

- **WHEN** the desktop scope is evaluated per platform
- **THEN** the narrowing is decided inside the platform adapter, where
  desktop membership knowledge lives, and the public layer applies
  Title / App text matching uniformly afterwards
- **AND** macOS uses the CG on-screen enumeration (current Space),
  Windows the DWM cloak reason (CLOAKED_SHELL = another desktop), KDE
  KWin's desktop membership, GNOME the extension-reported workspace
  index, Hyprland the active workspace id, sway the focused workspace,
  and X11 the `wmctrl` desktop column
- **AND** an adapter that cannot honor a scope degrades to the
  all-desktops behavior rather than failing

### Scenario: X11 App identity

- **WHEN** the X11 adapter reports a window's `App`
- **THEN** it is the class component of `WM_CLASS` (via
  `wmctrl -lx`), matching the identity the other adapters report
  (KWin's resourceClass, GNOME's wm_class, Windows' executable base
  name) — not the client machine hostname

## Requirement: Skip minimized (Go API)

### Scenario: SkipMinimized filter

- **WHEN** a library caller invokes `ListWindows(Filter{...,
  SkipMinimized: true})`
- **THEN** minimized windows are dropped, best-effort by platform:
  honored on macOS, Windows, KDE, GNOME, and Hyprland
- **AND** on X11 and sway the flag is a documented no-op (X11 would
  need one `xprop` call per window; sway has no minimized concept)

## Requirement: Filter by title

### Scenario: Partial title match

- **WHEN** the user runs `windowctl windows list --title chrome`
- **THEN** only windows whose title contains the substring `chrome` are
  returned
- **AND** the match is case-insensitive (per README example using lowercase
  `chrome`)

## Requirement: Filter by application

### Scenario: Filter by app name

- **WHEN** the user runs `windowctl windows list --app Firefox`
- **THEN** only windows belonging to the `Firefox` application are returned

## Requirement: JSON output

### Scenario: `--json` flag emits machine-readable output

- **WHEN** the user runs `windowctl windows list --json`
- **THEN** output is a JSON array of window objects suitable for piping
  into `jq` or other tools
- **AND** each object exposes the same fields as the table view

## Requirement: Performance

### Scenario: Listing returns quickly on a standard desktop

- **WHEN** `windowctl windows list` is invoked on a standard desktop
- **THEN** the command completes within 500 ms (per §8.1)
- **AND** no polling loop is involved — the OS API is queried directly

## Tasks

| Task | Purpose |
|------|---------|
| `task build` | Builds the CLI binary that exposes `windowctl windows list` |
| `task test` | Runs unit tests for filter parsing and output formatting |
