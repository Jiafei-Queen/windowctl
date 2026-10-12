# windowctl — OpenSpec Specifications

**Source of truth:** [`docs/requirements.md`](../requirements.md) v0.1.

This folder slices the windowctl requirements into vertical, user-facing
capability specs in OpenSpec style. Each slice lives in its **own file**, is
self-contained (input → behavior → output), and references back to the
requirement IDs it covers.

Slices are numbered by **delivery value**, not by document order.
[`os-spikes`](./01-os-spikes.md) leads as the tracer-bullet foundation:
it proves each target OS can actually drive a real window before any
higher-level capability is built. Everything else builds on it.

---

## Status Index

| # | Spec | Status | Source requirements |
|---|------|--------|---------------------|
| 1 | [`os-spikes`](./01-os-spikes.md) | Implemented | §7, §7.1, §10.3 |
| 2 | [`windows-list`](./02-windows-list.md) | Implemented | FR-WIN-01, FR-OUT-01, FR-OUT-02, §11 |
| 3 | [`monitors-list`](./03-monitors-list.md) | Implemented | FR-MON-01, FR-OUT-01, FR-OUT-02 |
| 4 | [`window-move-zone`](./04-window-move-zone.md) | Implemented | FR-MOV-01, FR-MOV-03, FR-MOV-04, §4.3.1, §4.3.2, §11 |
| 5 | [`window-move-coords`](./05-window-move-coords.md) | Implemented | FR-MOV-02, FR-MOV-04, §4.3.3, §11 |
| 6 | [`window-focus`](./06-window-focus.md) | Implemented | FR-FOC-01, §11 |
| 7 | [`go-library-api`](./07-go-library-api.md) | Implemented | LIB-01, LIB-02, LIB-03, §8.3 |
| 8 | [`npm-distribution`](./08-npm-distribution.md) | Implemented | §9.3, OI-05 |
| 9 | [`release-pipeline`](./09-release-pipeline.md) | Implemented | §9.1, §9.2 |
| 10 | [`ci-test-matrix`](./10-ci-test-matrix.md) | Partial | §10.1, §10.2, §10.3, §10.4 |
| 11 | [`permissions-subcommand`](./11-permissions-subcommand.md) | Implemented | §7.2, §11 |
| 12 | [`skills-subcommand`](./12-skills-subcommand.md) | Implemented | FR-SKL-01, FR-SKL-02, §11 |
| 13 | [`screenshot`](./13-screenshot.md) | Implemented | FR-SHOT-01, §8.2, §11 |
| 14 | [`input-automation`](./14-input-automation.md) | Implemented | FR-INP-01, FR-INP-02, FR-INP-03, §11 |
| 15 | [`remote-control`](./15-remote-control.md) | Implemented | FR-RMT-01, §8.2, §11 |
| 16 | [`draw`](./16-draw.md) | Implemented (mac/win) | FR-DRW-01, §8, §11 |

**Status legend:** `Implemented` · `Partial` · `Planned` · `Not planned`.

---

## Architectural ground truth

From v1, the CLI has always been a thin wrapper over an internal library
package — every CLI command across these specs is implemented by delegating
to the library, not by reimplementing logic in the CLI layer. What was
**not** done initially was **exporting** that library as a public, importable
Go module. That export is a later capability and is captured in
[`go-library-api`](./07-go-library-api.md), whose status reflects this
split: the wrapper-shape and OS-mockable core have been in place from the
beginning, the public Go-module surface and dependency-isolation guarantee
came later.

Individual CLI specs do not restate "the CLI delegates to the library" in
every scenario — that is a global invariant covered once, here.

---

## Spec file conventions

Each slice file follows the same shape:

- **Front matter:** `Status`, `Source`, `Summary`.
- **`## Requirement: <name>`** — one per behavioral requirement in scope.
- **`### Scenario: <name>`** — Given/When/Then-style bullets under each
  requirement.
- **`## Tasks`** — small Taskfile-style table mapping the spec back to
  the `task <name>` command(s) that build, test, or otherwise verify
  it. Where no current task covers a part of the spec, that gap is
  called out explicitly under the table.

Status values match the index legend above. When a single spec mixes
implemented and planned requirements, per-requirement status is annotated
inline (e.g. *(Implemented from v1)* / *(Planned)*).
