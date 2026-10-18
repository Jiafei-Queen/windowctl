//go:build linux

// Package hyprland is the Hyprland backend for the windowctl Wayland
// adapter.
//
// Hyprland's control channel is its own IPC over a unix socket
// ($XDG_RUNTIME_DIR/hypr/$HYPRLAND_INSTANCE_SIGNATURE/.socket.sock);
// the backend shells out to the hyprctl CLI and parses its JSON —
// same shape as the sway backend, no D-Bus, no resident helper.
//
// The exact command spellings and coordinate semantics were verified
// against a live Hyprland 0.55.2. Notable verified facts: hyprctl
// dispatch failures are reported on stdout with exit code 0, so
// verdicts are parsed from the output
// text; exact-geometry dispatches target the window's logical box
// directly (no title-bar compensation, unlike sway's SSD); resize
// recenters the window, so the move must run last.
package hyprland

import (
	"bytes"
	"fmt"
	"os/exec"
	"strings"
)

// Backend drives Hyprland through the hyprctl CLI. Stateless — every
// call is a fresh subprocess run.
type Backend struct{}

// New builds the Hyprland backend. It touches no socket; hyprctl
// availability and reachability surface as errors from the first
// backend call, with the same clear-message contract as the other
// Wayland backends.
func New() *Backend { return &Backend{} }

// Name identifies the compositor this backend drives.
func (b *Backend) Name() string { return "hyprland" }

// runQuery runs `hyprctl -j <topic>` and returns stdout (JSON).
func runQuery(topic string) (string, error) {
	if _, err := exec.LookPath("hyprctl"); err != nil {
		return "", fmt.Errorf("hyprctl not found on PATH — required for window management on Hyprland. "+
			"Install it (it ships with Hyprland): %w", err)
	}
	cmd := exec.Command("hyprctl", "-j", topic)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("hyprland: hyprctl -j %s: %w (stderr: %s)", topic, err, strings.TrimSpace(stderr.String()))
	}
	return stdout.String(), nil
}

// runDispatch runs `hyprctl dispatch <name> <args...>` and maps the
// verdict to a Go error. hyprctl reports dispatch failures on STDOUT
// with exit code 0 — "ok" means success; "No such window found" is
// the criteria miss (vanished window / stale ID), surfaced as
// core.ErrNoMatch to match the darwin contract; anything else
// surfaces verbatim.
func runDispatch(name string, args ...string) error {
	if _, err := exec.LookPath("hyprctl"); err != nil {
		return fmt.Errorf("hyprctl not found on PATH — required for window management on Hyprland. "+
			"Install it (it ships with Hyprland): %w", err)
	}
	argv := append([]string{"dispatch", name}, args...)
	cmd := exec.Command("hyprctl", argv...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("hyprland: hyprctl %s: %w (stderr: %s)",
			strings.Join(argv, " "), err, strings.TrimSpace(stderr.String()))
	}
	msg := strings.TrimSpace(stdout.String())
	return dispatchVerdict(argv, msg)
}

// dispatchVerdict maps hyprctl's stdout verdict text to a Go error.
func dispatchVerdict(args []string, msg string) error {
	switch {
	case msg == "ok" || msg == "":
		return nil
	case strings.Contains(msg, noSuchWindow):
		return fmt.Errorf("hyprland: hyprctl %s: window not found: %w",
			strings.Join(args, " "), errNoMatch)
	default:
		return fmt.Errorf("hyprland: hyprctl %s failed: %s", strings.Join(args, " "), msg)
	}
}
