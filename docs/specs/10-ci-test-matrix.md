# Spec: `ci-test-matrix`

> Part of the [windowctl OpenSpec capability set](./README.md).

- **Status:** Partial — three-OS build & test matrix is live and green;
  Windows and macOS real-window smoke tests are wired into the workflow
  via the `task smoke-windows` / `task smoke-darwin` targets in
  `Taskfile.yml` (which delegate to `scripts/smoke-windows.ps1` and
  `scripts/smoke-darwin.sh` respectively); the Linux real-window smoke
  tests (`task smoke-wayland*`) need a live compositor, so they stay
  local-only.
- **Source:** §10.1, §10.2, §10.3, §10.4
- **Summary:** GitHub Actions runs `go build ./...`, `go vet ./...`, and
  `go test ./...` on Ubuntu, macOS, and Windows on every push/PR, plus
  Ubuntu-only gates (`go test -race`, ShellCheck over `scripts/*.sh`,
  `node --check` on the GNOME Shell extension), with Windows and macOS
  real-window smoke tests as follow-up workflow steps. No Docker is used
  for window tests.

## Requirement: Build & test on three OSes

### Scenario: Workflow runs on every push and PR

- **WHEN** a push or pull request lands
- **THEN** `go vet ./...`, `go build ./...`, and `go test ./...` run on
  `ubuntu-latest`, `macos-latest`, and `windows-latest` (per
  `.github/workflows/ci.yaml`)
- **AND** the Ubuntu job additionally runs the tests under `-race` and
  gates the Linux-only surfaces: ShellCheck over `scripts/*.sh` and
  `node --check` on the embedded GNOME Shell extension
- **AND** all three jobs must pass for the workflow to succeed

## Requirement: Windows integration smoke test *(Implemented)*

### Scenario: Real-window smoke test on Windows runner

- **WHEN** the workflow runs on `windows-latest`
- **THEN** it builds the CLI, launches a real application (Notepad),
  detects it via `windowctl windows list --json`, executes a `move`,
  and verifies no errors are returned
- **AND** the step is wired in `.github/workflows/ci.yaml` as
  `task smoke-windows` (gated on `matrix.os == 'windows-latest'`),
  with the Task runner installed via `arduino/setup-task@v2`

## Requirement: macOS integration smoke test *(Implemented)*

### Scenario: Real-window smoke test on macOS runner (auto-detect)

- **WHEN** the workflow runs on `macos-latest` (or the script runs
  on a developer's local Mac)
- **THEN** the step builds the CLI, launches TextEdit via
  LaunchServices (`open -a TextEdit`), polls
  `windowctl windows list --json` until TextEdit is detected (10s
  budget, fail loud on timeout), runs `windowctl move`, and
  branches on the actual outcome:
  - move exit 0 → assert TextEdit's post-move bounds match the
    requested rectangle within `WCTL_AX_TOLERANCE` pixels (default
    50, to absorb the macOS title-bar / shadow / chrome-vs-content
    geometry skew)
  - move exit non-zero AND stderr contains "Accessibility permission
    denied" → assert that's the case and pass (matches the macOS
    adapter contract in [`01-os-spikes.md`](./01-os-spikes.md))
  - any other outcome → fail
- **AND** the step is wired in `.github/workflows/ci.yaml` as
  `task smoke-darwin` (gated on `matrix.os == 'macos-latest'`),
  with the Task runner installed via `arduino/setup-task@v2`
- **WHY auto-detect:** both `macos-latest` runners and developers'
  local Macs typically inherit Accessibility trust from the parent
  shell process, so a "fresh, AX-denied" runner is the exception
  rather than the default — asserting either path upfront is brittle

### Scenario: Real-window smoke test on macOS runner (strict — AX granted)

- **WHEN** the script runs locally with `WCTL_SMOKE_AX=1` after the
  developer has confirmed Accessibility access is granted to the
  invoking shell
- **THEN** the script REQUIRES the granted path: move must exit 0
  AND bounds must land within `WCTL_AX_TOLERANCE` pixels of the
  request. If move returns `ErrAccessibilityDenied` despite the
  flag, the script fails loudly (catches a regression where TCC
  trust silently breaks on a previously-trusted setup)

## Requirement: Testing constraints

### Scenario: No Docker for window-management tests

- **WHEN** any window-management test is added to CI
- **THEN** it must not run inside Docker (no real window manager
  available)
- **AND** multi-monitor behavior is acknowledged as not
  integration-tested in CI (GitHub runners are single-display)

## Tasks

| Task | Purpose |
|------|---------|
| `task build` | Runs in CI on each OS to verify cross-platform build |
| `task test` | Runs the unit-test matrix on each OS |
| `task lint` | Runs the linter |
| `task smoke-windows` | Drives `scripts/smoke-windows.ps1` (CI step on `windows-latest`) |
| `task smoke-darwin` | Drives `scripts/smoke-darwin.sh` (CI step on `macos-latest`) |
| `task smoke-wayland*` | Per-compositor Linux Wayland smokes (local-only — runners have no compositor) |
