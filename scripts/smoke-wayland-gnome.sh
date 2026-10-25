#!/usr/bin/env bash
#
# GNOME (Mutter) Wayland real-window smoke test for windowctl.
#
# The GNOME sibling of scripts/smoke-wayland.sh (KDE). Same contract:
# launch a real application, detect it via `windowctl windows list`,
# exercise `move` / `focus` / `monitors` / `screenshot`, stop the app.
#
# Two ways to run:
#
#   ./scripts/smoke-wayland-gnome.sh
#       on a local GNOME Wayland session (env guard checks it).
#
#   WCTL_SMOKE_SSH=user@host ./scripts/smoke-wayland-gnome.sh
#       builds the linux binary locally, pushes it plus this script to
#       the target's /tmp and runs the same body there with the
#       graphical session's environment injected (the SSH login shell
#       itself is a tty session — see docs/wayland/gnome-mechanism.md
#       §6). Handy for the VM test environment.
#
# Unlike the KDE smoke this needs no external tools on the target: the
# GNOME backend screenshots via the in-process extension API and needs
# no spectacle equivalent. What the target DOES need is the windowctl
# extension, which auto-installs on the first windowctl call — but a
# fresh install only activates after one logout/login cycle (GNOME
# Shell scans the extensions dir at session start), so run the smoke
# once after that.
#
# Tolerances: GNOME applied move_resize_frame synchronously in spike
# testing, but the assertion keeps a settle wait and the shared
# WCTL_WAYLAND_TOLERANCE (default 50px) to absorb toolkit min-size
# clamping (GNOME Calculator in particular resizes within its own
# minimum).

set -euo pipefail

WCTL_SMOKE_SSH="${WCTL_SMOKE_SSH:-}"
WCTL_WAYLAND_TOLERANCE="${WCTL_WAYLAND_TOLERANCE:-50}"

# Shared fail() + SSH push / remote / local build dispatch.
# shellcheck disable=SC1091  # sibling script, resolved at runtime
source "$(dirname "$0")/smoke-lib.sh"
smoke_dispatch "${1:-}"

echo '== windowctl Wayland (GNOME) smoke test =='

# windowctl runs from an SSH shell see a tty session, so the graphical
# session env must be injected (gnome-mechanism.md §6).
wctl() {
  if [ -n "${WCTL_IS_REMOTE:-}" ]; then
    env XDG_SESSION_TYPE=wayland XDG_CURRENT_DESKTOP=GNOME \
      DBUS_SESSION_BUS_ADDRESS="unix:path=/run/user/$(id -u)/bus" \
      XDG_RUNTIME_DIR="/run/user/$(id -u)/" \
      "$WCTL" "$@"
  else
    "$WCTL" "$@"
  fi
}

# --- 1. session guard --------------------------------------------------
# The graphical session's env lives in the gnome-shell process; this
# guard works identically for local runs and SSH runs.
GPID=$(pgrep -x gnome-shell | head -1) || true
[ -n "$GPID" ] || fail 'no gnome-shell process — is a GNOME session running?'
SENV=$(tr '\0' '\n' < "/proc/$GPID/environ")
grep -q '^XDG_SESSION_TYPE=wayland$' <<<"$SENV" \
  || fail 'gnome-shell is not running on Wayland (XDG_SESSION_TYPE != wayland)'
grep -q '^XDG_CURRENT_DESKTOP=.*GNOME' <<<"$SENV" \
  || fail "gnome-shell is not a GNOME session (XDG_CURRENT_DESKTOP: $(grep '^XDG_CURRENT_DESKTOP=' <<<"$SENV" || echo unset))"

WCTL_IS_REMOTE=1   # stamp the env injection for wctl()

# --- 2. sanity ---------------------------------------------------------
wctl windows list --json >/dev/null || fail 'windows list failed'

# --- 3. launch a real application --------------------------------------
# gnome-calculator: deterministic wm_class (locale-independent App
# matching), no profile/title quirks like terminals. Kill leftovers so
# repeated runs can never match a stale window.
pkill -f gnome-calculator 2>/dev/null || true
sleep 0.5
wctl launch --app gnome-calculator || fail 'launch gnome-calculator failed'

cleanup() {
  pkill -f gnome-calculator 2>/dev/null || true
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
hits = [w for w in ws if (w.get('App') or '').lower().startswith('org.gnome.calculator')]
print(hits[0]['ID'] if hits else '')
" || true)
  if [ -n "$detected" ]; then
    break
  fi
  sleep 0.3
done
[ -n "$detected" ] || fail "calculator window not detected within 10s (final list: $(wctl windows list | head -5))"
echo "Detected smoke window ID=$detected (App=org.gnome.Calculator)"

# Let the just-mapped window finish its initial configure cycle: mutter
# silently ignores move_resize_frame issued while the client is still
# settling after map (observed on GNOME 48; GNOME applies geometry
# synchronously once the window is mapped).
sleep 1

# --- 5. move, settle, verify bounds ------------------------------------
wctl move --app org.gnome.Calculator --x 700 --y 400 --w 500 --h 700 \
  2>/tmp/wctl-move.err || fail "move failed: $(cat /tmp/wctl-move.err)"
sleep 0.5

wctl windows list --json > /tmp/wctl-list-after.json
WCTL_TOL="$WCTL_WAYLAND_TOLERANCE" python3 - <<'PY'
import json, sys
tol = int(__import__('os').environ['WCTL_TOL'])
with open('/tmp/wctl-list-after.json') as f:
    ws = json.load(f) or []
hit = next((w for w in ws if (w.get('App') or '').lower().startswith('org.gnome.calculator')), None)
if hit is None:
    print('smoke window vanished after move', file=sys.stderr)
    sys.exit(1)
b = hit.get('Bounds') or {}
expected = {'X': 700, 'Y': 400, 'Width': 500, 'Height': 700}
diffs = {k: abs(b.get(k, 0) - expected[k]) for k in expected}
bad = {k: d for k, d in diffs.items() if d > tol}
if bad:
    print(f"Move bounds outside tolerance ({tol}px): got {b}, want {expected}, diffs {diffs}", file=sys.stderr)
    sys.exit(1)
print(f"Move applied within tolerance: bounds={b} (want {expected}, tol {tol}px)")
PY

# --- 6. focus, verify via the Focused stamp ----------------------------
# The public layer stamps window Focused from the focused monitor,
# which the extension derives from the compositor's focus window —
# so this round-trips activate() → has_focus → monitor → stamp.
wctl focus --app org.gnome.Calculator || fail 'focus failed'
sleep 0.5
WCTL_IS_REMOTE="${WCTL_IS_REMOTE:-}" python3 - <<'PY'
import json, os, subprocess, sys

# The bash wctl() helper is unreachable from python; rebuild the env
# injection here (remote runs see a tty session without the graphical
# session env — gnome-mechanism.md §6).
def wctl(*args):
    env = dict(os.environ)
    binary = '/tmp/wctl-smoke-bin' if os.environ.get('WCTL_IS_REMOTE') else './windowctl'
    if os.environ.get('WCTL_IS_REMOTE'):
        env.update({
            'XDG_SESSION_TYPE': 'wayland',
            'XDG_CURRENT_DESKTOP': 'GNOME',
            'DBUS_SESSION_BUS_ADDRESS': 'unix:path=/run/user/%d/bus' % os.getuid(),
            'XDG_RUNTIME_DIR': '/run/user/%d/' % os.getuid(),
        })
    return subprocess.run([binary, *args], capture_output=True, text=True, check=True, env=env).stdout

ws = json.loads(wctl('windows', 'list', '--json')) or []
hit = next((w for w in ws if (w.get('App') or '').lower().startswith('org.gnome.calculator')), None)
if hit is None:
    print('smoke window vanished after focus', file=sys.stderr)
    sys.exit(1)
# The Focused stamp is what the input guard (type/key with a filter)
# polls for, so it must land on the window that was just focused and on
# no other — a stamp on some other window silently retargets input.
# (Checking hits[0] alone is not enough: with two calculators open the
# first match is whichever one happens to sit lowest in the list.)
focused = [w for w in ws if w.get('Focused')]
if len(focused) != 1:
    print(f'expected exactly one Focused window, got {len(focused)}', file=sys.stderr)
    sys.exit(1)
if not (focused[0].get('App') or '').lower().startswith('org.gnome.calculator'):
    print(f"focus did not mark the calculator Focused (the stamp is on {focused[0].get('App')})", file=sys.stderr)
    sys.exit(1)
# The window list is frontmost-first, so the focused window heads it.
if ws[0].get('ID') != focused[0].get('ID'):
    print('the focused window is not the first entry of the window list', file=sys.stderr)
    sys.exit(1)
print("Focus applied: window reports Focused=true and heads the list")
PY

# --- 7. monitors: at least one output, primary flagged -----------------
mon_count=$(wctl monitors list --json 2>/dev/null | python3 -c '
import json, sys
ms = json.load(sys.stdin) or []
assert any(m.get("Primary") for m in ms), "no primary monitor flagged"
print(len(ms))
') || fail 'monitors list failed or has no primary'
echo "Monitors listed: $mon_count"

# --- 8. screenshot: 1 image pixel == 1 coordinate point ----------------
wctl screenshot --x 700 --y 400 --w 500 --h 700 --out /tmp/wctl-shot.png \
  || fail 'screenshot failed'
python3 - <<'PY'
import struct, sys
with open('/tmp/wctl-shot.png', 'rb') as f:
    data = f.read(33)
assert data[:8] == b'\x89PNG\r\n\x1a\n', 'not a PNG'
w, h = struct.unpack('>II', data[16:24])
if (w, h) != (500, 700):
    print(f'screenshot size {w}x{h}, want 500x700 (point normalization broken?)', file=sys.stderr)
    sys.exit(1)
print(f"Screenshot normalized: {w}x{h} = 1 image pixel per coordinate point")
PY

echo '== Smoke test PASSED =='
