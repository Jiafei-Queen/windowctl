#!/usr/bin/env bash
#
# macOS per-app smoke matrix for windowctl Move/Focus.
#
# The original `smoke-darwin.sh` only exercises TextEdit, which masked
# real failures: Move/Focus work on Finder / Ghostty / TextEdit but
# fail on Google Chrome and Visual Studio Code with
#
#   "window <id> is gone from the AX tree (its app may have quit
#    before move)"
#
# even though `windows list` clearly reports the same window seconds
# earlier and the app is still on screen. Root cause is in the darwin
# adapter's CGWindowID → AXUIElementRef bridge — the title+bounds
# matcher misses windows in apps where CG and AX disagree on the
# window title (Chrome's renderer host title, Electron-host titles in
# VSCode, etc.).
#
# This script is the reproduction harness. It loops over a fixture of
# apps, attempts Move and Focus on each, and prints a per-app PASS /
# FAIL table. Apps that aren't installed are SKIPPED loudly (so a CI
# matrix with a partial app set still gives useful signal). Exit 1 if
# any installed app fails — this script is meant to FAIL until the AX
# resolver is fixed, then keep failing if it regresses.
#
# Usage:
#   bash scripts/smoke-darwin-apps.sh                    # all apps
#   APPS="Google Chrome,Visual Studio Code" bash ...      # subset
#
# Requires AX permission already granted (run smoke-darwin.sh first
# if unsure). Does not prompt; missing AX is reported as a per-app
# FAIL rather than being silently treated as expected.

set -uo pipefail

cd "$(dirname "$0")/.." || exit 1

echo '== windowctl macOS per-app smoke matrix =='

# Per-app fixture: name|launch-arg|app-filter|detect-substring
# - name:           human-readable label for the report
# - launch-arg:     argument to `open -a` (must match the .app under /Applications)
# - app-filter:     value passed to `windowctl --app` (CG kCGWindowOwnerName,
#                   which differs from the .app filename for some apps —
#                   e.g. VSCode reports owner "Code")
# - detect-match:   substring to look for in App or Title from `windows list`
#
# Add new rows here; the loop is uniform.
DEFAULT_APPS=(
  "TextEdit|TextEdit|TextEdit|TextEdit"
  "Finder|Finder|Finder|Finder"
  "Ghostty|Ghostty|Ghostty|Ghostty"
  "Google Chrome|Google Chrome|Google Chrome|Chrome"
  "Safari|Safari|Safari|Safari"
  "Visual Studio Code|Visual Studio Code|Code|Code"
)

# Optional override: APPS="Google Chrome,Visual Studio Code"
if [ -n "${APPS:-}" ]; then
  IFS=',' read -ra WANT <<< "$APPS"
  FIXTURE=()
  for w in "${WANT[@]}"; do
    found=0
    for row in "${DEFAULT_APPS[@]}"; do
      label="${row%%|*}"
      if [ "$label" = "$w" ]; then
        FIXTURE+=("$row")
        found=1
        break
      fi
    done
    if [ "$found" -eq 0 ]; then
      echo "WARN: app \"$w\" not in fixture; ignoring" >&2
    fi
  done
else
  FIXTURE=("${DEFAULT_APPS[@]}")
fi

# Build CLI once.
go build -o windowctl ./cmd/windowctl

results=()  # "label|status|detail"

# Pre-poll for an existing window of `app_match`. Used so we can decide
# whether to launch (and later, whether to leave the app running). We
# don't kill apps the user already had open.
window_id_for() {
  local match="$1"
  ./windowctl windows list --json | python3 -c "
import json, sys
ws = json.load(sys.stdin)
m = '''$match'''
for w in ws:
    if m in (w.get('App') or '') or m in (w.get('Title') or ''):
        print(w['ID'])
        sys.exit(0)
"
}

# Wait up to 20s for `app_match` to surface a window. Returns ID on
# stdout, empty if none. 20s rather than 10s because Safari's first-
# launch profile/iCloud sync can push the Start Page past 10s on a
# cold Mac; harness should not flake on first run.
wait_for_window() {
  local match="$1"
  local deadline=$(($(date +%s) + 20))
  local id=''
  while [ "$(date +%s)" -lt "$deadline" ]; do
    id=$(window_id_for "$match")
    if [ -n "$id" ]; then
      echo "$id"
      return 0
    fi
    sleep 0.3
  done
  return 1
}

for row in "${FIXTURE[@]}"; do
  IFS='|' read -r label launch_arg app_filter match <<< "$row"
  echo
  echo "--- $label ---"

  # Skip unless installed — `mdfind` is faster than scanning /Applications.
  app_path=$(mdfind "kMDItemKind == 'Application' && kMDItemDisplayName == '${launch_arg}.app'" 2>/dev/null | head -1)
  if [ -z "$app_path" ] && [ ! -d "/Applications/${launch_arg}.app" ]; then
    echo "SKIP: $launch_arg not installed"
    results+=("$label|SKIP|not installed")
    continue
  fi

  was_running=0
  if pgrep -x "$launch_arg" >/dev/null 2>&1; then
    was_running=1
  fi

  # Launch (idempotent — `open -a` is a no-op if already running).
  open -a "$launch_arg" 2>/dev/null || true

  if ! id=$(wait_for_window "$match"); then
    echo "FAIL: $label window did not surface in 10s"
    results+=("$label|FAIL|no window detected")
    continue
  fi
  echo "Detected window ID=$id"

  # Settle window-server state — first-launch apps clobber AX writes
  # during the first ~1-2s.
  sleep 2

  # Move: target a small, safe rectangle on monitor 0. Capture stderr
  # for the failure detail.
  move_err=/tmp/wctl-move-${launch_arg// /_}.err
  if ./windowctl move --app "$app_filter" --monitor 0 --x 100 --y 100 --w 800 --h 600 \
        2>"$move_err"; then
    move_status=PASS
    move_detail='ok'
  else
    move_status=FAIL
    move_detail=$(tr '\n' ' ' < "$move_err" | sed 's/  */ /g')
  fi

  # Focus: regardless of move outcome, exercise focus too — the bug
  # may differ per code path.
  focus_err=/tmp/wctl-focus-${launch_arg// /_}.err
  if ./windowctl focus --app "$app_filter" 2>"$focus_err"; then
    focus_status=PASS
    focus_detail='ok'
  else
    focus_status=FAIL
    focus_detail=$(tr '\n' ' ' < "$focus_err" | sed 's/  */ /g')
  fi

  if [ "$move_status" = PASS ] && [ "$focus_status" = PASS ]; then
    overall=PASS
    detail='move=ok focus=ok'
  else
    overall=FAIL
    detail="move=$move_status [$move_detail] focus=$focus_status [$focus_detail]"
  fi
  echo "$overall: $detail"
  results+=("$label|$overall|$detail")

  # Leave apps the user had open alone; quit ones we launched. Skip
  # Finder regardless — Finder shouldn't be killed.
  if [ "$was_running" -eq 0 ] && [ "$launch_arg" != "Finder" ]; then
    pkill -x "$launch_arg" >/dev/null 2>&1 || true
  fi
done

echo
echo '== Per-app smoke matrix =='
printf '%-22s %-6s %s\n' 'APP' 'RESULT' 'DETAIL'
fail_count=0
for r in "${results[@]}"; do
  IFS='|' read -r label status detail <<< "$r"
  printf '%-22s %-6s %s\n' "$label" "$status" "$detail"
  if [ "$status" = FAIL ]; then
    fail_count=$((fail_count + 1))
  fi
done

if [ "$fail_count" -gt 0 ]; then
  echo
  echo "$fail_count app(s) failed Move/Focus — AX bridge bug." >&2
  exit 1
fi

echo
echo '== All installed apps PASSED =='
