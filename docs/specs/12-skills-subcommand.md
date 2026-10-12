# Spec: `skills-subcommand`

> Part of the [windowctl OpenSpec capability set](./README.md).

- **Status:** Implemented
- **Source:** FR-SKL-01, FR-SKL-02, requirements §11
- **Summary:** Add `windowctl install --skills` / `windowctl uninstall --skills`
  so the bundled `window-ctl-skill` can be installed directly from the CLI
  without requiring a separate `npx skills add ...` step.

The install surface is intentionally local-first: the skill payload is bundled
into the binary itself, so `windowctl install --skills` works from `go install`,
npm-distributed binaries, and direct release downloads without cloning the repo
or reaching out to GitHub at install time.

## Requirement: `windowctl install --skills` exists as a top-level entry point

### Scenario: Subcommand listed in CLI usage

- **WHEN** the user runs `windowctl --help` or `windowctl help`
- **THEN** the usage text lists
  `windowctl install --skills [--agents]`

### Scenario: Top-level command routed in `cmd/windowctl/main.go`

- **WHEN** the user invokes `windowctl install --skills` or
  `windowctl uninstall --skills`
- **THEN** `main.go` dispatches to `installCmd` / `uninstallCmd` handlers
- **AND** those handlers delegate the real work to public package functions,
  preserving the CLI-as-thin-wrapper invariant

## Requirement: Install bundled skill into the standard local skill roots

### Scenario: Install to the Claude skill directory

- **WHEN** the user invokes `windowctl install --skills`
- **THEN** the bundled skill is copied into
  `~/.claude/skills/window-ctl-skill`
- **AND** the command prints the installed path

### Scenario: Install to the agents skill directory when Codex is present

- **WHEN** the user invokes `windowctl install --skills`
- **AND** `codex` is on PATH
- **THEN** the bundled skill is also copied into
  `~/.agents/skills/window-ctl-skill`

### Scenario: Force the agents install without Codex discovery

- **WHEN** the user invokes `windowctl install --skills --agents`
- **THEN** the bundled skill is copied into
  `~/.agents/skills/window-ctl-skill` even if `codex` is not on PATH

## Requirement: Install works from packaged binaries

### Scenario: Binary has no adjacent checkout

- **WHEN** `windowctl` is installed via `go install`, npm, or a release tarball
- **THEN** `windowctl install --skills` still succeeds
- **AND** it does not depend on a sibling `skills/` directory on disk, because
  the skill assets are embedded into the Go binary

## Requirement: Automatic install on every run

Every `windowctl` command except `install`/`uninstall`/`help` calls `EnsureBundledSkill`: each target (`$CLAUDE_CONFIG_DIR/skills` or `~/.claude/skills`, then `window-ctl-skill`; `~/.agents/skills/window-ctl-skill` when `~/.agents` exists or `codex` is on PATH) is (re)installed when missing or when its `.windowctl-bundle` stamp differs from the sha256 of the embedded bundle. One stderr line per change; silent otherwise; failures never fail the command.

Skipped when: `WINDOWCTL_NO_SKILL_AUTOINSTALL` is non-empty; the target is a symlink (dev link); or the user ran `uninstall --skills` (marker `<UserConfigDir>/windowctl/skill-autoinstall-off`, cleared by `install --skills`).

### Scenario: upgrade refreshes the skill

- **Given** an installed skill stamped by an older binary
- **When** the new binary runs any command
- **Then** the skill is replaced and stderr says `agent skill updated at <path>`; the next run prints nothing.

## Requirement: Uninstall is symmetric and idempotent

### Scenario: Remove installed skill

- **WHEN** the user invokes `windowctl uninstall --skills`
- **THEN** the installed `window-ctl-skill` directory is removed from each
  targeted skill root

### Scenario: Remove missing skill

- **WHEN** the user invokes `windowctl uninstall --skills`
- **AND** the bundled skill is not currently installed in one or more targets
- **THEN** the command reports `not-installed` for those targets
- **AND** exits 0

## Tasks

| Task | Purpose |
|------|---------|
| `task build` | Builds the CLI binary that now exposes `windowctl install --skills` |
| `task test` | Runs unit tests for CLI wiring, embedded-skill install/uninstall behavior, and help text |
