//go:build linux

package gnome

import (
	"encoding/json"
	"os"
	"os/exec"
	"strings"
	"testing"
)

// metadata is the schema the extension directory contract depends on.
// extUUID in install.go must equal metadata.json's uuid, and the
// unlock-dialog mode is what keeps the service alive across lock.
type metadata struct {
	UUID         string   `json:"uuid"`
	Name         string   `json:"name"`
	Description  string   `json:"description"`
	ShellVersion []string `json:"shell-version"`
	SessionModes []string `json:"session-modes"`
	Version      int      `json:"version"`
}

func loadMetadata(t *testing.T) metadata {
	t.Helper()
	raw, err := extensionFS.ReadFile("extension/metadata.json")
	if err != nil {
		t.Fatalf("read embedded metadata: %v", err)
	}
	var m metadata
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatalf("metadata.json is not valid JSON: %v", err)
	}
	return m
}

// TestExtensionMetadataSchema pins the fields GNOME Shell's extension
// loading (and this package's install flow) depend on.
func TestExtensionMetadataSchema(t *testing.T) {
	m := loadMetadata(t)

	// The uuid doubles as the extensions-dir name and the D-Bus
	// identity; it must be a valid identifier (letters, digits, @ . _)
	// and match the install flow's constant.
	if m.UUID != extUUID {
		t.Errorf("metadata uuid %q ≠ install flow's extUUID %q", m.UUID, extUUID)
	}
	for _, r := range m.UUID {
		if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || strings.ContainsRune("@._-+", r)) {
			t.Errorf("uuid %q has illegal character %q (GNOME rejects some punctuation)", m.UUID, r)
			break
		}
	}

	if m.Name == "" || m.Description == "" {
		t.Error("name and description must be non-empty")
	}

	// shell-version: at least one supported major version declared,
	// otherwise GNOME Shell disables the extension outright.
	if len(m.ShellVersion) == 0 {
		t.Error("shell-version must declare at least one supported version")
	}

	// session-modes must include unlock-dialog — the whole point of
	// the resident service is surviving screen lock.
	found := false
	for _, mode := range m.SessionModes {
		if mode == "unlock-dialog" {
			found = true
		}
	}
	if !found {
		t.Errorf("session-modes = %v, must include unlock-dialog", m.SessionModes)
	}

	if m.Version <= 0 {
		t.Errorf("version = %d, want a positive integer", m.Version)
	}
}

// TestExtensionJSMethodNames pins that the extension exposes the
// D-Bus methods the Go side calls — keep them in lockstep with
// operations.go's calls.
func TestExtensionJSMethodNames(t *testing.T) {
	raw, err := extensionFS.ReadFile("extension/extension.js")
	if err != nil {
		t.Fatalf("read embedded extension.js: %v", err)
	}
	src := string(raw)
	if len(strings.TrimSpace(src)) == 0 {
		t.Fatal("extension.js is empty")
	}
	// GNOME Shell extensions export init() and can export classes;
	// the service object must expose exactly the methods the Go
	// adapter invokes over D-Bus.
	for _, method := range []string{"ListWindows", "ListMonitors", "Move", "Focus", "CursorPos", "Screenshot"} {
		if !strings.Contains(src, method) {
			t.Errorf("extension.js missing method %q (the Go side calls it over D-Bus)", method)
		}
	}
}

// TestExtensionJSSyntaxValid runs node's syntax checker when node is
// available — the JS is embedded into every linux build and a syntax
// error would only surface as a broken extension at runtime. Skipped
// (not failed) when node is absent, e.g. release runners.
func TestExtensionJSSyntaxValid(t *testing.T) {
	if _, err := exec.LookPath("node"); err != nil {
		t.Skip("node not on PATH — syntax check unavailable")
	}
	path, err := extractExtensionJS(t)
	if err != nil {
		t.Fatalf("extract extension.js: %v", err)
	}
	out, err := exec.Command("node", "--check", path).CombinedOutput()
	if err != nil {
		t.Fatalf("node --check rejected extension.js: %v\n%s", err, out)
	}
}

// extractExtensionJS writes the embedded source to a temp file for
// the external syntax checker.
func extractExtensionJS(t *testing.T) (string, error) {
	t.Helper()
	raw, err := extensionFS.ReadFile("extension/extension.js")
	if err != nil {
		return "", err
	}
	path := t.TempDir() + "/extension.js"
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		return "", err
	}
	return path, nil
}
