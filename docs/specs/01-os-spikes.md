# Spec: `os-spikes`

> Part of the [windowctl OpenSpec capability set](./README.md).

- **Status:** Partial (macOS Supported, Windows Supported, Linux X11
  best-effort, KDE Plasma Wayland Supported, other Wayland
  compositors unsupported)
- **Source:** §7, §7.1, §10.3
- **Summary:** Per-OS tracer bullet proving that the chosen native API on
  each supported platform can enumerate windows and monitors, move/resize a
  real window, and bring it to the foreground — end-to-end through a thin
  platform adapter behind a clean Go interface.

This spec is the foundation every later capability depends on. It is not a
user-facing CLI command on its own; it is the per-OS proof that the adapter
contract is real.

## Requirement: macOS adapter spike *(Implemented)*

### Scenario: Enumerate windows and monitors on macOS

- **WHEN** the macOS adapter is built (CGO enabled, `-framework
  CoreGraphics -framework CoreFoundation -framework
  ApplicationServices -framework AppKit`)
- **THEN** `ListWindows` enumerates real windows via
  `CGWindowListCopyWindowInfo` and `ListMonitors` enumerates displays
  via `CGGetActiveDisplayList`, exposed through the same Go interface
  used by the other platforms
- **AND** a plain `windowctl windows list` does NOT trigger the
  Accessibility permission prompt (no AX call happens during list
  paths or at adapter construction)

### Scenario: Move and Focus on macOS

- **WHEN** the macOS adapter is asked to move or focus a window
- **THEN** the adapter first re-fetches the CG window entry by ID via
  `CGWindowListCopyWindowInfo` (no adapter-level cache of any prior
  `ListWindows` result) to obtain the owner PID and current bounds at
  call time
- **AND** it walks `AXUIElementCreateApplication(pid)` →
  `kAXWindowsAttribute` and disambiguates against the CG entry by
  matching `kAXTitleAttribute` and the AX position+size against the
  CG bounds (with a small pixel tolerance to account for window-shadow
  geometry differences between CG and AX)
- **AND** for `Move` it calls `AXUIElementSetAttributeValue` for
  `kAXPositionAttribute` and `kAXSizeAttribute` against the resolved
  AX window
- **AND** for `Focus` it calls `AXUIElementPerformAction` with
  `kAXRaiseAction` on the resolved AX window and then calls
  `[NSRunningApplication activateWithOptions:]` on the owning app to
  bring the application's process to the foreground
- **AND** Public `Window.ID` stays a `CGWindowID` decimal string —
  the `core.Adapter` interface shape is unchanged

### Scenario: Accessibility permission denied on macOS

- **WHEN** the macOS adapter `Move` or `Focus` is invoked and the
  current process is not trusted by Accessibility
  (`AXIsProcessTrustedWithOptions` returns false)
- **THEN** the operation exits non-zero with the
  `core.ErrAccessibilityDenied` sentinel, whose message instructs the
  user to grant Accessibility access in System Settings → Privacy &
  Security → Accessibility (per requirements §11)
- **AND** the AX permission prompt is only triggered by `Move` /
  `Focus` calls — never during adapter construction or `ListWindows`
  / `ListMonitors`

### Scenario: Window vanished between list and move on macOS

- **WHEN** `Move` or `Focus` is called with a window ID whose CG
  entry can no longer be found (window was closed or app quit
  between the list and the action)
- **THEN** the operation exits non-zero with `core.ErrNoMatch` and a
  message that mentions the window may have been closed

### Scenario: Title collision on macOS (FR-MOV-04 first-match)

- **WHEN** the same application has two windows with identical title
  AND identical bounds
- **THEN** `Move` and `Focus` resolve to the first AX window that
  matches both attributes during the AX walk, consistent with the
  FR-MOV-04 first-match rule (the lookup is deterministic per
  `AXUIElementCopyAttributeValue(kAXWindowsAttribute)` ordering)

## Requirement: Windows adapter spike

### Scenario: Enumerate, move, and focus a real window on Windows

- **WHEN** the Windows adapter is built against `user32.dll` (via `syscall`
  or `x/sys/windows`)
- **THEN** it can list windows, and move/resize/focus the matched window
  using Win32 APIs
- **AND** the operations are exposed through the shared Go interface

### Scenario: Operation requires elevated privileges on Windows

- **WHEN** a Windows operation cannot be completed at the current privilege
  level
- **THEN** it fails gracefully with a descriptive error rather than
  crashing

## Requirement: Linux adapter spike

### Scenario: Enumerate, move, and focus a real window under X11

- **WHEN** the Linux adapter runs under an X11 session
- **THEN** it can list windows and move/focus the matched window using X11
  as the primary backend, with `wmctrl` / `xdotool` available as fallback

### Scenario: Wayland session — KDE Plasma

- **WHEN** the Linux adapter detects a Wayland session with KWin
- **THEN** the adapter delegates to the KDE backend: window management
  goes through KWin's D-Bus scripting interface (script injection via
  `org.kde.kwin.Scripting` with a `callDBus` callback to a bus name the
  process owns), screenshots through `spectacle`, and input through
  `ydotool`
- **AND** the dispatch happens at runtime based on
  `XDG_SESSION_TYPE` / `WAYLAND_DISPLAY`, with the X11 path unchanged
  for X11 sessions
- **AND** missing external tools produce actionable errors naming the
  tool and how to install it (per §8.2), never silent no-ops

### Scenario: Wayland session — other compositors

- **WHEN** the Linux adapter detects a Wayland session on a compositor
  without an implementation (GNOME and the wlroots family are
  recognized; GNOME is the next planned backend, per the roadmap in
  docs/wayland/design.md §8)
- **THEN** backend-dependent operations fail with a descriptive error
  naming the detected compositor and which ones are implemented,
  rather than silent no-ops or cryptic D-Bus failures

## Requirement: Adapter isolation

### Scenario: Core layer has no OS-specific code

- **WHEN** the core/business-logic packages are inspected
- **THEN** they contain no Win32, Cocoa, or Xlib references
- **AND** all OS calls go through a Go interface implemented by the
  platform adapter modules

## Tasks

| Task | Purpose |
|------|---------|
| `task build` | Builds the per-OS CLI binary that exercises the platform adapter |
| `task test` | Runs the OS-mockable core unit tests |

> Per-OS adapter validation against real windows runs in
> [`ci-test-matrix`](./10-ci-test-matrix.md), not via a standalone
> Taskfile target.
