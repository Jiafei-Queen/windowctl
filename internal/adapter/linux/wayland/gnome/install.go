//go:build linux

package gnome

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"embed"
)

// extensionFS carries the extension source inside the windowctl
// binary, so a GNOME Wayland session needs nothing but the binary
// itself: the first call that finds no live service copies the files
// into place and pre-seeds the enable state. GNOME Shell then picks
// the extension up at the next session start (a one-time logout, see
// the package comment).
//
//go:embed all:extension
var extensionFS embed.FS

// extUUID must match extension/metadata.json's uuid; the directory
// name under the extensions dir must match it too.
const extUUID = "windowctl@local"

var extensionFiles = []string{"metadata.json", "extension.js"}

// extensionsDir returns the user extensions directory GNOME Shell
// scans for per-user extensions.
func extensionsDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".local", "share", "gnome-shell", "extensions", extUUID), nil
}

// ensureInstalled copies the embedded extension into the user
// extensions dir when its files are missing or stale, and pre-seeds
// org.gnome.shell enabled-extensions on a fresh install. Returns
// whether anything was installed (and hence whether the session still
// needs its one-time restart).
func ensureInstalled() (bool, error) {
	dir, err := extensionsDir()
	if err != nil {
		return false, fmt.Errorf("gnome: resolve extensions dir: %w", err)
	}

	stale := false
	for _, name := range extensionFiles {
		want, err := extensionFS.ReadFile("extension/" + name)
		if err != nil {
			return false, fmt.Errorf("gnome: embedded extension %s: %w", name, err)
		}
		got, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil || !bytes.Equal(got, want) {
			stale = true
			break
		}
	}
	if !stale {
		return false, nil
	}

	if err := os.MkdirAll(dir, 0o755); err != nil {
		return false, fmt.Errorf("gnome: create %s: %w", dir, err)
	}
	for _, name := range extensionFiles {
		want, err := extensionFS.ReadFile("extension/" + name)
		if err != nil {
			return false, fmt.Errorf("gnome: embedded extension %s: %w", name, err)
		}
		if err := os.WriteFile(filepath.Join(dir, name), want, 0o644); err != nil {
			return false, fmt.Errorf("gnome: write %s: %w", name, err)
		}
	}
	if err := seedEnabledExtensions(); err != nil {
		return true, fmt.Errorf("gnome: extension files installed but %w", err)
	}
	return true, nil
}

// seedEnabledExtensions appends the extension uuid to
// org.gnome.shell enabled-extensions (preserving any entries already
// there) via the gsettings CLI. Requires the session dconf service —
// reachable from a graphical-session shell, and from SSH with
// DBUS_SESSION_BUS_ADDRESS injected (see gnome-mechanism.md §6).
func seedEnabledExtensions() error {
	out, err := exec.Command("gsettings", "get", "org.gnome.shell", "enabled-extensions").Output()
	if err != nil {
		return fmt.Errorf("read enabled-extensions (is DBUS_SESSION_BUS_ADDRESS set?): %w", err)
	}
	cur := parseStringVariant(strings.TrimSpace(string(out)))
	if cur != nil && containsString(cur, extUUID) {
		return nil
	}
	list := cur
	if list == nil {
		list = []string{}
	}
	list = append(list, extUUID)
	quoted := make([]string, len(list))
	for i, s := range list {
		quoted[i] = "'" + strings.ReplaceAll(s, "'", `\'`) + "'"
	}
	value := "[" + strings.Join(quoted, ", ") + "]"
	if err := exec.Command("gsettings", "set", "org.gnome.shell", "enabled-extensions", value).Run(); err != nil {
		return fmt.Errorf("write enabled-extensions: %w", err)
	}
	return nil
}

// parseStringVariant parses a GVariant string-array rendering as
// produced by `gsettings get`: `@as []` for empty, `['a', 'b']`
// otherwise. Returns nil for anything unparseable.
func parseStringVariant(s string) []string {
	s = strings.TrimPrefix(s, "@as ")
	if !strings.HasPrefix(s, "[") || !strings.HasSuffix(s, "]") {
		return nil
	}
	s = strings.TrimSpace(s[1 : len(s)-1])
	if s == "" {
		return []string{}
	}
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		p = strings.TrimPrefix(p, "'")
		p = strings.TrimSuffix(p, "'")
		if p = strings.ReplaceAll(p, `\'`, "'"); p != "" {
			out = append(out, p)
		}
	}
	return out
}

func containsString(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}
