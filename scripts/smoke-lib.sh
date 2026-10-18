#!/usr/bin/env bash
#
# Shared plumbing for the Wayland smoke scripts (KDE/GNOME/sway/
# Hyprland). Sourced by each script: carries the fail helper and the
# SSH push / remote / local build dispatch. Per-compositor parts —
# session guards, app selection, the wctl() env injection, the
# assertion bodies — stay in the individual scripts.

# fail aborts the smoke run with a FAIL-prefixed message.
fail() {
  echo "FAIL: $*" >&2
  exit 1
}

# smoke_dispatch runs the shared mode dispatch at the top of each smoke
# script:
#
#   --remote        running on the SSH target (the re-invocation below
#                   passes it): the binary is /tmp/wctl-smoke-bin and
#                   the per-compositor env injection must kick in.
#   WCTL_SMOKE_SSH  build locally, push binary + this script, re-run
#                   remotely with --remote, and exit with its status.
#   (otherwise)     build ./windowctl from the repo root and run here.
smoke_dispatch() {
  if [ "${1:-}" = "--remote" ]; then
    export WCTL=/tmp/wctl-smoke-bin
    export WCTL_IS_REMOTE=1
    return
  fi
  cd "$(dirname "$0")/.." || fail 'cannot cd to the repo root'
  if [ -n "${WCTL_SMOKE_SSH:-}" ]; then
    echo '== Building linux binary and pushing to the SSH target =='
    go build -o /tmp/wctl-smoke-bin ./cmd/windowctl || fail 'go build failed'
    scp -q /tmp/wctl-smoke-bin "$WCTL_SMOKE_SSH:/tmp/wctl-smoke-bin" \
      || fail "scp binary to $WCTL_SMOKE_SSH failed"
    scp -q "$0" "$WCTL_SMOKE_SSH:/tmp/$(basename "$0")" \
      || fail "scp script to $WCTL_SMOKE_SSH failed"
    # The remote re-run sources this library from /tmp — push it too.
    scp -q "$(dirname "$0")/smoke-lib.sh" "$WCTL_SMOKE_SSH:/tmp/smoke-lib.sh" \
      || fail "scp smoke-lib.sh to $WCTL_SMOKE_SSH failed"
    # Re-run this script on the target with the remote flag.
    ssh -o BatchMode=yes "$WCTL_SMOKE_SSH" "bash /tmp/$(basename "$0") --remote"
    exit $?
  fi
  export WCTL=./windowctl
  go build -o windowctl ./cmd/windowctl || fail 'go build failed'
}
