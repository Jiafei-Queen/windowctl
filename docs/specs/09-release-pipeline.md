# Spec: `release-pipeline`

> Part of the [windowctl OpenSpec capability set](./README.md).

- **Status:** Implemented — local-driven `task release -- <version>`
  bumps versions, cross-compiles five targets via goreleaser, stages
  binaries into `npm/platforms/<os-arch>/bin/`, publishes all 6 npm
  packages, commits the bump, tags, pushes, and creates the matching
  GitHub release. The tag-triggered workflow
  (`.github/workflows/release.yaml`) builds the same five targets and
  attaches the archives to a GitHub release; this fork uses the workflow
  and does not publish to npm.
- **Source:** §9.1, §9.2
- **Summary:** Cross-compile, checksum, and publish GitHub releases
  AND npm packages for macOS, Windows, and Linux on both `amd64` and
  `arm64` (windows-arm64 excluded), orchestrated locally by Taskfile,
  goreleaser, and a small shell script.

## Requirement: Taskfile-driven build orchestration

### Scenario: Required tasks exist

- **WHEN** a developer runs `task --list`
- **THEN** the available tasks include `build`, `test`, `lint`,
  `snapshot`, `release:dry-run`, `npm:bump`, `npm:publish`, and
  `release` with the behavior described in this spec

## Requirement: Cross-compiled binaries

### Scenario: GoReleaser produces all target binaries

- **WHEN** `task snapshot` (or `task release -- <version>` internally)
  runs
- **THEN** GoReleaser produces five binaries — `darwin/amd64`,
  `darwin/arm64`, `linux/amd64`, `linux/arm64`, `windows/amd64` —
  with the documented ldflags (`-s -w` + version/commit/date stamping)
- **AND** each per-build post hook fires
  `scripts/stage-npm-binary.sh "{{ .Target }}" "{{ .Path }}"`, which
  copies the freshly-built binary into the matching
  `npm/platforms/<os-arch>/bin/` directory

### Scenario: windows-arm64 is intentionally skipped

- **WHEN** the goreleaser matrix expands
- **THEN** the `windowctl-nondarwin` build's `ignore` rule excludes
  `goos: windows, goarch: arm64` — niche platform, no demand yet,
  matching the npm sub-package set (which also has no
  `windows-arm64` package)

## Requirement: macOS CGO link

### Scenario: macOS build uses CGO with the required frameworks

- **WHEN** the macOS binary is built
- **THEN** the `windowctl-darwin` build entry sets `CGO_ENABLED=1` and
  the framework link line lives in
  `internal/adapter/darwin/adapter.go`'s `#cgo LDFLAGS:` directive
  (`-framework CoreGraphics -framework CoreFoundation -framework
  ApplicationServices -framework AppKit`) — goreleaser does not need
  to re-state framework flags

## Requirement: Checksums

### Scenario: SHA-256 checksums are produced alongside the binaries

- **WHEN** a release (or snapshot) is built
- **THEN** SHA-256 checksums are generated for every archive and
  recorded in `dist/checksums.txt`

## Requirement: Local publish path

### Scenario: One-shot release

- **WHEN** the developer runs `task release -- <version>` (which
  invokes `scripts/release-local.sh <version>`)
- **THEN** the script runs the full pipeline on the local machine:
  preflight checks (`npm whoami`, `gh auth status`, clean working
  tree, no existing tag) → `node scripts/bump-npm-version.js
  <version>` → `goreleaser release --snapshot --clean --skip=publish`
  (binaries land in `dist/` AND get staged into `npm/platforms/...`
  via the post-build hook) → `bash scripts/publish-npm-local.sh`
  (publish 5 sub-packages, then main) → commit the version bump
  → `git tag v<version>` → `git push origin main` →
  `git push origin v<version>` → `gh release create v<version>`
  with the tarballs, zips, and checksums
- **AND** this path needs no GitHub Actions secrets and no `NPM_TOKEN`
  in CI — auth is whatever `npm whoami` and `gh auth status` already
  trust on the developer's machine. The tag-triggered workflow
  (`.github/workflows/release.yaml`) is the CI alternative

## Tasks

| Task | Purpose |
|------|---------|
| `task build` | Builds the CLI binary for the current platform |
| `task test` | Runs all tests prior to release |
| `task lint` | Runs `go vet` prior to release |
| `task snapshot` | `goreleaser --snapshot --skip=publish` — five binaries staged into `npm/platforms/...` |
| `task release:dry-run` | Same as snapshot, with `--verbose` for debugging the goreleaser config |
| `task npm:bump -- <ver>` | Sync all 6 `package.json` versions |
| `task npm:publish` | Publish all 6 packages non-interactively (binaries must already be staged) |
| `task release -- <ver>` | Full one-shot local release |
