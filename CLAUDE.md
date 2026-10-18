# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project

`windowctl` is a cross-platform CLI **and** Go library for window/monitor management on macOS, Windows, and Linux. The CLI is a thin shell over the importable Go package — never put business logic in `cmd/windowctl`.

Authoritative specs live in `docs/requirements.md` and `docs/specs/` (OpenSpec-style slices indexed by `docs/specs/README.md`). When adding or changing behavior, locate the matching slice (FR-WIN-01, FR-MOV-03, etc.) and keep code, tests, and the spec aligned.

## Common commands

Uses [Taskfile](https://taskfile.dev):

```sh
task build              # go build -o bin/windowctl ./cmd/windowctl
task test               # go test ./...
task lint               # go vet ./...
task local-install      # build + symlink bin/windowctl → /usr/local/bin/windowctl (each `task build` is then live)
task local-uninstall    # remove the dev symlink (only if it points at this repo)
task snapshot           # goreleaser release --snapshot --skip=publish; stages binaries into npm/platforms/<os-arch>/bin/
task release:dry-run    # same as snapshot, with --verbose for goreleaser config debugging
task npm:bump -- 0.2.0  # sync version across main + 5 platform sub-packages
task npm:publish        # npm publish all 6 packages non-interactively (binaries must already be staged)
task release -- 0.2.0   # full local release: bump → snapshot+stage → publish 6 packages → commit → tag → push → GH release
```

Single test: `go test ./... -run TestApplyFilterByTitleIsPartialAndCaseInsensitive`. Tests are in the repo root package (`windowctl`) and `cmd/windowctl`; the per-OS adapter packages have no unit tests — they're validated via the smoke scripts in CI.

Real-window smoke tests (run automatically by `.github/workflows/ci.yaml` via `task smoke-darwin` / `task smoke-windows` on the matching runner; the workflow installs `task` via `arduino/setup-task@v2`):

- `task smoke-darwin` — builds CLI, launches TextEdit via LaunchServices (`open -a`), polls `windowctl windows list` until detected (10s budget), then runs `windowctl move` and auto-detects the AX state from the actual outcome — exit 0 means trust is inherited (asserts bounds within `WCTL_AX_TOLERANCE`, default 50px to absorb title-bar/chrome variance), exit !=0 with "Accessibility permission denied" means a fresh runner (asserts the denied error). `WCTL_SMOKE_AX=1` is an opt-in strict mode that REQUIRES the granted path, useful for catching TCC regressions on a known-good local setup.
- `task smoke-darwin-apps` — per-app Move/Focus matrix on macOS for real third-party apps (Chrome, VSCode, etc.); override the set with `APPS="Google Chrome"`. Local-only, not part of CI.
- `task smoke-windows` — builds CLI, launches Notepad, verifies `windows list` detects it and `move` succeeds.
- `task smoke-wayland` — KDE Wayland real-window smoke (local-only): launches `konsole -e sleep`, detects it, moves (settle-then-assert within `WCTL_WAYLAND_TOLERANCE`, default 50px), focuses (verified via stacking order — the target must be topmost), lists monitors, and screenshots a region asserting the point-normalized size. Not in CI — runners have no compositor (spec 10 forbids Docker for window tests). Requires a KDE Plasma Wayland session and `spectacle` installed.

Module is Go 1.24, dependencies `golang.org/x/sys`, `golang.org/x/image` (font rasterization + HiDPI image scaling), and `github.com/godbus/dbus/v5` (KWin scripting callback channel — the KDE Wayland backend; pure Go, no CGO, no transitive deps).

## Architecture

Three layers, with build-tag-isolated platform code:

1. **`cmd/windowctl/`** — flag parsing, table/JSON formatting. Calls only the public package. No OS code, no business logic.
2. **Public package `github.com/muthuishere/windowctl`** (root `*.go` files) — `ListWindows`, `ListMonitors`, `Move`, `MoveZone`, `MoveCoords`, `Focus`, `Resize`, `Batch`, `Screenshot`, `MouseMove`, `MouseClick`, `CursorPosition`, `TypeText`, `TypeInto`, `PressKey`, `PressKeyInto`, `ParseChord`, `Launch`, `WaitForWindow`, `RequestAccessibility`, `CheckAccessibility`, `RequestScreenCapture`, `CheckScreenCapture`, `InstallBundledSkill`, `UninstallBundledSkill`, `Draw`, `RenderDraw`. Re-exports `core` types. Holds shared logic that must remain OS-agnostic and unit-testable: filter matching (`applyFilter`), monitor auto-resolution (`resolveCurrentMonitor` — majority-overlap per FR-MOV-03), zone parsing (`zone.go`), and the relative-vs-absolute coord rule for `MoveCoords` (relative when `--monitor` is set). `ListWindows` also stamps `Window.Monitor` (1-indexed monitor ID under the centroid; 0 if off-screen) and `Window.Focused` (frontmost-in-z-order on the focused monitor) on the **unfiltered** list before applying the filter — so those fields stay consistent regardless of what the caller filters out (BUG-1 / BUG-9 in `bugs.md`). `Batch` (`windowctl.go`) applies a slice of `BatchEntry` (title/app match + either a zone or full x/y/w/h + optional monitor) sequentially via the same `moveZoneWith`/`moveCoordsWith` helpers `Move`/`MoveZone`/`MoveCoords` use, never aborting on a single entry's failure — the returned `[]BatchResult` is one-to-one with the input so callers can inspect per-entry `Err`. The CLI's `windowctl batch` (`cmd/windowctl/main.go`) reads a JSON array of entries from stdin or `--file` and calls `Batch` directly.
3. **`internal/core`** — OS-agnostic `Window`/`Monitor`/`Rect`/`Filter`/`Match`/`Target` types and the `Adapter` interface. Exists *only* to break the import cycle between the public package and the per-OS adapters; nothing else should live here.
4. **`internal/adapter/{darwin,linux,windows}`** — one `Adapter` implementation per OS. Selected at link time by `adapter_darwin.go` / `adapter_linux.go` / `adapter_windows.go` (each with the matching `//go:build` tag) which call `newPlatformAdapter()`. On linux the adapter is itself a runtime dispatcher: `internal/adapter/linux/New()` reads the session environment and returns either `x11/` (the historical wmctrl/xdotool code) or `wayland/` (per-compositor backends; KDE Plasma, GNOME, sway, and Hyprland implemented). See [docs/specs/17-wayland-adapter.md](../docs/specs/17-wayland-adapter.md).

The adapter interface stays small but now also carries the desktop-automation primitives (`CaptureRect`, `MouseMove`, `MouseClick`, `CursorPosition`, `TypeText`, `PressChord`, `Launch`, plus `CheckScreenCapture`/`RequestScreenCapture`). Anything that can be expressed as composition of those primitives belongs in the public package, not the adapters — e.g. `Screenshot` (monitor/window/region → resolved `Rect` → `CaptureRect`), the monitor-relative point rule for mouse coords, chord *parsing* (`ParseChord`), `WaitForWindow` (a poll over `ListWindows`), and the focus-verify guard (`TypeInto`/`PressKeyInto`) all live in root `*.go`, never in an adapter. Adapters only ever receive fully-resolved global points and a parsed `core.Chord`. `RequestAccessibility` is a no-op on linux/windows and the only AX-**prompting** call site on darwin — surfaced through `windowctl permissions`. `CheckAccessibility` is its read-only sibling: it inspects current AX trust without ever triggering the system dialog, and powers the script-friendly `windowctl permissions --status` (no-op returning true on linux/windows).

### Per-OS notes

- **`windowctl remote`** (`cmd/windowctl/remote.go` + `remote_viewer.go`): a transport layer, not new library logic — a localhost HTTP server that streams `Screenshot` PNGs (origin in headers) and forwards `/input` actions to `MouseClick`/`TypeText`/`PressKey`, plus an embedded single-page viewer that maps image-pixel clicks to global points via the point-normalization guarantee. Per-run random token gates every endpoint (the printed URL is the credential); local-only by default, `--tunnel` spawns cloudflared for a public URL. Deliberately uses the *unguarded* input calls — the human watching the stream is the focus authority. See `docs/specs/15-remote-control.md`.
- **`windowctl draw`** (`draw*.go` in root; `overlay.go` per adapter): big arrow + text sign (+ ring/box) over every window, ported from bigarrow (MIT). All placement, geometry and pixels are pure Go in root (`draw_geometry.go` placement/arrow/spiral, `draw_raster.go` rasterizer + text + shadow, `draw_style.go` palette/metrics, `draw_config.go` `~/.config/windowctl/fontconfig.json`); adapters only add `OverlayScale` + `ShowOverlay` (darwin: non-activating NSPanel via `objc_msgSend`, main thread — the darwin package `init` locks the main goroutine; windows: layered `WS_EX_TRANSPARENT|NOACTIVATE|TOPMOST` window via `UpdateLayeredWindow`; linux: `ErrNotImplemented`, `--preview` still works). No admin, no macOS permission. `--detach` / `draw stop` process bookkeeping lives in `cmd/windowctl/draw*.go`. See `docs/specs/16-draw.md`.
- **Desktop automation** (`screenshot.go` + `input.go` in root; `automation.go` per adapter): the visual-loop surface. macOS is pure CGO/no-subprocess — `CaptureRect` uses `CGWindowListCreateImage` → optional `CGBitmapContext` downscale → ImageIO PNG (the API is obsoleted in the macOS 15 SDK; `automation.go` builds with `-mmacosx-version-min=12.0` in CFLAGS to downgrade the error to a warning — verified working on macOS 26; ScreenCaptureKit is the eventual replacement if the symbol is pulled). Input is CGEvent posted to `kCGHIDEventTap` (mouse via `CGEventCreateMouseEvent` with `kCGMouseEventClickState` for double-click, text via `CGEventKeyboardSetUnicodeString`, chords via an ANSI keycode table). Windows is direct Win32: GDI `BitBlt` into a top-down DIB for capture, `SendInput`/`SetCursorPos` for input. Linux shells out (`import`/`scrot`, `xdotool`) matching the wmctrl style. **The load-bearing invariant is point-normalization: every capture is resampled so 1 image pixel == 1 global coordinate point (retina-safe), which is what lets an agent read a pixel off the screenshot and click it.** macOS input reuses AX trust (silent check, `ErrAccessibilityDenied`); `screenshot` needs the *separate* Screen Recording grant (`ErrScreenCaptureDenied`, surfaced via `windowctl permissions --screen`). See `docs/specs/13-screenshot.md` + `14-input-automation.md`. **Focus guard:** `type`/`key` with a `--title`/`--app` filter route through `TypeInto`/`PressKeyInto`, which focus-then-poll until the target reports `Focused` before injecting — the safeguard against keystrokes landing in the previously-focused window (a real data-corruption hazard when typing right after launch/focus).
- **darwin** (`internal/adapter/darwin/adapter.go`): CGO against CoreGraphics + CoreFoundation + ApplicationServices + AppKit. ListWindows / ListMonitors use CG; Move / Focus / RequestAccessibility use the Accessibility (AX) API via `AXUIElementSetAttributeValue` (position+size sandwich for cross-display moves) and `AXUIElementPerformAction(kAXRaiseAction)` + `NSRunningApplication activateWithOptions:` for focus (the latter reached via `objc_msgSend` so the file stays pure C — no `.m` or Swift). All CFRetain/CFRelease and AX lifetime is owned by the C helpers (`wctl_collect_windows`, `wctl_collect_monitors`, `wctl_ax_check`, `wctl_ax_request`, `wctl_window_for_id`, `wctl_ax_resolve`, `wctl_ax_set_bounds`, `wctl_ax_focus`); Go only iterates POD struct arrays via `unsafe.Slice`. The CGWindowID → AXUIElementRef bridge is a per-call AX walk under the owner PID, disambiguated by title + bounds (no caching, no `_AXUIElementGetWindow` private SPI). `wctl_collect_windows` only emits windows with `kCGWindowLayer == 0` (normal application windows), so menubar/Dock/Control-Center/AltTab chrome never reaches `windows list` — anything on a higher CG layer is skipped in the C helper itself, not filtered in Go. Move/Focus return `core.ErrAccessibilityDenied` when AX permission is missing — `windowctl permissions` is the discoverable opt-in for granting it. AX prompt only fires from `windowctl permissions` or the lazy guard inside Move/Focus, never from `windows list`.
- **windows** (`internal/adapter/windows/adapter.go`): `user32.dll` via `golang.org/x/sys/windows`. `EnumWindows` callback for window listing; `ListMonitors` enumerates every attached display via `EnumDisplayMonitors` + `GetMonitorInfoW`, assigning sequential IDs (0..N-1) and setting `Primary` from `MONITORINFOF_PRIMARY`.
- **linux** (`internal/adapter/linux/adapter.go`): runtime session dispatcher — `XDG_SESSION_TYPE=wayland` (or `WAYLAND_DISPLAY` without an X11 declaration; `XDG_SESSION_TYPE` wins over XWayland's `DISPLAY`) routes to the Wayland adapter, everything else to `x11/` (the historical wmctrl/xdotool/xrandr shell-outs, unchanged). **wayland** (`internal/adapter/linux/wayland/`): dispatches per compositor; the KDE backend (`wayland/kde/`) drives KWin through JS scripts injected via `org.kde.kwin.Scripting` with results returned over a `callDBus` callback to a per-process bus name (needs the `godbus/dbus` dependency — gdbus/dbus-send can't own bus names). Window IDs are KWin `internalId` UUID strings; `frameGeometry` must be assigned a whole literal object (verified KWin 6 behavior). Screenshots go through `spectacle -b -n -f` + Go-side crop/downscale (KWin 6 removed its Screenshot D-Bus interface); input through `ydotool` (uinput, shared across compositors, needs the ydotoold daemon + udev setup). Point normalization (1 image pixel == 1 coordinate point) is maintained by downscaling the device-pixel crop by each output's `devicePixelRatio`. Missing external tools fail with actionable errors. GNOME (`wayland/gnome/`, resident Shell extension exposing a D-Bus service), sway (`wayland/sway/`, swaymsg IPC), and Hyprland (`wayland/hyprland/`, hyprctl; screenshots via the shared `wayland/capture` grim pipeline) are implemented alongside KDE; unrecognized compositors fall through to the clear unsupported error. Local-only validation: `task smoke-wayland*` (one script per backend). See [docs/specs/17-wayland-adapter.md](../docs/specs/17-wayland-adapter.md).

### Build & release

`.goreleaser.yaml` has two build entries: `windowctl-nondarwin` (linux/windows, `CGO_ENABLED=0`, ignores `windows/arm64`) and `windowctl-darwin` (`CGO_ENABLED=1`). Cross-compiling to darwin from a non-darwin host requires a darwin C toolchain (osxcross); on macOS the system clang handles it. Each build entry has a per-build post hook (`scripts/stage-npm-binary.sh`) that copies the freshly-built binary into the matching `npm/platforms/<os>-<arch>/bin/` directory.

The npm distribution uses the multi-package `optionalDependencies` pattern: `@muthuishere/windowctl` is a thin JS launcher (`npm/bin/windowctl.js`) that `require.resolve`s the matching `@muthuishere/windowctl-<os>-<arch>` sub-package and execs the native binary. No postinstall, no GitHub Releases dependency at install time. Five sub-packages: darwin-{arm64,x64}, linux-{arm64,x64}, windows-x64. Each has `os`+`cpu` fields so npm only installs the matching one.

Local release flow (no GitHub Actions involved): `task release -- <version>` runs the full pipeline — `node scripts/bump-npm-version.js` syncs versions across all 6 `package.json` files, `goreleaser release --snapshot --clean --skip=publish` cross-compiles + stages binaries via the post hooks, `scripts/publish-npm-local.sh` publishes all 6 packages non-interactively (`npm publish --access public` per package; assumes the configured token bypasses 2FA), then commit + tag + push + `gh release create`. See `docs/specs/09-release-pipeline.md`.

### Bundled agent skill (`windowctl install/uninstall --skills`)

`skills/` is a standalone, self-contained agent-skill package (`SKILL.md` + `references/` + its own `Taskfile.yml`) that teaches an LLM agent (Claude Code, Codex, etc.) how to drive `windowctl` in natural language — it never touches Quartz/Win32/X11/wmctrl directly, only shells out to `windowctl`. `skill_install.go` embeds that whole tree via `//go:embed skills` and copies it (write-to-temp-dir + atomic rename, not symlink) to `~/.claude/skills/window-ctl-skill` and, when `--agents` is passed or `codex` is on `PATH`, also to `~/.agents/skills/window-ctl-skill` (`CLAUDE_SKILLS_DIR` / `AGENTS_SKILLS_DIR` env vars override the roots). `cmd/windowctl/skills.go` wires this up as `windowctl install --skills [--agents]` / `windowctl uninstall --skills [--agents]` (FR-SKL-01/02, `docs/specs/12-skills-subcommand.md`). For editing the skill itself, `skills/Taskfile.yml` has its own `local:install` that **symlinks** the working tree instead (live-reload of `references/*.md` without reinstalling) — don't confuse it with the embedded-copy install the CLI does for end users.

## Conventions

- `--title` is a case-insensitive substring match; `--app` is case-insensitive **exact** match. Don't change one without the other (covered by `adapter_test.go`).
- When multiple windows match a filter, the first match is used (FR-MOV-04 / OI-01 in `docs/requirements.md` — TBD for v0.1; flag any change as a spec change).
- Errors that flow to the user must be descriptive enough to act on (see §11 of `docs/requirements.md`); use `core.ErrNotImplemented` / `core.ErrNoMatch` rather than ad-hoc strings where they apply.
- CI runs on `ubuntu-latest`, `macos-latest`, `windows-latest`. Anything that won't compile/test on all three breaks the build — keep OS-specific code behind build tags.

<!-- ctx-optimize:begin -->
<ctx-optimize>
  <precondition>Run `command -v ctx-optimize` first. If it is NOT installed, IGNORE this entire
  block and answer by reading the code normally — the store is an optimization, not a requirement
  (install later with `npm install -g @muthuishere/ctx-optimize`, or download the binary). Everything
  below applies ONLY when the command exists.</precondition>
  <store>MULTI-MODULE repo, pre-built knowledge store at `~/ctxoptimize/windowctl/` — one graph per module + a navigator, 7 modules declared in `.ctxoptimize/config.json`.</store>
  <use>Use it INSTEAD of grep-and-read chains — PICK BY INTENT: find → `ctx-optimize query "<terms>"` ·
  inspect a symbol → `card <symbol>` · about to EDIT → `change-plan <symbol>` (callers+impact+tests, one
  call) · blast radius → `affected <symbol>` · connection → `path <a> <b>` ·
  list/filter (no jq): `nodes --kind K` / `edges --relation R` / `deps --scope dev`.
  Scope follows your cwd: a module dir answers from that module (zero hits escalate repo-wide); the root
  federates via the navigator (`~/ctxoptimize/windowctl/navigator.md`; `--modules all|a,b` widens).
  Output is parsed fact with exact file:line — cite it directly, do NOT re-verify in source.
  Exhaustive literal-string sweeps stay grep's job.</use>
  <deep-doc>The FULL usage card — verify discipline, store-vs-grep ladder, sources (databases/
  buckets/queues/APIs by env-var name), remote push/pull, `up` — is committed at
  `.ctxoptimize/instructions.md`. Read it before deeper store work.</deep-doc>
  <no-local-store>Fresh clone with nothing at `~/ctxoptimize/windowctl/`? Run `ctx-optimize up` —
  it pulls the team's prebuilt store when the config declares one, otherwise rebuilds every module store in seconds.</no-local-store>
</ctx-optimize>
<!-- ctx-optimize:end -->
