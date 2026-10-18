#!/usr/bin/env bash
#
# sway (wlroots) Wayland real-window smoke test for windowctl.
#
# The sway sibling of scripts/smoke-wayland-gnome.sh (GNOME). Same
# contract: launch a real application, detect it via `windowctl
# windows list`, exercise `move` / `focus` / `monitors` /
# `screenshot`, stop the app.
#
# Two ways to run:
#
#   ./scripts/smoke-wayland-sway.sh
#       on a local sway session (env guard checks it).
#
#   WCTL_SMOKE_SSH=user@host ./scripts/smoke-wayland-sway.sh
#       builds the linux binary locally, pushes it plus this script to
#       the target's /tmp and runs the same body there with the
#       graphical session's environment injected (the SSH login shell
#       itself is a tty session — see docs/wayland/sway-mechanism.md
#       §2). Handy for the VM test environment.
#
# Target prerequisites: foot (test app), grim (screenshots). ydotool is
# not exercised (ydotoold is not part of a default sway setup; input is
# shared code already validated on KDE/GNOME).
#
# Sway specifics exercised here (sway-mechanism.md):
#   - the smoke window starts tiled, so Move's auto `floating enable`
#     path (the decided tiled-window policy) is what gets tested;
#   - Move lands the get_tree rect exactly at the target (the
#     deco-title compensation is internal to the adapter);
#   - the app is identified by foot's stable app_id, not by title
#     (foot titles are overwritten by the shell's OSC escape).
#
# Tolerances: position lands exactly in spike testing; the size side
# can shift 1-2px through client-side min-size negotiation, absorbed
# by the shared WCTL_WAYLAND_TOLERANCE (default 50px).

set -euo pipefail

SCRIPT=$(basename "$0")
WCTL_SMOKE_SSH="${WCTL_SMOKE_SSH:-}"
WCTL_WAYLAND_TOLERANCE="${WCTL_WAYLAND_TOLERANCE:-50}"
SMOKE_APP_ID=wctl-smoke

fail() {
  echo "FAIL: $*" >&2
  exit 1
}

# --- mode dispatch -----------------------------------------------------

if [ "${1:-}" = "--remote" ]; then
  # Running on the target over SSH: the binary was pushed to /tmp.
  WCTL=/tmp/wctl-smoke-bin
  WCTL_IS_REMOTE=1
else
  cd "$(dirname "$0")/.."
  if [ -n "$WCTL_SMOKE_SSH" ]; then
    echo '== Building linux binary and pushing to the SSH target =='
    go build -o /tmp/wctl-smoke-bin ./cmd/windowctl || fail 'go build failed'
    scp -q /tmp/wctl-smoke-bin "$WCTL_SMOKE_SSH:/tmp/wctl-smoke-bin" \
      || fail "scp binary to $WCTL_SMOKE_SSH failed"
    scp -q "$0" "$WCTL_SMOKE_SSH:/tmp/$SCRIPT" \
      || fail "scp script to $WCTL_SMOKE_SSH failed"
    # Re-run this script on the target with the remote flag.
    ssh -o BatchMode=yes "$WCTL_SMOKE_SSH" "bash /tmp/$SCRIPT --remote"
    exit $?
  fi
  WCTL=./windowctl
  go build -o windowctl ./cmd/windowctl || fail 'go build failed'
fi

echo '== windowctl Wayland (sway) smoke test =='

# windowctl runs from an SSH shell see a tty session, so the graphical
# session env must be injected (sway-mechanism.md §2). swaymsg does
# NOT autodiscover the socket from XDG_RUNTIME_DIR alone, and the
# socket path contains the sway PID — discover both env vars here.
SWAYSOCK="${SWAYSOCK:-}"
WAYLAND_DISPLAY="${WAYLAND_DISPLAY:-}"
if [ -n "${WCTL_IS_REMOTE:-}" ]; then
  XDG_RUNTIME_DIR="/run/user/$(id -u)/"
  [ -n "$SWAYSOCK" ] || SWAYSOCK=$(ls "$XDG_RUNTIME_DIR"sway-ipc.*.sock 2>/dev/null | head -1 || true)
  [ -n "$SWAYSOCK" ] || fail 'no sway IPC socket in XDG_RUNTIME_DIR — is a sway session running?'
  [ -n "$WAYLAND_DISPLAY" ] || WAYLAND_DISPLAY=$(find "$XDG_RUNTIME_DIR" -maxdepth 1 -name 'wayland-*' ! -name '*.lock' 2>/dev/null | head -1 || true)
  [ -n "$WAYLAND_DISPLAY" ] || fail 'no wayland socket in XDG_RUNTIME_DIR'
  WAYLAND_DISPLAY=$(basename "$WAYLAND_DISPLAY")
fi

wctl() {
  if [ -n "${WCTL_IS_REMOTE:-}" ]; then
    env XDG_SESSION_TYPE=wayland XDG_CURRENT_DESKTOP=sway \
      SWAYSOCK="$SWAYSOCK" WAYLAND_DISPLAY="$WAYLAND_DISPLAY" \
      XDG_RUNTIME_DIR="${XDG_RUNTIME_DIR:-/run/user/$(id -u)/}" \
      "$WCTL" "$@"
  else
    "$WCTL" "$@"
  fi
}

# --- 1. session guard --------------------------------------------------
# The graphical session's env lives in sway's children (sway itself
# does not carry SWAYSOCK — sway-mechanism.md §2); the socket check
# above is the real guard for remote runs. This guard covers local
# runs identically for both.
pgrep -x sway >/dev/null || fail 'no sway process — is a sway session running?'
[ -n "${SWAYSOCK}" ] || SWAYSOCK=$(ls "/run/user/$(id -u)/"sway-ipc.*.sock 2>/dev/null | head -1 || true)
[ -n "${SWAYSOCK}" ] || fail 'no sway IPC socket — SWAYSOCK unset and none discoverable'

WCTL_IS_REMOTE=1   # stamp the env injection for wctl()

# --- 2. sanity ---------------------------------------------------------
wctl windows list --json >/dev/null || fail 'windows list failed'

# --- 3. launch a real application --------------------------------------
# foot with a dedicated app_id: deterministic, locale-independent, and
# the title is shell-controlled (OSC escape) so app_id is the only
# stable handle. Kill leftovers so repeated runs never match a stale
# window. Launched here rather than via `wctl launch` so the app_id
# argument can be passed; the launch path itself is exercised by the
# KDE/GNOME smokes and is compositor-independent exec code.
pkill -x foot 2>/dev/null || true
sleep 0.5
WAYLAND_DISPLAY="$WAYLAND_DISPLAY" XDG_RUNTIME_DIR="${XDG_RUNTIME_DIR:-/run/user/$(id -u)/}" \
  setsid foot --app-id="$SMOKE_APP_ID" >/dev/null 2>&1 < /dev/null &

cleanup() {
  pkill -x foot 2>/dev/null || true
  rm -f /tmp/wctl-shot.png /tmp/wctl-list-after.json /tmp/wctl-move.err
}
trap cleanup EXIT

# --- 4. poll until the window appears (up to ~10s) ---------------------
deadline=$(($(date +%s) + 10))
detected=''
while [ "$(date +%s)" -lt "$deadline" ]; do
  detected=$(wctl windows list --json 2>/dev/null | python3 -c "
import json, sys
ws = json.load(sys.stdin) or []
hits = [w for w in ws if (w.get('App') or '') == '$SMOKE_APP_ID']
print(hits[0]['ID'] if hits else '')
" || true)
  if [ -n "$detected" ]; then
    break
  fi
  sleep 0.3
done
[ -n "$detected" ] || fail "smoke window not detected within 10s (final list: $(wctl windows list | head -5))"
echo "Detected smoke window ID=$detected (app_id=$SMOKE_APP_ID)"

# --- 5. move, settle, verify bounds ------------------------------------
# The window is tiled at this point, so this exercises the
# float-enable + deco-compensation Move path end to end.
wctl move --app "$SMOKE_APP_ID" --x 700 --y 400 --w 500 --h 600 \
  2>/tmp/wctl-move.err || fail "move failed: $(cat /tmp/wctl-move.err)"
sleep 0.5

wctl windows list --json > /tmp/wctl-list-after.json
WCTL_TOL="$WCTL_WAYLAND_TOLERANCE" python3 - <<'PY'
import json, sys
tol = int(__import__('os').environ['WCTL_TOL'])
with open('/tmp/wctl-list-after.json') as f:
    ws = json.load(f) or []
hit = next((w for w in ws if (w.get('App') or '') == 'wctl-smoke'), None)
if hit is None:
    print('smoke window vanished after move', file=sys.stderr)
    sys.exit(1)
b = hit.get('Bounds') or {}
expected = {'X': 700, 'Y': 400, 'Width': 500, 'Height': 600}
diffs = {k: abs(b.get(k, 0) - expected[k]) for k in expected}
bad = {k: d for k, d in diffs.items() if d > tol}
if bad:
    print(f"Move bounds outside tolerance ({tol}px): got {b}, want {expected}, diffs {diffs}", file=sys.stderr)
    sys.exit(1)
print(f"Move applied within tolerance: bounds={b} (want {expected}, tol {tol}px)")
PY

# --- 6. focus, verify via the Focused stamp ----------------------------
# The public layer stamps window Focused from the focused monitor,
# which the backend derives from the tree's focused flag — this
# round-trips [con_id=N] focus → node.focused → monitor → stamp.
wctl focus --app "$SMOKE_APP_ID" || fail 'focus failed'
sleep 0.5
WCTL_IS_REMOTE="${WCTL_IS_REMOTE:-}" SWAYSOCK="${SWAYSOCK:-}" WAYLAND_DISPLAY="${WAYLAND_DISPLAY:-}" \
XDG_CURRENT_DESKTOP=sway python3 - <<'PY'
import json, os, subprocess, sys

# The bash wctl() helper is unreachable from python; rebuild the env
# injection here (remote runs see a tty session without the graphical
# session env — sway-mechanism.md §2).
def wctl(*args):
    env = dict(os.environ)
    binary = '/tmp/wctl-smoke-bin' if os.environ.get('WCTL_IS_REMOTE') else './windowctl'
    if os.environ.get('WCTL_IS_REMOTE'):
        uid = os.getuid()
        xrd = '/run/user/%d/' % uid
        env.update({
            'XDG_SESSION_TYPE': 'wayland',
            'XDG_CURRENT_DESKTOP': 'sway',
            'XDG_RUNTIME_DIR': xrd,
        })
        if os.environ.get('SWAYSOCK'):
            env['SWAYSOCK'] = os.environ['SWAYSOCK']
        if os.environ.get('WAYLAND_DISPLAY'):
            env['WAYLAND_DISPLAY'] = os.environ['WAYLAND_DISPLAY']
    return subprocess.run([binary, *args], capture_output=True, text=True, check=True, env=env).stdout

ws = json.loads(wctl('windows', 'list', '--json')) or []
hit = next((w for w in ws if (w.get('App') or '') == 'wctl-smoke'), None)
if hit is None:
    print('smoke window vanished after focus', file=sys.stderr)
    sys.exit(1)
if not hit.get('Focused'):
    print('focus did not mark the window Focused', file=sys.stderr)
    sys.exit(1)
print("Focus applied: window reports Focused=true")
PY

# --- 7. monitors: at least one output, primary flagged -----------------
# sway reports a real primary flag; when no output declares one (the
# VM's QEMU display does not), the backend promotes the focused
# output, so exactly one Primary is always expected here.
mon_count=$(wctl monitors list --json 2>/dev/null | python3 -c '
import json, sys
ms = json.load(sys.stdin) or []
assert any(m.get("Primary") for m in ms), "no primary monitor flagged"
print(len(ms))
') || fail 'monitors list failed or has no primary'
echo "Monitors listed: $mon_count"

# --- 8. screenshot: 1 image pixel == 1 coordinate point ----------------
wctl screenshot --x 700 --y 400 --w 500 --h 600 --out /tmp/wctl-shot.png \
  || fail 'screenshot failed'
python3 - <<'PY'
import struct, sys
with open('/tmp/wctl-shot.png', 'rb') as f:
    data = f.read(33)
assert data[:8] == b'\x89PNG\r\n\x1a\n', 'not a PNG'
w, h = struct.unpack('>II', data[16:24])
if (w, h) != (500, 600):
    print(f'screenshot size {w}x{h}, want 500x600 (point normalization broken?)', file=sys.stderr)
    sys.exit(1)
print(f"Screenshot normalized: {w}x{h} = 1 image pixel per coordinate point")
PY

echo '== Smoke test PASSED =='
