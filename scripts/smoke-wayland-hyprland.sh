#!/usr/bin/env bash
#
# Hyprland Wayland real-window smoke test for windowctl.
#
# The Hyprland sibling of scripts/smoke-wayland-sway.sh (sway). Same
# contract: launch a real application, detect it via `windowctl
# windows list`, exercise `move` / `focus` / `monitors` / `screenshot`
# / `mouse`, stop the app.
#
# Two ways to run:
#
#   ./scripts/smoke-wayland-hyprland.sh
#       on a local Hyprland session (env guard checks it).
#
#   WCTL_SMOKE_SSH=user@host ./scripts/smoke-wayland-hyprland.sh
#       builds the linux binary locally, pushes it plus this script to
#       the target's /tmp and runs the same body there with the
#       graphical session's environment injected (the SSH login shell
#       itself is a tty session — see docs/wayland/hyprland-mechanism.md).
#       Handy for the VM test environment.
#
# Target prerequisites: foot (test app), grim (screenshots). The test
# app is identified by foot's stable class (follows --app-id), not by
# title (foot titles are overwritten by the shell's OSC escape).
#
# Hyprland specifics exercised here (hyprland-mechanism.md):
#   - the smoke window starts tiled, so Move's auto-togglefloating
#     path (the decided tiled-window policy) is what gets tested;
#   - exact dispatches land at/size precisely (no title-bar
#     compensation on Hyprland's default border-only decoration);
#   - `mouse move` / `mouse position` go through hyprctl's movecursor
#     / cursorpos (NOT ydotool --absolute, which libinput's pointer
#     acceleration distorts on default configs);
#   - with a scaled monitor (the VM runs scale 2), the screenshot
#     verification below exercises the real HiDPI downsample path.

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

echo '== windowctl Wayland (Hyprland) smoke test =='

# windowctl runs from an SSH shell see a tty session, so the graphical
# session env must be injected. hyprctl needs HYPRLAND_INSTANCE_SIGNATURE
# (instance socket under XDG_RUNTIME_DIR/hypr/<sig>/); the signature is
# the newest instance directory name — stale crashed instances leave
# older dirs behind, so pick the newest.
HYPRLAND_INSTANCE_SIGNATURE="${HYPRLAND_INSTANCE_SIGNATURE:-}"
WAYLAND_DISPLAY="${WAYLAND_DISPLAY:-}"
if [ -n "${WCTL_IS_REMOTE:-}" ]; then
  XDG_RUNTIME_DIR="/run/user/$(id -u)/"
  [ -n "$HYPRLAND_INSTANCE_SIGNATURE" ] || HYPRLAND_INSTANCE_SIGNATURE=$(ls -t "$XDG_RUNTIME_DIR"hypr/ 2>/dev/null | head -1 || true)
  [ -n "$HYPRLAND_INSTANCE_SIGNATURE" ] || fail 'no Hyprland instance dir in XDG_RUNTIME_DIR — is a Hyprland session running?'
  [ -n "$WAYLAND_DISPLAY" ] || WAYLAND_DISPLAY=$(find "$XDG_RUNTIME_DIR" -maxdepth 1 -name 'wayland-*' ! -name '*.lock' 2>/dev/null | head -1 || true)
  [ -n "$WAYLAND_DISPLAY" ] || fail 'no wayland socket in XDG_RUNTIME_DIR'
  WAYLAND_DISPLAY=$(basename "$WAYLAND_DISPLAY")
fi

wctl() {
  if [ -n "${WCTL_IS_REMOTE:-}" ]; then
    env XDG_SESSION_TYPE=wayland XDG_CURRENT_DESKTOP=Hyprland \
      HYPRLAND_INSTANCE_SIGNATURE="$HYPRLAND_INSTANCE_SIGNATURE" \
      WAYLAND_DISPLAY="$WAYLAND_DISPLAY" \
      XDG_RUNTIME_DIR="${XDG_RUNTIME_DIR:-/run/user/$(id -u)/}" \
      "$WCTL" "$@"
  else
    "$WCTL" "$@"
  fi
}

# --- 1. session guard --------------------------------------------------
pgrep -x Hyprland >/dev/null || fail 'no Hyprland process — is a Hyprland session running?'
[ -n "${HYPRLAND_INSTANCE_SIGNATURE}" ] || HYPRLAND_INSTANCE_SIGNATURE=$(ls -t "/run/user/$(id -u)/hypr/" 2>/dev/null | head -1 || true)
[ -n "${HYPRLAND_INSTANCE_SIGNATURE}" ] || fail 'no Hyprland instance dir — HYPRLAND_INSTANCE_SIGNATURE unset and none discoverable'

WCTL_IS_REMOTE=1   # stamp the env injection for wctl()

# --- 2. sanity ---------------------------------------------------------
wctl windows list --json >/dev/null || fail 'windows list failed'

# --- 3. launch a real application --------------------------------------
# foot with a dedicated app-id (class follows it). Launched here rather
# than via `wctl launch` so the app-id argument can be passed; the
# launch path itself is compositor-independent exec code, exercised by
# the KDE/GNOME smokes.
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
echo "Detected smoke window ID=$detected (class=$SMOKE_APP_ID)"

# --- 5. move, settle, verify bounds ------------------------------------
# The window is tiled at this point, so this exercises the
# togglefloating + exact-resize/exact-move Move path end to end. The
# target is computed from the first reported monitor so it lands fully
# on-screen on any display size/scale (on the scale-2 VM the logical
# desktop is only 1168x630 — fixed targets would push the window off).
read -r MOVE_X MOVE_Y MOVE_W MOVE_H MOUSE_X MOUSE_Y < <(
  wctl monitors list --json | python3 -c '
import json, sys
ms = json.load(sys.stdin) or []
m = ms[0]
w = min(500, m["Width"] - 100)
h = min(600, m["Height"] - 100)
print(m["X"] + (m["Width"] - w) // 2, m["Y"] + (m["Height"] - h) // 2, w, h,
      m["X"] + m["Width"] // 2, m["Y"] + m["Height"] // 2)
') || fail 'monitors list failed'
echo "Move target: ${MOVE_X},${MOVE_Y} ${MOVE_W}x${MOVE_H} (from monitors list)"

wctl move --app "$SMOKE_APP_ID" --x "$MOVE_X" --y "$MOVE_Y" --w "$MOVE_W" --h "$MOVE_H" \
  2>/tmp/wctl-move.err || fail "move failed: $(cat /tmp/wctl-move.err)"
sleep 0.5

wctl windows list --json > /tmp/wctl-list-after.json
WCTL_TOL="$WCTL_WAYLAND_TOLERANCE" MOVE_X="$MOVE_X" MOVE_Y="$MOVE_Y" MOVE_W="$MOVE_W" MOVE_H="$MOVE_H" python3 - <<'PY'
import json, sys, os
tol = int(os.environ['WCTL_TOL'])
expected = {'X': int(os.environ['MOVE_X']), 'Y': int(os.environ['MOVE_Y']),
            'Width': int(os.environ['MOVE_W']), 'Height': int(os.environ['MOVE_H'])}
with open('/tmp/wctl-list-after.json') as f:
    ws = json.load(f) or []
hit = next((w for w in ws if (w.get('App') or '') == 'wctl-smoke'), None)
if hit is None:
    print('smoke window vanished after move', file=sys.stderr)
    sys.exit(1)
b = hit.get('Bounds') or {}
diffs = {k: abs(b.get(k, 0) - expected[k]) for k in expected}
bad = {k: d for k, d in diffs.items() if d > tol}
if bad:
    print(f"Move bounds outside tolerance ({tol}px): got {b}, want {expected}, diffs {diffs}", file=sys.stderr)
    sys.exit(1)
print(f"Move applied within tolerance: bounds={b} (want {expected}, tol {tol}px)")
PY

# --- 6. focus, verify via the Focused stamp ----------------------------
wctl focus --app "$SMOKE_APP_ID" || fail 'focus failed'
sleep 0.5
WCTL_IS_REMOTE="${WCTL_IS_REMOTE:-}" HYPRLAND_INSTANCE_SIGNATURE="${HYPRLAND_INSTANCE_SIGNATURE:-}" \
WAYLAND_DISPLAY="${WAYLAND_DISPLAY:-}" XDG_CURRENT_DESKTOP=Hyprland python3 - <<'PY'
import json, os, subprocess, sys

# The bash wctl() helper is unreachable from python; rebuild the env
# injection here (remote runs see a tty session without the graphical
# session env — hyprland-mechanism.md).
def wctl(*args):
    env = dict(os.environ)
    binary = '/tmp/wctl-smoke-bin' if os.environ.get('WCTL_IS_REMOTE') else './windowctl'
    if os.environ.get('WCTL_IS_REMOTE'):
        uid = os.getuid()
        env.update({
            'XDG_SESSION_TYPE': 'wayland',
            'XDG_CURRENT_DESKTOP': 'Hyprland',
            'XDG_RUNTIME_DIR': '/run/user/%d/' % uid,
        })
        if os.environ.get('HYPRLAND_INSTANCE_SIGNATURE'):
            env['HYPRLAND_INSTANCE_SIGNATURE'] = os.environ['HYPRLAND_INSTANCE_SIGNATURE']
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
mon_count=$(wctl monitors list --json 2>/dev/null | python3 -c '
import json, sys
ms = json.load(sys.stdin) or []
assert any(m.get("Primary") for m in ms), "no primary monitor flagged"
print(len(ms))
') || fail 'monitors list failed or has no primary'
echo "Monitors listed: $mon_count"

# --- 8. mouse: position round-trips through movecursor/cursorpos -------
# hyprctl dispatch movecursor + hyprctl cursorpos, exact at any scale —
# this is the channel that must NOT go through ydotool (see header).
wctl mouse move --x "$MOUSE_X" --y "$MOUSE_Y" || fail 'mouse move failed'
sleep 0.4
pos=$(wctl mouse position --json 2>/dev/null) || fail 'mouse position failed'
MOUSE_JSON="$pos" MOUSE_X="$MOUSE_X" MOUSE_Y="$MOUSE_Y" python3 - <<'PY'
import json, sys, os
mx, my = int(os.environ['MOUSE_X']), int(os.environ['MOUSE_Y'])
p = json.loads(os.environ['MOUSE_JSON'])
assert p['X'] == mx and p['Y'] == my, f'cursor at ({p["X"]},{p["Y"]}), want ({mx},{my}) — movecursor/cursorpos round-trip broken'
print(f"Mouse round-trip exact: cursor=({p['X']},{p['Y']})")
PY

# --- 9. screenshot: 1 image pixel == 1 coordinate point ----------------
# On the scale-2 VM grim writes logical×2 device pixels; the backend
# must resample back to the requested point size.
wctl screenshot --x "$MOVE_X" --y "$MOVE_Y" --w "$MOVE_W" --h "$MOVE_H" --out /tmp/wctl-shot.png \
  || fail 'screenshot failed'
MOVE_W="$MOVE_W" MOVE_H="$MOVE_H" python3 - <<'PY'
import struct, sys, os
w_want, h_want = int(os.environ['MOVE_W']), int(os.environ['MOVE_H'])
with open('/tmp/wctl-shot.png', 'rb') as f:
    data = f.read(33)
assert data[:8] == b'\x89PNG\r\n\x1a\n', 'not a PNG'
w, h = struct.unpack('>II', data[16:24])
if (w, h) != (w_want, h_want):
    print(f'screenshot size {w}x{h}, want {w_want}x{h_want} (point normalization broken?)', file=sys.stderr)
    sys.exit(1)
print(f"Screenshot normalized: {w}x{h} = 1 image pixel per coordinate point")
PY

echo '== Smoke test PASSED =='
