#!/usr/bin/env bash
#
# Wayland (KDE Plasma) real-window smoke test for windowctl.
#
# Mirrors scripts/smoke-darwin.sh: build the CLI, launch a real
# application (Konsole running sleep), detect it via `windowctl windows
# list`, exercise `move` / `focus` / `monitors` / `screenshot`, and stop
# the app.
#
# LOCAL-ONLY by design: GitHub runners have no Wayland compositor, and
# specs/10-ci-test-matrix.md forbids Docker for window tests, so this
# never runs in CI. Run it on a KDE Plasma Wayland session via
# `task smoke-wayland`.
#
# The script refuses to run (exit 2) on non-Wayland or non-KDE
# sessions — the smoke contract is specifically the KDE backend. On
# wlroots compositors (Sway/Hyprland) the backend is not implemented
# yet; P2 will add a sibling smoke script.
#
# Tolerances: Wayland geometry changes are asynchronous (the client
# must acknowledge the compositor's configure event), so `move` is
# followed by a settle wait before verification. WCTL_WAYLAND_TOLERANCE
# (default 50px) absorbs Konsole size-hint clamping and frame-vs-content
# skew, mirroring WCTL_AX_TOLERANCE on macOS.

set -euo pipefail

echo '== windowctl Wayland (KDE) smoke test =='

WCTL_WAYLAND_TOLERANCE="${WCTL_WAYLAND_TOLERANCE:-50}"
SMOKE_TITLE='windowctl : sleep'

fail() {
  echo "FAIL: $*" >&2
  exit 1
}

# 1. Session guard: this smoke test exercises the KDE backend only.
if [ "${XDG_SESSION_TYPE:-}" != "wayland" ]; then
  echo 'Not a Wayland session (XDG_SESSION_TYPE != wayland) — skipping.' >&2
  exit 2
fi
case "${XDG_CURRENT_DESKTOP:-}" in
  *KDE*) ;;
  *) fail "not a KDE session (XDG_CURRENT_DESKTOP=${XDG_CURRENT_DESKTOP:-unset}) — this smoke test covers the KDE backend only" ;;
esac

# 2. Build the CLI.
go build -o windowctl ./cmd/windowctl || fail 'go build failed'

# 3. Sanity: list returns something.
./windowctl windows list --json >/dev/null || fail 'windows list failed'

# 4. Launch a real application. `konsole -e sleep` gives an empty,
#    instantly-recognizable window with a stable title.
konsole -e sleep 120 &
SMOKE_PID=$!
cleanup() {
  kill "$SMOKE_PID" >/dev/null 2>&1 || true
  wait "$SMOKE_PID" 2>/dev/null || true
  rm -f /tmp/wctl-shot.png /tmp/wctl-list-after.json /tmp/wctl-move.err
}
trap cleanup EXIT

# 5. Poll until the window appears (up to ~10s).
deadline=$(($(date +%s) + 10))
detected=''
while [ "$(date +%s)" -lt "$deadline" ]; do
  detected=$(./windowctl windows list --json 2>/dev/null | python3 -c '
import json, sys
ws = json.load(sys.stdin) or []
hits = [w for w in ws if (w.get("Title") or "").startswith("windowctl : sleep")]
print(hits[0]["ID"] if hits else "")
' || true)
  if [ -n "$detected" ]; then
    break
  fi
  sleep 0.3
done
[ -n "$detected" ] || fail "smoke window not detected within 10s (final list: $(./windowctl windows list | head -5))"
echo "Detected smoke window ID=$detected"

# 6. Move. Wayland applies geometry asynchronously, so settle before
#    verifying — the same "wait for the client to ack the configure
#    event" pattern Rectangle/yabai use.
sleep 1
./windowctl move --title "$SMOKE_TITLE" --x 700 --y 500 --w 600 --h 400 2>/tmp/wctl-move.err \
  || fail "move failed: $(cat /tmp/wctl-move.err)"
sleep 1.5

./windowctl windows list --json > /tmp/wctl-list-after.json
WCTL_TOL="$WCTL_WAYLAND_TOLERANCE" python3 - <<'PY'
import json, os, sys
tol = int(os.environ['WCTL_TOL'])
with open('/tmp/wctl-list-after.json') as f:
    ws = json.load(f) or []
hit = next((w for w in ws if (w.get('Title') or '').startswith('windowctl : sleep')), None)
if hit is None:
    print('smoke window vanished after move', file=sys.stderr)
    sys.exit(1)
b = hit.get('Bounds') or {}
expected = {'X': 700, 'Y': 500, 'Width': 600, 'Height': 400}
diffs = {k: abs(b.get(k, 0) - expected[k]) for k in expected}
bad = {k: d for k, d in diffs.items() if d > tol}
if bad:
    print(f"Move bounds outside tolerance ({tol}px): got {b}, want {expected}, diffs {diffs}", file=sys.stderr)
    sys.exit(1)
print(f"Move applied within tolerance: bounds={b} (want {expected}, tol {tol}px)")
PY

# 7. Focus, then verify via KWin's own queryWindowInfo that the smoke
#    window really is the active one (the CLI has no "active window"
#    query of its own; qdbus is the ground truth here).
./windowctl focus --title "$SMOKE_TITLE" || fail 'focus failed'
active_caption=$(qdbus6 org.kde.KWin /KWin org.kde.KWin.queryWindowInfo 2>/dev/null | grep '^caption:' || true)
case "$active_caption" in
  *sleep*) echo "Focus applied: active window is '$active_caption'" ;;
  *) fail "focus did not stick: active window is '$active_caption'" ;;
esac

# 8. Monitors: at least one output, primary flagged.
mon_count=$(./windowctl monitors list --json 2>/dev/null | python3 -c '
import json, sys
ms = json.load(sys.stdin) or []
assert any(m.get("Primary") for m in ms), "no primary monitor flagged"
print(len(ms))
' ) || fail 'monitors list failed or has no primary'
echo "Monitors listed: $mon_count"

# 9. Screenshot: capture a region over the moved window; the PNG must
#    be exactly the requested point size (the 1px==1pt normalization
#    invariant the automation loop depends on).
./windowctl screenshot --x 700 --y 500 --w 600 --h 400 --out /tmp/wctl-shot.png \
  || fail 'screenshot failed (is spectacle installed?)'
python3 - <<'PY'
import struct, sys
with open('/tmp/wctl-shot.png', 'rb') as f:
    data = f.read(33)
assert data[:8] == b'\x89PNG\r\n\x1a\n', 'not a PNG'
w, h = struct.unpack('>II', data[16:24])
if (w, h) != (600, 400):
    print(f'screenshot size {w}x{h}, want 600x400 (point normalization broken?)', file=sys.stderr)
    sys.exit(1)
print(f"Screenshot normalized: {w}x{h} = 1 image pixel per coordinate point")
PY

echo '== Smoke test PASSED =='
