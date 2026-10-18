# Spec: `go-library-api`

> Part of the [windowctl OpenSpec capability set](./README.md).

- **Status:** Implemented
- **Source:** LIB-01, LIB-02, LIB-03, §8.3
- **Summary:** Make the library that already powers the CLI usable as a
  first-class, importable Go module at `github.com/Jiafei-Queen/windowctl`,
  with no CLI dependencies leaking into consumers.

> **Timeline note.** From v1 the CLI has been a thin wrapper around an
> internal library package, and core logic has been OS-mockable from the
> start. The exported public surface and the no-CLI-deps guarantee landed
> with the `os-spikes` slice (module rooted at the repo with library files
> at the top level) and were sealed by the godoc examples that import the
> package as an external consumer.

## Requirement: CLI is a thin wrapper

### Scenario: All business logic lives in the library

- **WHEN** the CLI binary handles any subcommand (`windows list`,
  `monitors list`, `move`, `focus`)
- **THEN** it parses flags and delegates to the library
- **AND** it contains no zone-math, monitor-resolution, or
  window-matching logic of its own

## Requirement: Core logic is testable without OS dependencies

### Scenario: Core packages mock the OS adapter

- **WHEN** the core packages' unit tests run
- **THEN** they execute without a real window manager, using mock
  adapters satisfying the OS interface (per §8.3)

## Requirement: Public API surface

### Scenario: Library exposes the documented types and functions

- **WHEN** a Go consumer imports `github.com/Jiafei-Queen/windowctl`
- **THEN** the package exposes the types `Window`, `Monitor`, `Rect`,
  `Filter`, `Match`, `Target` and the functions
  `ListWindows(filter Filter)`, `ListMonitors()`,
  `Move(match Match, target Target)`, `Focus(match Match)` with the
  signatures defined in LIB-01
- **AND** the higher-level helpers `MoveZone(match, monitorID, zoneStr)`
  and `MoveCoords(match, monitorID, bounds)` are also exported for
  callers that don't want to compute target rectangles themselves

## Requirement: Library importability without CLI deps

### Scenario: Importing the library does not pull in CLI deps

- **WHEN** a third-party Go module depends only on
  `github.com/Jiafei-Queen/windowctl`
- **THEN** transitive dependencies do not include the CLI's flag/parsing
  packages (per LIB-03) — the CLI lives in its own `package main` under
  `cmd/windowctl/` and is never reached from the library's import graph
- **AND** `example_test.go` (in `package windowctl_test`) imports the
  library the same way an external consumer would and is built on every
  CI run, so any future regression that pulls CLI packages into the
  library graph would surface as a build failure

## Tasks

| Task | Purpose |
|------|---------|
| `task build` | Builds the CLI, which already delegates to the library |
| `task test` | Runs the library's OS-mockable unit tests and the external `windowctl_test` package containing the godoc examples |

> The external `windowctl_test` package is the importer smoke that
> previously did not exist; it builds on every CI run and would fail
> if the library accidentally imported CLI-only packages.
