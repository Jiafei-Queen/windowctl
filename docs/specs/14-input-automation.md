# Spec 14 — input-automation

- **Status:** Implemented
- **Source:** FR-INP-01, FR-INP-02, FR-INP-03, requirements §11
- **Summary:** Synthetic mouse/keyboard input plus launch/wait, completing the agent visual loop: screenshot → (agent reads image) → click/type → screenshot to verify.

## Requirement: Mouse (FR-INP-01)

`mouse move --x --y [--monitor]`, `mouse click [--x --y] [--monitor] [--right|--middle] [--double]`, `mouse position [--json]`.

- Coordinates are monitor-relative iff `--monitor` is set (FR-MOV-02 rule); `resolvePoint` in the public package owns this — adapters only ever see global points.
- `click` without coordinates clicks where the cursor is; `--x` without `--y` (or vice versa) is a usage error.
- Multi-click pairs carry incrementing click-state (darwin `kCGMouseEventClickState`, xdotool `--repeat`) so apps see a real double-click, not two singles.

#### Scenario: click on monitor 2

- **WHEN** `mouse click --monitor 2 --x 30 --y 40` runs and monitor 2's origin is (1920, 0)
- **THEN** the adapter receives the global point (1950, 40).

## Requirement: Keyboard (FR-INP-02)

`type --text <s>` injects literal unicode (CGEventKeyboardSetUnicodeString / SendInput KEYEVENTF_UNICODE / xdotool type) — layout- and IME-independent, chunked with breathers so slow apps don't drop characters. `key --combo <s>` parses `+`-separated modifiers and one key via `ParseChord` (public package, OS-agnostic, exported) and presses it through the per-OS keycode table.

- Modifier aliases: `cmd|command|meta|super|win` → Cmd (Command on macOS, Win/Super elsewhere), `ctrl|control`, `alt|opt|option`, `shift`.
- Keys: single characters, `f1..f12`, and the named set (enter/tab/esc/space/arrows/delete/backspace/home/end/pageup/pagedown, plus normalized aliases return→enter, escape→esc, del→delete, pgup/pgdn→pageup/pagedown). Unknown modifiers/keys are parse errors — never silently dropped.
- Letters are lowercased during parsing; shift must be explicit (`cmd+shift+s`).

## Requirement: Focus guard (the wrong-window hazard)

Bare `type`/`key` send to whatever holds the keyboard **at delivery time**. During focus transitions (right after `launch`, `focus`, or a window-creating chord like cmd+n) that is frequently the *previous* window — silent input corruption of an unrelated document.

With `--title`/`--app`, `type`/`key` route through `TypeInto`/`PressKeyInto`: Focus the match, then poll the window list until the matched window reports `Focused`, then inject. If focus hasn't landed within 2s the command fails with "input NOT sent". On platforms whose adapter cannot flag a focused monitor (X11 today) verification degrades to trusting Focus(). The Wayland backends do flag it, so the guard is enforced there — which is why their window lists must be frontmost-first, or the stamp lands on a window that is not the one just focused.

#### Scenario: focus never lands

- **WHEN** `type --app Notes --text hi` runs and the Notes window never becomes the focused window within 2s
- **THEN** the command exits non-zero, the error contains "input NOT sent", and no keyboard event was posted.

## Requirement: Launch + Wait (FR-INP-03)

`launch --app <s>` hands off to the OS launcher (macOS `open -a` resolves app names; Windows `cmd /c start` resolves App Paths; linux execs a PATH binary detached) and returns without waiting. `wait (--title|--app) [--timeout <ms>]` is pure composition in the public package — a 250ms poll over `ListWindows` — and prints the first match (Monitor/Focused stamped) or errors after the timeout (default 10s).

**Caveat (by design):** `wait` matches pre-existing windows. "Wait for the NEW window" requires the caller to diff against a pre-launch snapshot — the bundled skill documents this recipe.

## Requirement: Accessibility gating (macOS)

Every synthetic-input entry point (move/click/type/chord) runs the silent AX trust check first and returns `ErrAccessibilityDenied` when untrusted — same permission, same error, same `windowctl permissions` remediation as Move/Focus (spec 11). `mouse position` and `wait` are read-only and never gated.
