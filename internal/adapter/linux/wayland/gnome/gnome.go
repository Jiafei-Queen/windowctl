//go:build linux

// Package gnome is the GNOME (Mutter) backend for the windowctl
// Wayland adapter.
//
// Unlike KWin's one-shot script injection with callDBus callbacks, the
// GNOME side is a resident Shell extension (see ../extension) that owns
// the org.muthuishere.windowctl.gnome bus name on the session bus and
// answers direct method calls, replying JSON strings in a single out
// arg. windowctl therefore only needs a client-side godbus connection;
// there is no callback/tag/timeout machinery of the kde backend here —
// D-Bus itself bounds every call (callTimeout below).
//
// Lifecycle constraint verified on GNOME Shell 48.7: the Shell only
// scans the extensions directory at session start, so a freshly
// installed extension is invisible until the user logs out and back in
// once; after that every session auto-activates it (and the screen
// lock keeps it, via the extension's unlock-dialog session mode).
// When a call finds no live service, connect() installs the embedded
// extension files if missing and turns the failure into an explicit
// "log out and back in once" message — docs/wayland/gnome-mechanism.md
// §2.
package gnome

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/godbus/dbus/v5"
)

const (
	busName   = "org.muthuishere.windowctl.gnome"
	objPath   = "/org/muthuishere/windowctl/gnome"
	ifaceName = "org.muthuishere.windowctl.gnome"

	// callTimeout bounds every method call. The extension runs in the
	// Shell process; anything slower means the session is wedged or
	// locked, mirroring the kde backend's script timeout.
	callTimeout = 5 * time.Second
)

// Backend drives the resident extension over the session bus. The
// connection is established lazily on first use so that constructing
// the backend (which happens at windowctl init) costs nothing.
type Backend struct {
	mu   sync.Mutex
	conn *dbus.Conn
}

// New returns a Backend with no live connection yet.
func New() *Backend { return &Backend{} }

func (b *Backend) Name() string { return "gnome" }

// connect lazily establishes the session-bus connection.
func (b *Backend) connect() (*dbus.Conn, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.conn != nil {
		return b.conn, nil
	}
	conn, err := dbus.ConnectSessionBus()
	if err != nil {
		return nil, fmt.Errorf("gnome: connect to session bus: %w", err)
	}
	b.conn = conn
	return conn, nil
}

// call invokes the extension method with args and returns its JSON
// reply string. A dead or missing service is reported with the
// install/lifecycle context the user needs, after running the
// auto-installer (a no-op when the files are already in place).
func (b *Backend) call(method string, args ...any) (string, error) {
	conn, err := b.connect()
	if err != nil {
		return "", err
	}
	obj := conn.Object(busName, dbus.ObjectPath(objPath))
	ctx, cancel := context.WithTimeout(context.Background(), callTimeout)
	defer cancel()

	var reply string
	err = obj.CallWithContext(ctx, ifaceName+"."+method, 0, args...).Store(&reply)
	if err == nil {
		return reply, nil
	}
	if isServiceUnknown(err) {
		installed, ierr := ensureInstalled()
		switch {
		case ierr != nil:
			return "", fmt.Errorf(
				"gnome: the windowctl extension is not running and auto-install failed: %v (original error: %w)",
				ierr, err)
		case installed:
			return "", fmt.Errorf(
				"gnome: the windowctl extension has been installed — log out and back in once to activate it "+
					"(GNOME Shell only scans the extensions directory at session start): %w", err)
		default:
			return "", fmt.Errorf(
				"gnome: the windowctl extension is installed but not active — log out and back in once if it was "+
					"just installed, or unlock the screen if the session is locked: %w", err)
		}
	}
	return "", fmt.Errorf("gnome: %s: %w", method, err)
}

// isServiceUnknown recognizes the bus daemon's reply for a name nobody
// owns — the extension is not running (not installed, not yet picked
// up, disabled, or the screen is locked).
func isServiceUnknown(err error) bool {
	var derr dbus.Error
	if errors.As(err, &derr) {
		return derr.Name == "org.freedesktop.DBus.Error.ServiceUnknown"
	}
	return false
}
