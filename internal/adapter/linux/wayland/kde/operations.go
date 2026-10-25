//go:build linux

package kde

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/Jiafei-Queen/windowctl/internal/core"
)

// The JS bodies below are prepended with the script preamble (tag +
// __wctl_report helper) by Backend.runScript. Every spelling here was
// verified against a live KWin 6.7.4 — do not change any of them without
// re-verifying on KWin.

// findWindowBody is the shared window lookup: match by the braced-UUID
// string form of internalId (what ListWindows reports as Window.ID).
const findWindowBody = `
function __wctl_find(uuid) {
    for (var i = 0; i < workspace.stackingOrder.length; i++) {
        if (String(workspace.stackingOrder[i].internalId) === uuid) {
            return workspace.stackingOrder[i];
        }
    }
    return null;
}
`

// listWindowsBody enumerates normal application windows FRONTMOST
// FIRST, the ordering the public layer's stampFocused relies on (it
// stamps Focused on the first window lying on the focused monitor).
// normalWindow filters panels/docks/OSD chrome — the same
// "application windows only" contract as the darwin adapter's layer-0
// filter; type=DIALOG windows are excluded with it, deliberately.
//
// stackingOrder is bottom-to-top ("后覆盖前"), so the loop walks it
// backwards. On top of that the active window is hoisted to the
// front: workspace.activeWindow is the authoritative focus signal, and
// it is NOT necessarily the topmost window (verified on KWin 6.7.4: a
// window was active while another one sat above it in the stacking
// order), so plain reverse order still stamped Focused on the wrong
// window.
//
// Desktop-scope narrowing is driven by two boolean vars the Go side
// injects ahead of this body (see listWindowsPrologue). The filter
// semantics follow KWin's scripting API: onAllDesktops windows
// (pinned/sticky) always count as current, and desktop membership is
// compared by the VirtualDesktop.id string — not by object identity,
// because QJSEngine may hand out fresh wrapper objects per property
// read (the same trap monitorsBody dodges for screen objects).
const listWindowsBody = findWindowBody + `
function __wctl_main() {
    var cur = workspace.currentDesktop;
    var out = [];
    var so = workspace.stackingOrder;
    for (var i = so.length - 1; i >= 0; i--) {
        var w = so[i];
        if (!w.normalWindow || w.deleted || w.outline) { continue; }
        if (__wctl_current_desktop_only && !w.onAllDesktops) {
            var here = false;
            var ds = w.desktops || [];
            for (var j = 0; j < ds.length; j++) {
                if (String(ds[j].id) === String(cur.id)) { here = true; break; }
            }
            if (!here) { continue; }
        }
        if (__wctl_skip_minimized && w.minimized) { continue; }
        var g = w.frameGeometry;
        out.push({
            id: String(w.internalId),
            title: w.caption,
            app: w.resourceClass,
            pid: w.pid,
            x: Math.round(g.x),
            y: Math.round(g.y),
            width: Math.round(g.width),
            height: Math.round(g.height)
        });
    }
    var active = workspace.activeWindow;
    if (active) {
        var activeId = String(active.internalId);
        for (var k = 0; k < out.length; k++) {
            if (out[k].id === activeId) {
                out.unshift(out.splice(k, 1)[0]);
                break;
            }
        }
    }
    __wctl_report({ ok: true, windows: out });
}
__wctl_main();
`

// moveBody moves/resizes one window. The frameGeometry assignment MUST
// be a whole literal object — mutating a read-back snapshot or writing
// single fields silently does nothing (Wayland geometry changes are
// asynchronous and same-tick reads are stale).
const moveBody = findWindowBody + `
function __wctl_main() {
    var w = __wctl_find(__wctl_uuid);
    if (!w) {
        __wctl_report({ ok: false, error: "window not found: " + __wctl_uuid });
        return;
    }
    w.frameGeometry = { x: __wctl_x, y: __wctl_y, width: __wctl_w, height: __wctl_h };
    __wctl_report({ ok: true });
}
__wctl_main();
`

// focusBody raises and activates one window via the writable
// workspace.activeWindow property.
const focusBody = findWindowBody + `
function __wctl_main() {
    var w = __wctl_find(__wctl_uuid);
    if (!w) {
        __wctl_report({ ok: false, error: "window not found: " + __wctl_uuid });
        return;
    }
    workspace.activeWindow = w;
    __wctl_report({ ok: true });
}
__wctl_main();
`

// monitorsBody reports every output with its geometry (logical,
// already scale-adjusted — same point space as window bounds) and
// devicePixelRatio. Primary is proxied by workspace.activeScreen
// (KWin scripting exposes no "primary output" concept); Focused is the
// output containing the active window's centroid and Active the one
// under the cursor, mirroring what the darwin adapter stamps.
//
// Output identity is compared by name, not object identity: QJSEngine
// may hand out fresh wrapper objects per property read, so === on two
// screen wrappers is not reliable.
const monitorsBody = `
function __wctl_main() {
    var primaryName = null;
    var primary = workspace.activeScreen;
    if (primary) { primaryName = primary.name; }
    var cursor = workspace.cursorPos;
    var activeW = workspace.activeWindow;
    var activeCenter = null;
    if (activeW) {
        var ag = activeW.frameGeometry;
        activeCenter = { x: ag.x + ag.width / 2, y: ag.y + ag.height / 2 };
    }
    var out = [];
    for (var i = 0; i < workspace.screens.length; i++) {
        var s = workspace.screens[i];
        var g = s.geometry;
        var m = {
            name: s.name,
            x: Math.round(g.x), y: Math.round(g.y),
            width: Math.round(g.width), height: Math.round(g.height),
            dpr: s.devicePixelRatio,
            primary: (s.name === primaryName)
        };
        if (activeCenter && activeCenter.x >= g.x && activeCenter.x < g.x + g.width &&
            activeCenter.y >= g.y && activeCenter.y < g.y + g.height) {
            m.focused = true;
        }
        if (cursor && cursor.x >= g.x && cursor.x < g.x + g.width &&
            cursor.y >= g.y && cursor.y < g.y + g.height) {
            m.active = true;
        }
        out.push(m);
    }
    __wctl_report({ ok: true, monitors: out });
}
__wctl_main();
`

// cursorBody reports the pointer position in the same logical point
// space as window bounds.
const cursorBody = `
function __wctl_main() {
    var c = workspace.cursorPos;
    __wctl_report({ ok: true, x: Math.round(c.x), y: Math.round(c.y) });
}
__wctl_main();
`

// listWindowsPrologue injects the desktop-scope switches the list
// body reads as top-level vars. Keeping them out of listWindowsBody
// lets the plain ListWindows path run the same enumeration with both
// switches off.
func listWindowsPrologue(currentDesktopOnly, skipMinimized bool) string {
	return fmt.Sprintf("var __wctl_current_desktop_only = %t;\nvar __wctl_skip_minimized = %t;\n",
		currentDesktopOnly, skipMinimized)
}

func (b *Backend) ListWindows() ([]core.Window, error) {
	return b.listWindows(false, false)
}

// ListWindowsFiltered implements the optional desktop-scope capability
// (core.FilteredWindowLister). Only CurrentDesktopOnly / SkipMinimized
// are honored here — Title / App remain the public layer's job.
//
// TODO: workspace.currentDesktop is the focused screen's active
// desktop. Under KDE's per-screen virtual-desktop mode the precise
// predicate would be currentDesktopForScreen(w.output); revisit if the
// default=all-desktops contract ever needs that precision.
func (b *Backend) ListWindowsFiltered(filter core.Filter) ([]core.Window, error) {
	return b.listWindows(filter.CurrentDesktopOnly, filter.SkipMinimized)
}

func (b *Backend) listWindows(currentDesktopOnly, skipMinimized bool) ([]core.Window, error) {
	payload, err := b.runScript(listWindowsPrologue(currentDesktopOnly, skipMinimized) + listWindowsBody)
	if err != nil {
		return nil, err
	}
	var rep windowReport
	if err := json.Unmarshal([]byte(payload), &rep); err != nil {
		return nil, fmt.Errorf("kde: parse window report: %w", err)
	}
	if !rep.OK {
		return nil, fmt.Errorf("kde: list windows: %s", rep.Error)
	}
	ws := make([]core.Window, 0, len(rep.Windows))
	for _, w := range rep.Windows {
		ws = append(ws, core.Window{
			ID:     w.ID,
			Title:  w.Title,
			App:    w.App,
			PID:    w.PID,
			Bounds: core.Rect{X: w.X, Y: w.Y, W: w.Width, H: w.Height},
		})
	}
	return ws, nil
}

func (b *Backend) Move(windowID string, bounds core.Rect) error {
	body := fmt.Sprintf("var __wctl_uuid = %s;\nvar __wctl_x = %d;\nvar __wctl_y = %d;\nvar __wctl_w = %d;\nvar __wctl_h = %d;\n",
		jsString(windowID), bounds.X, bounds.Y, bounds.W, bounds.H) + moveBody
	payload, err := b.runScript(body)
	if err != nil {
		return err
	}
	var rep report
	if err := json.Unmarshal([]byte(payload), &rep); err != nil {
		return fmt.Errorf("kde: parse move report: %w", err)
	}
	if !rep.OK {
		if isNotFound(rep.Error) {
			return fmt.Errorf("kde: window %s not found (may have been closed): %w", windowID, core.ErrNoMatch)
		}
		return fmt.Errorf("kde: move: %s", rep.Error)
	}
	return nil
}

func (b *Backend) Focus(windowID string) error {
	body := fmt.Sprintf("var __wctl_uuid = %s;\n", jsString(windowID)) + focusBody
	payload, err := b.runScript(body)
	if err != nil {
		return err
	}
	var rep report
	if err := json.Unmarshal([]byte(payload), &rep); err != nil {
		return fmt.Errorf("kde: parse focus report: %w", err)
	}
	if !rep.OK {
		if isNotFound(rep.Error) {
			return fmt.Errorf("kde: window %s not found (may have been closed): %w", windowID, core.ErrNoMatch)
		}
		return fmt.Errorf("kde: focus: %s", rep.Error)
	}
	return nil
}

// isNotFound recognizes the script's "window not found" message so the
// adapter can return core.ErrNoMatch, matching the darwin contract for
// windows that vanished between list and act.
func isNotFound(msg string) bool {
	return strings.HasPrefix(msg, "window not found")
}
