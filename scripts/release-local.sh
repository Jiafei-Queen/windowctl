#!/usr/bin/env bash
# release-local.sh — manual, all-local release flow.
#
# One-shot:
#   bash scripts/release-local.sh 0.2.0
#
# What it does, in order:
#   1. Preflight (npm login, goreleaser, gh CLI, clean working tree).
#   2. Bump versions across all 6 package.json files.
#   3. goreleaser --snapshot: cross-compile 5 targets into dist/ and
#      stage binaries into npm/platforms/<os-arch>/bin/.
#   4. Publish all 6 npm packages (non-interactive, no OTP prompt).
#   5. Commit the version bump, tag v<version>, push main + tag.
#   6. Create GitHub release with tarballs + checksums.
#
# No GitHub Actions are involved. No NPM_TOKEN secret needed in CI.

set -e
cd "$(dirname "$0")/.."

VERSION="${1:-}"
if [ -z "$VERSION" ]; then
  echo "usage: bash scripts/release-local.sh <version>" >&2
  echo "example: bash scripts/release-local.sh 0.2.0" >&2
  exit 1
fi

# ── preflight ────────────────────────────────────────────────────────
command -v goreleaser >/dev/null || { echo "✗ goreleaser not on PATH (brew install goreleaser)" >&2; exit 1; }
command -v gh         >/dev/null || { echo "✗ gh CLI not on PATH" >&2; exit 1; }
command -v node       >/dev/null || { echo "✗ node not on PATH" >&2; exit 1; }

who=$(npm whoami 2>/dev/null || true)
[ -z "$who" ] && { echo "✗ npm not logged in — run: npm login" >&2; exit 1; }
echo "→ npm: $who"

gh auth status >/dev/null || { echo "✗ gh not authed — run: gh auth login" >&2; exit 1; }
echo "→ gh:  $(gh api user -q .login)"

if ! git diff --quiet || ! git diff --staged --quiet; then
  echo "✗ working tree is dirty. Commit or stash first." >&2
  exit 1
fi

git fetch --tags >/dev/null 2>&1 || true
if git rev-parse "v$VERSION" >/dev/null 2>&1; then
  echo "✗ tag v$VERSION already exists" >&2
  exit 1
fi

# ── bump ─────────────────────────────────────────────────────────────
echo ""
echo "── bump npm versions to $VERSION ──"
node scripts/bump-npm-version.js "$VERSION"

# ── build ────────────────────────────────────────────────────────────
echo ""
echo "── goreleaser snapshot build (5 targets) ──"
goreleaser release --snapshot --clean --skip=publish

# Goreleaser snapshot mode names artifacts using its synthesized
# "{{.Version}}-snapshot" template, where {{.Version}} is derived from
# the latest git tag (often "0.0.0" pre-first-release). That doesn't
# match the $VERSION the user is publishing under, so rename the
# tarballs/zips and rewrite checksums.txt to use $VERSION before we
# attach them to the GitHub release. Idempotent: no-op if the
# artifacts are already named with $VERSION.
SNAPSHOT_VERSION=$(ls dist/windowctl_*_darwin_arm64.tar.gz 2>/dev/null \
  | sed -E 's|.*windowctl_(.+)_darwin_arm64\.tar\.gz|\1|' | head -1)
if [ -n "$SNAPSHOT_VERSION" ] && [ "$SNAPSHOT_VERSION" != "$VERSION" ]; then
  echo ""
  echo "── renaming dist artifacts: $SNAPSHOT_VERSION → $VERSION ──"
  for f in dist/windowctl_${SNAPSHOT_VERSION}_*; do
    [ -e "$f" ] || continue
    mv "$f" "${f//${SNAPSHOT_VERSION}/${VERSION}}"
  done
  # sed -i needs a backup-suffix arg on BSD/macOS; use ".bak" then drop
  # to stay portable across mac and linux.
  sed -i.bak "s/${SNAPSHOT_VERSION}/${VERSION}/g" dist/checksums.txt
  rm -f dist/checksums.txt.bak
fi

# ── publish npm ──────────────────────────────────────────────────────
echo ""
echo "── publish to npm ──"
bash scripts/publish-npm-local.sh

# ── tag + push ───────────────────────────────────────────────────────
echo ""
echo "── commit + tag + push ──"
git add npm/package.json npm/platforms/*/package.json
# When the bump is a no-op (versions already at $VERSION from a previous
# attempt or because the working tree was already up-to-date), there's
# nothing to commit and `git commit` would exit non-zero, killing the
# script before it tags/pushes/creates the release. Guard with a staged-
# diff check so the bump-commit becomes optional but the rest of the
# release still runs.
if git diff --staged --quiet; then
  echo "── version already at $VERSION; skipping bump commit ──"
else
  git commit -m "release: v$VERSION"
fi
git tag "v$VERSION"
git push origin main
git push origin "v$VERSION"

# ── GH release ───────────────────────────────────────────────────────
echo ""
echo "── create GitHub release v$VERSION ──"
gh release create "v$VERSION" \
  dist/*.tar.gz \
  dist/*.zip \
  dist/checksums.txt \
  --title "v$VERSION" \
  --generate-notes

echo ""
echo "✓ released v$VERSION"
echo "    npm: https://www.npmjs.com/package/@muthuishere/windowctl/v/$VERSION"
echo "    gh:  https://github.com/Jiafei-Queen/windowctl/releases/tag/v$VERSION"
