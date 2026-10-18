// GNOME Shell extension backing the windowctl Wayland backend.
//
// Wayland gives clients no protocol for managing other clients'
// windows, and Mutter exposes no one-shot D-Bus methods for it either
// (org.gnome.Shell.Screenshot's methods are whitelist-gated to the
// portal and friends). This extension runs inside the Shell process —
// where the Meta window objects and the in-process Shell.Screenshot
// API are reachable — and exposes them as a resident session-bus
// service that windowctl calls directly. docs/wayland/gnome-mechanism.md
// records the verified API spellings and the pitfalls they avoid.
//
// GNOME 45+ extension format (ESM). metadata.json pins shell-version
// to the major version this was verified against (48); see design.md
// §8 for the cross-version strategy.

import Gio from 'gi://Gio';
import GLib from 'gi://GLib';
import Meta from 'gi://Meta';
import Shell from 'gi://Shell';
import St from 'gi://St';

const BUS_NAME = 'org.muthuishere.windowctl.gnome';
const OBJ_PATH = '/org/muthuishere/windowctl/gnome';
const IFACE_NAME = 'org.muthuishere.windowctl.gnome';

// Every reply is a single JSON string in one out arg, with the same
// {ok, ...} / {ok, error} shapes the KDE backend's injected scripts
// report — the Go side parses both with one convention.
const IFACE_XML = `
<node>
  <interface name="${IFACE_NAME}">
    <method name="ListWindows">
      <arg type="s" direction="out"/>
    </method>
    <method name="ListMonitors">
      <arg type="s" direction="out"/>
    </method>
    <method name="CursorPos">
      <arg type="s" direction="out"/>
    </method>
    <method name="Move">
      <arg type="s" direction="in"/>
      <arg type="i" direction="in"/>
      <arg type="i" direction="in"/>
      <arg type="i" direction="in"/>
      <arg type="i" direction="in"/>
      <arg type="s" direction="out"/>
    </method>
    <method name="Focus">
      <arg type="s" direction="in"/>
      <arg type="s" direction="out"/>
    </method>
    <method name="Screenshot">
      <arg type="i" direction="in"/>
      <arg type="i" direction="in"/>
      <arg type="i" direction="in"/>
      <arg type="i" direction="in"/>
      <arg type="s" direction="in"/>
      <arg type="s" direction="out"/>
    </method>
  </interface>
</node>`;

// findWindow resolves a ListWindows-reported ID (the string form of
// Meta.Window.get_stable_sequence()) against the live actor list.
function findWindow(id) {
    for (const actor of global.get_window_actors()) {
        if (String(actor.meta_window.get_stable_sequence()) === id) {
            return actor.meta_window;
        }
    }
    return null;
}

// windowReply mirrors the KDE window report: logical frame geometry in
// the same point space as every other windowctl API.
function windowReply(w) {
    const f = w.get_frame_rect();
    return {
        id: String(w.get_stable_sequence()),
        title: w.get_title(),
        app: w.get_wm_class(),
        pid: w.get_pid(),
        x: Math.round(f.x),
        y: Math.round(f.y),
        width: Math.round(f.width),
        height: Math.round(f.height),
    };
}

function listWindows() {
    // type NORMAL filters docks/panels/dialogs — the same
    // "application windows only" contract as KWin's normalWindow.
    const windows = global.get_window_actors()
        .map(a => a.meta_window)
        .filter(w => w.get_window_type() === Meta.WindowType.NORMAL)
        .map(windowReply);
    return { ok: true, windows };
}

// listMonitors stamps the same Primary/Focused/Active flags the KDE
// script does: Primary from the display, Focused from the focused
// window's monitor, Active from the pointer position. Geometry is
// logical (already scale-adjusted); scale is the per-monitor factor
// CaptureRect needs to convert point rects to device pixels.
function listMonitors() {
    const d = global.display;
    const n = d.get_n_monitors();
    const primary = d.get_primary_monitor();
    const [px, py] = global.get_pointer();
    const fw = d.get_focus_window();
    const focused = fw ? fw.get_monitor() : -1;

    const monitors = [];
    for (let i = 0; i < n; i++) {
        const g = d.get_monitor_geometry(i);
        monitors.push({
            x: Math.round(g.x),
            y: Math.round(g.y),
            width: Math.round(g.width),
            height: Math.round(g.height),
            scale: d.get_monitor_scale(i),
            primary: i === primary,
            focused: i === focused,
            active: g.x <= px && px < g.x + g.width &&
                    g.y <= py && py < g.y + g.height,
        });
    }
    return { ok: true, monitors };
}

function cursorPos() {
    const [x, y] = global.get_pointer();
    return { ok: true, x: Math.round(x), y: Math.round(y) };
}

function move(id, x, y, w, h) {
    const mw = findWindow(id);
    if (!mw) {
        return { ok: false, error: `window not found: ${id}` };
    }
    // user_op=true: geometry change driven by a user-equivalent action.
    // Verified in-process: applies synchronously, frame rect readback
    // is already the new value.
    mw.move_resize_frame(true, x, y, w, h);
    return { ok: true };
}

function focus(id) {
    const mw = findWindow(id);
    if (!mw) {
        return { ok: false, error: `window not found: ${id}` };
    }
    mw.activate(global.get_current_time());
    return { ok: true };
}

// Screenshot captures a point-space rect to path via the in-process
// Shell.Screenshot API. The in-process call takes DEVICE pixels (the
// D-Bus ScreenshotService scales logical coords by the stage's
// scale factor before calling it — mirrored here), and the resulting
// PNG is logical×scale, which the Go side resamples down to point
// size using the scale this reply reports.
async function screenshot(x, y, w, h, path) {
    const scale = St.ThemeContext.get_for_stage(global.stage).scale_factor;
    const stream = Gio.File.new_for_path(path)
        .replace(null, false, Gio.FileCreateFlags.NONE, null);
    try {
        const shot = new Shell.Screenshot();
        await shot.screenshot_area(
            Math.round(x * scale), Math.round(y * scale),
            Math.round(w * scale), Math.round(h * scale), stream);
        stream.close(null);
        return { ok: true, scale };
    } catch (e) {
        return { ok: false, error: String(e) };
    }
}

export default class WindowctlExtension {
    enable() {
        console.log('windowctl: extension enabled, owning ' + BUS_NAME);
        this._ownerId = Gio.bus_own_name(Gio.BusType.SESSION, BUS_NAME,
            Gio.BusNameOwnerFlags.NONE,
            this._onBusAcquired.bind(this), null, null);
    }

    disable() {
        if (this._ownerId) {
            Gio.bus_unown_name(this._ownerId);
            this._ownerId = 0;
        }
    }

    _onBusAcquired(conn) {
        const info = Gio.DBusNodeInfo.new_for_xml(IFACE_XML);
        conn.register_object(OBJ_PATH, info.interfaces[0],
            (c, sender, path, iface, method, params, invocation) => {
                const finish = (obj) =>
                    invocation.return_value(new GLib.Variant('(s)',
                        [JSON.stringify(obj)]));
                try {
                    if (method === 'ListWindows') {
                        finish(listWindows());
                    } else if (method === 'ListMonitors') {
                        finish(listMonitors());
                    } else if (method === 'CursorPos') {
                        finish(cursorPos());
                    } else if (method === 'Move') {
                        const [id, x, y, w, h] = params.deepUnpack();
                        finish(move(id, x, y, w, h));
                    } else if (method === 'Focus') {
                        const [id] = params.deepUnpack();
                        finish(focus(id));
                    } else if (method === 'Screenshot') {
                        // Async (promisified Shell.Screenshot): the
                        // GDBus reply is deferred until the capture
                        // completes, which GDBus allows.
                        const [x, y, w, h, outPath] = params.deepUnpack();
                        screenshot(x, y, w, h, outPath).then(finish);
                    } else {
                        invocation.return_error_literal(
                            Gio.IOErrorEnum, Gio.IOErrorEnum.UNKNOWN,
                            `unknown method ${method}`);
                    }
                } catch (e) {
                    finish({ ok: false, error: String(e) });
                }
            }, null, null);
    }
}
