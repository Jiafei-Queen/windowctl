//go:build linux

// Package kde is the KDE Plasma (KWin) backend for the windowctl
// Wayland adapter.
//
// KWin exposes no one-shot D-Bus methods for window management; the
// only channel is its scripting interface: a JavaScript file is
// injected via org.kde.kwin.Scripting.loadScript, executed inside the
// compositor process (where workspace.stackingOrder, frameGeometry and
// workspace.activeWindow are writable), and reports its result back
// through callDBus to a session-bus name this process owns.
//
// gdbus / dbus-send can only place calls — they cannot own bus names —
// so receiving the callback requires a D-Bus client library, hence the
// github.com/godbus/dbus/v5 dependency (pure Go, no CGO).
//
// The exact JS spellings are load-bearing and were verified against a
// live KWin 6.7.4. In particular:
// frameGeometry must be assigned a whole literal object
// ({x, y, width, height}) — mutating a read-back snapshot or writing
// single fields does not work.
package kde

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/godbus/dbus/v5"
)

const (
	kwinService    = "org.kde.KWin"
	scriptingPath  = "/Scripting"
	scriptingIface = "org.kde.kwin.Scripting"
	scriptIface    = "org.kde.kwin.Script"

	// reportIface / reportPath identify the D-Bus service the injected
	// scripts call back into; the bus name is reportIface + ".<pid>"
	// so concurrent windowctl processes never collide.
	reportIface = "org.muthuishere.windowctl.kde"
	reportPath  = "/org/muthuishere/windowctl/kde"

	// scriptTimeout bounds the wait for a script's callDBus callback.
	// KWin scripts are synchronous inside the compositor, so anything
	// slower means KWin is wedged or the session is locked.
	scriptTimeout = 5 * time.Second
)

// Backend drives KWin through one-shot injected scripts. The D-Bus
// connection and bus name are established lazily on first use so that
// constructing the backend (which happens at windowctl init) costs
// nothing.
type Backend struct {
	mu     sync.Mutex
	conn   *dbus.Conn
	report chan scriptResult // waiter for the in-flight call
	seq    int               // monotonic call tag
}

// New returns a Backend with no live connection yet.
func New() *Backend { return &Backend{} }

// scriptResult is one callback delivery: the call's tag and the raw
// JSON payload the script reported.
type scriptResult struct {
	tag     int
	payload string
}

func (b *Backend) Name() string { return "kde" }

// reportObject is the D-Bus object the injected scripts call. It is
// exported once per Backend, right after the bus name is acquired.
type reportObject struct {
	backend *Backend
}

// Report receives "<tag>|<json>" from an injected script and routes it
// to the matching runScript waiter. Payloads whose tag does not match
// the in-flight call are late callbacks from timed-out calls and are
// dropped.
func (o *reportObject) Report(tagged string) *dbus.Error {
	tagStr, payload, ok := strings.Cut(tagged, "|")
	if !ok {
		return nil
	}
	tag, err := strconv.Atoi(tagStr)
	if err != nil {
		return nil
	}
	o.backend.deliver(scriptResult{tag: tag, payload: payload})
	return nil
}

// deliver hands a callback to the in-flight call, dropping stale tags
// (from previously timed-out calls) so they cannot corrupt the next
// call's result.
func (b *Backend) deliver(res scriptResult) {
	b.mu.Lock()
	ch := b.report
	cur := b.seq
	b.mu.Unlock()
	if ch == nil || res.tag != cur {
		return
	}
	select {
	case ch <- res:
	default:
	}
}

// connect lazily establishes the session-bus connection, exports the
// report object, and owns the per-process bus name.
func (b *Backend) connect() (*dbus.Conn, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.conn != nil {
		return b.conn, nil
	}
	conn, err := dbus.ConnectSessionBus()
	if err != nil {
		return nil, fmt.Errorf("kde: connect to session bus: %w", err)
	}
	if err := conn.Export(&reportObject{backend: b}, dbus.ObjectPath(reportPath), reportIface); err != nil {
		conn.Close()
		return nil, fmt.Errorf("kde: export report object: %w", err)
	}
	name := reportBusName()
	if reply, err := conn.RequestName(name, dbus.NameFlagDoNotQueue); err != nil || reply != dbus.RequestNameReplyPrimaryOwner {
		conn.Close()
		return nil, fmt.Errorf("kde: own bus name %s: %v (reply %v)", name, err, reply)
	}
	b.conn = conn
	return conn, nil
}

func reportBusName() string {
	// D-Bus name elements must not start with a digit, hence the "p"
	// prefix on the pid suffix.
	return reportIface + ".p" + strconv.Itoa(os.Getpid())
}

// runScript injects body into KWin and waits for its callDBus callback,
// returning the reported JSON payload. The preamble (tag + report
// helper) is prepended by this method so bodies stay pure logic.
func (b *Backend) runScript(body string) (string, error) {
	conn, err := b.connect()
	if err != nil {
		return "", err
	}

	b.mu.Lock()
	b.seq++
	tag := b.seq
	ch := make(chan scriptResult, 1)
	b.report = ch
	b.mu.Unlock()
	defer func() {
		b.mu.Lock()
		b.report = nil
		b.mu.Unlock()
	}()

	dir, err := os.MkdirTemp("", "windowctl-kwin-")
	if err != nil {
		return "", fmt.Errorf("kde: create temp dir: %w", err)
	}
	defer os.RemoveAll(dir)
	path := filepath.Join(dir, "script.js")
	if err := os.WriteFile(path, []byte(scriptPreamble(tag)+body), 0o600); err != nil {
		return "", fmt.Errorf("kde: write script: %w", err)
	}

	name := fmt.Sprintf("windowctl-p%d-%d", os.Getpid(), tag)
	kwin := conn.Object(kwinService, scriptingPath)
	var scriptID int32
	if err := kwin.Call(scriptingIface+".loadScript", 0, path, name).Store(&scriptID); err != nil {
		return "", fmt.Errorf("kde: loadScript: %w", err)
	}
	if scriptID < 0 {
		// A stale script with this name is still loaded (e.g. a
		// previous run died between load and unload). Unload and retry
		// once before giving up.
		if err := kwin.Call(scriptingIface+".unloadScript", 0, name).Store(); err != nil {
			return "", fmt.Errorf("kde: unloadScript(%s) after loadScript -1: %w", name, err)
		}
		if err := kwin.Call(scriptingIface+".loadScript", 0, path, name).Store(&scriptID); err != nil || scriptID < 0 {
			return "", fmt.Errorf("kde: loadScript retry failed (id %d): %v", scriptID, err)
		}
	}
	scriptObj := conn.Object(kwinService, dbus.ObjectPath(scriptingPath+"/Script"+strconv.Itoa(int(scriptID))))
	if err := scriptObj.Call(scriptIface+".run", 0).Store(); err != nil {
		_ = kwin.Call(scriptingIface+".unloadScript", 0, name).Store()
		return "", fmt.Errorf("kde: script run: %w", err)
	}

	select {
	case res := <-ch:
		_ = scriptObj.Call(scriptIface+".stop", 0).Store()
		_ = kwin.Call(scriptingIface+".unloadScript", 0, name).Store()
		if res.tag != tag {
			return "", fmt.Errorf("kde: script callback tag mismatch (got %d, want %d)", res.tag, tag)
		}
		return res.payload, nil
	case <-time.After(scriptTimeout):
		_ = scriptObj.Call(scriptIface+".stop", 0).Store()
		_ = kwin.Call(scriptingIface+".unloadScript", 0, name).Store()
		return "", fmt.Errorf("kde: script did not report back within %s (KWin busy or session locked)", scriptTimeout)
	}
}

// scriptPreamble renders the JS preamble every injected script gets:
// the call tag and the report helper. Bodies below assume __wctl_report
// exists.
func scriptPreamble(tag int) string {
	return fmt.Sprintf(`var __wctl_tag = %d;
function __wctl_report(obj) {
    callDBus(%s, %s, %s, "Report", __wctl_tag + "|" + JSON.stringify(obj));
}
`, tag, jsString(reportBusName()), jsString(reportPath), jsString(reportIface))
}

// jsString renders s as a JavaScript string literal. JSON encoding is
// a subset of JS string syntax; U+2028/U+2029 (line/paragraph
// separators, legal raw in JSON) are the one incompatibility — they
// are syntax errors in JS string literals — so they get escaped.
func jsString(s string) string {
	b, _ := json.Marshal(s)
	out := string(b)
	out = strings.ReplaceAll(out, "\u2028", "\\u2028")
	out = strings.ReplaceAll(out, "\u2029", "\\u2029")
	return out
}

// --- report shapes -----------------------------------------------------

type report struct {
	OK    bool   `json:"ok"`
	Error string `json:"error"`
}

type windowReport struct {
	report
	Windows []struct {
		ID     string `json:"id"`
		Title  string `json:"title"`
		App    string `json:"app"`
		PID    int    `json:"pid"`
		X      int    `json:"x"`
		Y      int    `json:"y"`
		Width  int    `json:"width"`
		Height int    `json:"height"`
		Active bool   `json:"active"`
	} `json:"windows"`
}

type monitorReport struct {
	report
	Monitors []struct {
		Name    string  `json:"name"`
		X       int     `json:"x"`
		Y       int     `json:"y"`
		Width   int     `json:"width"`
		Height  int     `json:"height"`
		DPR     float64 `json:"dpr"`
		Primary bool    `json:"primary"`
		Focused bool    `json:"focused"`
		Active  bool    `json:"active"`
	} `json:"monitors"`
}

type cursorReport struct {
	report
	X int `json:"x"`
	Y int `json:"y"`
}
