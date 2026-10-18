//go:build linux

// Package sway is the sway (wlroots) backend for the windowctl
// Wayland adapter.
//
// sway ships a native IPC (the i3 protocol) that answers both queries
// and commands: `swaymsg -t get_tree` enumerates windows with
// geometry, and `swaymsg [con_id=N] <command>` acts on one window.
// The backend shells out to the swaymsg CLI and parses its JSON — no
// D-Bus, no resident helper, unlike the kde/gnome backends.
//
// The exact command spellings are load-bearing and were verified
// against a live sway 1.10.1. In particular the geometry commands take
// container coordinates that include the SSD title bar, while the get_tree `rect` this package
// reports excludes it; Move compensates with deco_rect.height.
package sway

import (
	"bytes"
	"fmt"
	"os/exec"
	"strings"
)

// Backend drives sway through the swaymsg CLI. It is stateless —
// every call is a fresh subprocess run, mirroring the CLI's own
// one-shot model (KDE/GNOME keep connections; sway needs none).
type Backend struct{}

// New builds the sway backend. It touches no process or socket;
// swaymsg availability and reachability surface as errors from the
// first backend call, with the same clear-message contract as the
// other Wayland backends.
func New() *Backend { return &Backend{} }

// Name identifies the compositor this backend drives.
func (b *Backend) Name() string { return "sway" }

// runQuery runs `swaymsg -t <type>` and returns stdout (JSON). It
// fails on a missing binary or non-zero exit with the CLI's own
// message — the most common causes (SWAYSOCK unset / socket stale)
// are stated verbatim by swaymsg there.
func runQuery(treeType string) (string, error) {
	if _, err := exec.LookPath("swaymsg"); err != nil {
		return "", fmt.Errorf("swaymsg not found on PATH — required for window management on sway. "+
			"Install it (Debian: sudo apt install sway; the CLI ships with the compositor): %w", err)
	}
	cmd := exec.Command("swaymsg", "-t", treeType)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("sway: swaymsg -t %s: %w (stderr: %s)", treeType, err, strings.TrimSpace(stderr.String()))
	}
	return stdout.String(), nil
}

// runCommand runs `swaymsg [criteria] args...` — a command, not a
// query. swaymsg prints a structured JSON verdict and exits non-zero
// when the command fails, so the verdicts are evaluated even on a
// non-zero exit: a criteria miss ("No matching node.") must surface as
// core.ErrNoMatch, not a bare exit-status error (caught by the
// fake-bin integration test — the pre-integration code returned the
// raw exec error before parsing anything).
func runCommand(args ...string) error {
	if _, err := exec.LookPath("swaymsg"); err != nil {
		return fmt.Errorf("swaymsg not found on PATH — required for window management on sway. "+
			"Install it (Debian: sudo apt install sway; the CLI ships with the compositor): %w", err)
	}
	cmd := exec.Command("swaymsg", args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	runErr := cmd.Run()

	verdicts, verdictErr := parseVerdicts(stdout.String())
	if verdictErr == nil && len(verdicts) > 0 {
		for _, v := range verdicts {
			if !v.Success {
				return commandError(args, v.Error)
			}
		}
	}
	if runErr != nil {
		return fmt.Errorf("sway: swaymsg %s: %w (stderr: %s)",
			strings.Join(args, " "), runErr, strings.TrimSpace(stderr.String()))
	}
	if verdictErr != nil {
		return fmt.Errorf("sway: swaymsg %s: %w", strings.Join(args, " "), verdictErr)
	}
	return nil
}

// commandError maps a failed sway command verdict to a Go error.
// "No matching node." is the criteria miss — a window that vanished
// (or a stale ID), reported as core.ErrNoMatch to match the darwin
// contract. Everything else surfaces verbatim as context.
func commandError(args []string, msg string) error {
	if msg == noMatchingNode {
		return fmt.Errorf("sway: swaymsg %s: window not found: %w", strings.Join(args, " "), errNoMatch)
	}
	return fmt.Errorf("sway: swaymsg %s failed: %s", strings.Join(args, " "), msg)
}
