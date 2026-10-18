# windowctl — Requirements Document (v0.1)

## 1. Introduction

### 1.1 Purpose

This document defines the functional and non-functional requirements for **windowctl v0.1**, a cross-platform CLI tool and Go library for programmatic desktop window management.

### 1.2 Scope

This document covers:

- CLI command structure and behavior
- Go library public API
- Zone and layout system
- Cross-platform support requirements
- Build, release, and distribution pipeline
- Testing strategy

This document explicitly excludes:

- GUI components
- Background daemon or hotkey system
- Wayland support beyond KDE Plasma, GNOME, sway, and Hyprland (other compositors are out of scope)

---

## 2. Problem Statement

### 2.1 Fragmented OS Window Management

Each major operating system provides different, incompatible mechanisms for window management:

- **Windows**: Win32 APIs
- **macOS**: Accessibility (AX) + CoreGraphics
- **Linux**: X11 / Wayland / WM-specific tools

No standard cross-platform abstraction exists.

### 2.2 Absence of a CLI-first Tool

Existing tools are either:

- GUI-focused (PowerToys, built-in OS features)
- OS-specific scripts (AutoHotKey, wmctrl, xdotool)
- Not portable across macOS, Windows, and Linux

### 2.3 No Declarative Layout System

Users cannot currently express workspace intent in a portable, declarative way (e.g., "move Chrome to monitor 2, top-right quadrant"). Manual pixel-coordinate calculations are required.

### 2.4 Automation Gap

Developers who want to script workspace setups, restore layouts, or integrate window control into terminal workflows lack portable, non-brittle tooling.

---

## 3. Goals

### 3.1 Primary Goals

| ID   | Goal                                                |
|------|-----------------------------------------------------|
| G-01 | Provide a simple CLI for managing desktop windows   |
| G-02 | Support macOS, Windows, and Linux                   |
| G-03 | Enable layout-based window placement via zones      |
| G-04 | Abstract monitor geometry for portable commands     |
| G-05 | Expose all capabilities as a Go library             |

### 3.2 Secondary Goals

| ID   | Goal                                                |
|------|-----------------------------------------------------|
| G-06 | Provide JSON output for scripting and automation    |
| G-07 | Maintain minimal external dependencies              |

### 3.3 Non-Goals

- No GUI
- No background daemon
- No hotkeys or global shortcuts
- No Wayland support beyond KDE Plasma (best-effort elsewhere)

---

## 4. Core Concepts

### 4.1 Window

Represents an OS-level application window.

| Attribute   | Type   | Description                        |
|-------------|--------|------------------------------------||
| ID          | string | OS-specific window handle          |
| Title       | string | Window title bar text              |
| App         | string | Application name                   |
| PID         | int    | Process ID                         |
| Executable  | string | Executable filename                |
| Monitor     | int    | Monitor the window is assigned to  |
| Bounds      | Rect   | x, y, width, height                |

### 4.2 Monitor

Represents a physical display.

| Attribute | Type | Description                     |
|-----------|------|---------------------------------|
| ID        | int  | Monitor identifier              |
| X, Y      | int  | Position in the global screen space |
| Width     | int  | Width in pixels                 |
| Height    | int  | Height in pixels                |
| Primary   | bool | Whether this is the primary display |

### 4.3 Zone

A zone is a logical region within a monitor used to position windows without specifying raw coordinates.

#### 4.3.1 Predefined (Enum) Zones

| Zone | Region              |
|------|---------------------|
| 1A   | Left half           |
| 1B   | Right half          |
| 2A   | Top-left quarter    |
| 2B   | Top-right quarter   |
| 2C   | Bottom-left quarter |
| 2D   | Bottom-right quarter|

#### 4.3.2 Split-based Zones

Format: `N:M` — divide the screen width into N equal columns and place the window in column M (1-indexed).

Examples:

| Zone | Meaning        |
|------|----------------|
| 2:1  | Left half      |
| 2:2  | Right half     |
| 3:1  | First third    |
| 3:2  | Middle third   |
| 3:3  | Last third     |

Calculation:

```
cellWidth      = monitorWidth / N
positionIndex  = M - 1
x              = monitor.X + (positionIndex * cellWidth)
y              = monitor.Y
width          = cellWidth
height         = monitorHeight
```

#### 4.3.3 Manual Coordinates

Users may bypass zones and specify raw coordinates:

```sh
windowctl move --title "chrome" --x 0 --y 0 --w 960 --h 1080
```

- Without `--monitor`: coordinates are absolute (global screen space)
- With `--monitor`: coordinates are relative to that monitor's origin

### 4.4 Layout

A declarative YAML file describing how multiple windows should be arranged across monitors.

---

## 5. Functional Requirements

### 5.1 CLI — Windows

#### FR-WIN-01: List Windows

- **Command**: `windowctl windows list`
- **Flags**:
  - `--title <string>` — filter by title (partial match)
  - `--app <string>` — filter by application name
  - `--json` — output as JSON array
- **Default output**: human-readable table
- **Fields shown**: ID, Title, App, PID, Monitor, Bounds

### 5.2 CLI — Monitors

#### FR-MON-01: List Monitors

- **Command**: `windowctl monitors list`
- **Default output**: human-readable table
- **Fields shown**: ID, X, Y, Width, Height, Primary

### 5.3 CLI — Move

#### FR-MOV-01: Move via Zone

- **Command**: `windowctl move --title <string> [--monitor <int>] --zone <zone>`
- **Zone formats accepted**: enum (e.g., `2B`) or split (e.g., `3:2`)
- `--monitor` is optional; if omitted, the window's current monitor is used

#### FR-MOV-02: Move via Manual Coordinates

- **Command**: `windowctl move --title <string> --x <int> --y <int> --w <int> --h <int>`
- Without `--monitor`: x/y treated as absolute global coordinates
- With `--monitor`: x/y treated as relative to the specified monitor

#### FR-MOV-03: Monitor Resolution

- When `--monitor` is not provided, resolve the current monitor as the one containing the **majority** of the window's visible area.

#### FR-MOV-04: Window Matching

- Windows may be matched by `--title` or `--app`
- At least one matcher must be provided
- If multiple windows match, the first match is used (behavior TBD for v0.1)

### 5.4 CLI — Focus

#### FR-FOC-01: Focus Window

- **Command**: `windowctl focus --title <string>`
- Brings the matched window to the foreground

### 5.5 CLI — Layout

#### FR-LAY-01: Apply Layout

- **Command**: `windowctl apply <layout.yaml>`
- Reads a YAML layout file and moves/arranges windows according to its specification

### 5.6 CLI — Skills

#### FR-SKL-01: Install Bundled Agent Skill

- **Command**: `windowctl install --skills`
- Installs the bundled `window-ctl-skill` into `~/.claude/skills/window-ctl-skill`
- If `codex` is detected on PATH, also installs the bundled skill into `~/.agents/skills/window-ctl-skill`
- `--agents` forces the `~/.agents/skills` install even when `codex` is not on PATH

#### FR-SKL-02: Uninstall Bundled Agent Skill

- **Command**: `windowctl uninstall --skills`
- Removes the bundled `window-ctl-skill` from the same target directories as install
- Missing installs are treated as a no-op, not an error

### 5.7 CLI — Desktop Automation

#### FR-SHOT-01: Screenshot

- **Command**: `windowctl screenshot [--monitor <n>] [--x --y --w --h] [--title <s> | --app <s>] [--out <path>] [--json]`
- Captures a monitor (default: the focused one, falling back to primary), an explicit region (monitor-relative when `--monitor` is set, absolute otherwise — same rule as FR-MOV-02), or a matched window's bounds, to a PNG
- The written image is **point-normalized**: 1 image pixel == 1 global coordinate point even on HiDPI/retina displays, so pixel positions read off the image translate to click coordinates as `global = capturedRect.origin + imagePixel`
- Output reports the captured rect (origin + size); `--json` shape: `{Path, X, Y, Width, Height}`
- macOS requires the Screen Recording TCC permission (separate from Accessibility); denial returns the actionable `ErrScreenCaptureDenied` message pointing at `windowctl permissions --screen`

#### FR-INP-01: Mouse Control

- **Commands**: `windowctl mouse move --x --y [--monitor <n>]`, `windowctl mouse click [--x --y] [--monitor <n>] [--right|--middle] [--double]`, `windowctl mouse position [--json]`
- Coordinates follow the FR-MOV-02 monitor-relative rule; `click` without coords clicks at the current cursor position
- Native event synthesis: CGEvent (macOS, requires Accessibility), SendInput/SetCursorPos (Windows), xdotool (Linux)

#### FR-INP-02: Keyboard Input

- **Commands**: `windowctl type --text <s> [--title <s> | --app <s>]`, `windowctl key --combo <s> [--title <s> | --app <s>]`
- `type` injects literal unicode (layout/IME-independent); `key` presses one chord parsed from `+`-separated modifiers (`cmd|command|meta|super|win`, `ctrl|control`, `alt|opt|option`, `shift`) plus a key (single char, `f1..f12`, or named: enter/tab/esc/space/arrows/delete/backspace/home/end/pageup/pagedown)
- **Focus guard**: with `--title`/`--app` the command focuses the matched window and polls until the window list reports it `Focused` before injecting anything; if focus does not land within 2s it errors with input NOT sent. Without a filter, input goes to whatever is focused (caller beware — a window created milliseconds ago may not yet own the keyboard). On platforms that cannot report per-monitor focus (linux) the guard degrades to trusting Focus()

#### FR-INP-03: Launch and Wait

- **Commands**: `windowctl launch --app <s>`, `windowctl wait (--title <s> | --app <s>) [--timeout <ms>] [--json]`
- `launch` hands off to the OS launcher (`open -a` / `cmd /c start` / direct exec) and returns immediately; `wait` polls the window list (250ms interval) until a window matches or the timeout (default 10000ms) elapses, then prints the matched window
- `wait` matches **pre-existing** windows too — callers that need "the NEW window" must snapshot the window list before launching and compare

#### FR-RMT-01: Remote Screen Share + Control

- **Command**: `windowctl remote [--monitor <n>] [--port <n>] [--fps <n>] [--tunnel]`
- Serves a browser viewer that streams a monitor as PNG frames and forwards clicks/keystrokes back for live remote control
- Binds `127.0.0.1` and gates every request on a per-run random token embedded in the printed URL (the link is the credential; Ctrl-C revokes); local URL by default, `--tunnel` additionally spawns cloudflared for a public `*.trycloudflare.com` URL
- Clicks map to true screen coordinates client-side via the FR-SHOT-01 point-normalization guarantee; needs Accessibility (control) + Screen Recording (frames) on macOS
- See `docs/specs/15-remote-control.md`

### 5.8 Output Modes

#### FR-OUT-01: Table Output (default)

- Human-readable, column-aligned output

#### FR-OUT-02: JSON Output

- Machine-readable JSON via `--json` flag
- Suitable for piping into `jq` or other tools

---

## 6. Library Requirements

### 6.1 LIB-01: Public Go API

windowctl must be importable as a Go package with the following interface:

```go
package windowctl

type Window struct {
    ID         string
    Title      string
    App        string
    PID        int
    Executable string
    Monitor    int
    Bounds     Rect
}

type Monitor struct {
    ID      int
    X, Y    int
    Width   int
    Height  int
    Primary bool
}

type Rect struct {
    X, Y int
    W, H int
}

func ListWindows(filter Filter) ([]Window, error)
func ListMonitors() ([]Monitor, error)
func Move(match Match, target Target) error
func Focus(match Match) error
```

### 6.2 LIB-02: CLI as Library Wrapper

- The CLI layer must contain no business logic
- All operations are delegated to the library
- The library interface is OS-agnostic

### 6.3 LIB-03: Importability

- The library must be usable by third-party Go tools without pulling in CLI dependencies

---

## 7. Cross-Platform Requirements

| Platform | Requirement                                                   |
|----------|---------------------------------------------------------------|
| macOS    | Use CoreGraphics for listing; Accessibility API for move/resize |
| Windows  | Use Win32 APIs (`user32.dll`) via `syscall` or `x/sys/windows` |
| Linux    | Use X11 as primary; `wmctrl`/`xdotool` as fallback           |
| Wayland  | Supported on KDE Plasma (KWin D-Bus scripting); GNOME next, then wlroots compositors |

### 7.1 CGO Strategy

- Prefer native OS APIs over external tools
- CGO is permitted where required (macOS Objective-C, Linux Xlib)
- OS-specific code must be isolated in platform adapter modules
- Clean Go interfaces must be exposed to the core layer

---

## 8. Non-Functional Requirements

### 8.1 Performance

- `windows list` must complete within 500 ms on a standard desktop
- No polling loops; use direct OS API calls

### 8.2 Security

- macOS: Accessibility permissions must be requested at runtime with a clear error if denied
- macOS: Screen Recording is a separate TCC permission gating `screenshot`; checked/requested via `windowctl permissions --screen [--status]`, same clear-error contract
- Windows: Operations requiring elevated privileges must fail gracefully with a descriptive error
- Linux: Behavior depends on display server; no special handling required beyond clear error messages
- Linux Wayland (KDE): external tools (`spectacle` for capture, `ydotool` for input) are required for the automation surface; missing tools must produce actionable errors, never silent no-ops

### 8.3 Portability

- Core logic must be unit-testable without OS dependencies (using interfaces/mocks)
- OS adapters must be thin and isolated

### 8.4 Minimal Dependencies

- No GUI toolkits
- No runtime daemons
- Prefer standard library and `x/sys` over third-party packages

---

## 9. Build & Release Requirements

### 9.1 Build Orchestration

- **Tool**: [Taskfile](https://taskfile.dev)
- Required tasks:

| Task              | Description                          |
|-------------------|--------------------------------------|
| `task build`      | Build CLI binary for current platform |
| `task test`       | Run all tests                        |
| `task lint`       | Run linter                           |
| `task snapshot`   | Build snapshot release (all platforms) |
| `task release`    | Publish release via GoReleaser       |

### 9.2 Release Automation

- **Tool**: [GoReleaser](https://goreleaser.com)
- Requirements:
  - Cross-compile for `darwin`, `windows`, `linux` (amd64 and arm64)
  - CGO enabled for macOS builds with proper linker flags (`-framework ApplicationServices`, `-framework CoreGraphics`)
  - Generate SHA-256 checksums for all binaries
  - Publish GitHub releases with binaries attached

### 9.3 npm Distribution

- CLI must be distributable via npm
- The npm package contains a thin JS wrapper that:
  1. Detects OS and architecture at runtime
  2. Downloads or invokes the correct native binary
- Must support `npx windowctl <command>`

---

## 10. Testing Requirements

### 10.1 Unit Tests

- All core logic (zone calculation, monitor resolution, window matching) must have unit tests
- Unit tests must run without OS dependencies (mock adapters)
- Must pass on all CI platforms: `ubuntu-latest`, `macos-latest`, `windows-latest`

### 10.2 CI Build Matrix

- GitHub Actions workflow must run `go build ./...` and `go test ./...` on all three platforms on every push and pull request

### 10.3 Windows Integration Tests

- Run on `windows-latest` GitHub runner
- Workflow:
  1. Build CLI binary
  2. Launch a real application (e.g., Notepad)
  3. Detect it with `windowctl windows list`
  4. Execute `move` and verify no errors
- Treated as smoke/integration tests, not full validation

### 10.4 Testing Constraints

- Docker must **not** be used for window management tests (no real window manager or GUI environment)
- GitHub runners do not support multi-monitor setups; multi-monitor behavior is not integration-tested in CI
- Timing-sensitive operations must include appropriate waits

---

## 11. Error Handling Requirements

| Scenario                        | Required Behavior                                  |
|---------------------------------|----------------------------------------------------|
| No matching window found        | Exit non-zero with descriptive message             |
| Invalid monitor ID              | Exit non-zero with descriptive message             |
| Invalid zone string             | Exit non-zero with descriptive message             |
| OS permission denied (macOS AX) | Exit non-zero, instruct user to grant Accessibility access |
| OS permission denied (Windows)  | Exit non-zero with descriptive message             |
| Layout file not found           | Exit non-zero with descriptive message             |
| Layout file invalid YAML        | Exit non-zero with parse error details             |

---

## 12. Execution Flow

```
1. Parse CLI flags and subcommand
2. Resolve window filter (title / app)
3. List all windows via OS adapter
4. Match target window(s)
5. Resolve monitor (explicit or auto-detect from majority-overlap)
6. Compute target rectangle (zone → coordinates, or pass-through manual coords)
7. Invoke OS adapter to move/resize/focus the window
8. Output result (table or JSON)
```

---

## 13. Open Items (v0.1)

| ID    | Topic                                              | Status      |
|-------|----------------------------------------------------|-------------|
| OI-01 | Behavior when multiple windows match a filter      | TBD         |
| OI-02 | Layout YAML schema definition                      | TBD         |
| OI-03 | Vertical split zone notation                       | Not planned |
| OI-04 | Wayland support scope                              | KDE implemented; GNOME next, then wlroots |
| OI-05 | npm binary download vs. bundling strategy          | TBD         |

---

## 14. Glossary

| Term        | Definition                                                         |
|-------------|--------------------------------------------------------------------|
| Zone        | A named or computed logical region within a monitor                |
| Split zone  | A zone expressed as `N:M` dividing monitor width into N columns    |
| Layout      | A YAML file declaring how multiple windows should be arranged      |
| Adapter     | OS-specific implementation of the window management interface      |
| CGO         | Go's mechanism for calling C/Objective-C code                      |
| Majority overlap | The monitor that contains more than 50% of a window's area   |

---

*Document version: 0.1 — windowctl initial specification*
