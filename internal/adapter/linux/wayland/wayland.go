//go:build linux

// Package wayland is the Wayland platform adapter for windowctl.
//
// Wayland deliberately gives clients no protocol for managing other
// clients' windows, so this adapter dispatches per compositor: window
// enumeration and geometry control go through each compositor's own
// channel — KWin's D-Bus scripting interface for KDE (kde/), a
// resident GNOME Shell extension exposing a D-Bus service for
// GNOME/Mutter (gnome/), the swaymsg IPC CLI for sway (sway/), and
// hyprctl for Hyprland (hyprland/) — see docs/wayland/design.md §8.
// Input injection (mouse/keyboard) is compositor-independent via
// ydotool's uinput virtual device, so it lives here as shared code.
//
// See docs/specs/17-wayland-adapter.md for the spec.
package wayland

import (
	"fmt"
	"image"
	"os"
	"os/exec"

	"github.com/muthuishere/windowctl/internal/adapter/linux/wayland/gnome"
	"github.com/muthuishere/windowctl/internal/adapter/linux/wayland/hyprland"
	"github.com/muthuishere/windowctl/internal/adapter/linux/wayland/kde"
	"github.com/muthuishere/windowctl/internal/adapter/linux/wayland/sway"
	"github.com/muthuishere/windowctl/internal/core"
)

// Backend is the per-compositor contract the wayland adapter delegates
// to. Everything compositor-specific (enumeration, geometry, capture)
// hangs off it; the input primitives are shared and stay in this
// package.
type Backend interface {
	// Name identifies the compositor this backend drives.
	Name() string
	ListWindows() ([]core.Window, error)
	ListMonitors() ([]core.Monitor, error)
	Move(windowID string, bounds core.Rect) error
	Focus(windowID string) error
	CursorPosition() (int, int, error)
	CaptureRect(bounds core.Rect, outPath string) error
}

// pointerMover is implemented by backends that can position the OS
// cursor through their compositor's own channel. That is the
// preferred MouseMove primitive: ydotool's --absolute emulation (an
// INT32_MIN relative slam + offset, per ydotool Client/tool_mousemove.c)
// runs through libinput pointer acceleration, which on default
// configs distorts the landing (2× on Hyprland 0.55.2, and
// speed-dependent on plain relative moves — see
// hyprland-mechanism.md). Backends without such a channel (KDE/GNOME
// at dpr=1) keep using ydotool.
type pointerMover interface {
	MoveCursor(x, y int) error
}

// Adapter implements core.Adapter for Wayland sessions. backend is nil
// when the detected compositor has no implementation yet — every
// backend-dependent method then fails with a descriptive
// unsupportedCompositor error rather than silently no-oping.
type Adapter struct {
	compositor string
	backend    Backend
}

// New builds the adapter for the current session. It only reads the
// environment; no D-Bus connection or subprocess happens here because
// the public package constructs the adapter at init time.
func New() *Adapter {
	name := detectCompositor()
	switch name {
	case compositorKDE:
		return &Adapter{compositor: name, backend: kde.New()}
	case compositorGNOME:
		return &Adapter{compositor: name, backend: gnome.New()}
	case compositorSway:
		return &Adapter{compositor: name, backend: sway.New()}
	case compositorHyprland:
		return &Adapter{compositor: name, backend: hyprland.New()}
	default:
		return &Adapter{compositor: name}
	}
}

func (a *Adapter) unsupported() error {
	// Name the detected compositor. When nothing was recognized, the
	// raw desktop hint is the closest signal the user has — spec 17's
	// "descriptive error naming the detected compositor" contract.
	name := a.compositor
	if name == compositorUnknown {
		name = os.Getenv("XDG_CURRENT_DESKTOP")
	}
	return fmt.Errorf("no Wayland backend for compositor %q (implemented: KDE Plasma/KWin, GNOME/Mutter, sway, Hyprland): %w",
		name, core.ErrNotImplemented)
}

func (a *Adapter) ListWindows() ([]core.Window, error) {
	if a.backend == nil {
		return nil, a.unsupported()
	}
	return a.backend.ListWindows()
}

func (a *Adapter) ListMonitors() ([]core.Monitor, error) {
	if a.backend == nil {
		return nil, a.unsupported()
	}
	return a.backend.ListMonitors()
}

func (a *Adapter) Move(windowID string, bounds core.Rect) error {
	if a.backend == nil {
		return a.unsupported()
	}
	return a.backend.Move(windowID, bounds)
}

func (a *Adapter) Focus(windowID string) error {
	if a.backend == nil {
		return a.unsupported()
	}
	return a.backend.Focus(windowID)
}

func (a *Adapter) CursorPosition() (int, int, error) {
	if a.backend == nil {
		return 0, 0, a.unsupported()
	}
	return a.backend.CursorPosition()
}

func (a *Adapter) CaptureRect(bounds core.Rect, outPath string) error {
	if a.backend == nil {
		return a.unsupported()
	}
	return a.backend.CaptureRect(bounds, outPath)
}

// Launch execs the app name directly (detached). Wayland has no
// LaunchServices equivalent that resolves pretty names, so the app
// argument is a binary on PATH (or a full path) — same contract as the
// x11 adapter.
func (a *Adapter) Launch(app string) error {
	cmd := exec.Command(app)
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("launch %q: %w", app, err)
	}
	// Detach: don't hold a zombie; the process outlives windowctl.
	go func() { _ = cmd.Wait() }()
	return nil
}

// RequestAccessibility / CheckAccessibility are no-ops on Wayland for
// the same reason they are on X11: there is no per-process trust gate
// comparable to macOS Accessibility. Kept on the interface so
// `windowctl permissions` stays symmetric cross-platform. (The real
// prerequisites on KDE are external tools — spectacle, ydotool — whose
// absence surfaces as errors from the calls that need them.)
func (a *Adapter) RequestAccessibility() error { return nil }
func (a *Adapter) CheckAccessibility() bool    { return true }

// CheckScreenCapture / RequestScreenCapture mirror the pair for the
// macOS Screen Recording permission; likewise no-ops here.
func (a *Adapter) CheckScreenCapture() bool    { return true }
func (a *Adapter) RequestScreenCapture() error { return nil }

// OverlayScale: no HiDPI handling for Wayland coordinates yet — KWin
// exposes devicePixelRatio per output, but the coordinates windowctl
// works in are already logical (scaled) points, matching macOS
// semantics. Only consumed by ShowOverlay, which is unimplemented on
// Wayland, so the values are effectively documentation.
func (a *Adapter) OverlayScale(core.Rect) (float64, float64) { return 1, 1 }

// ShowOverlay is not implemented on Wayland yet: it needs a layer-shell
// client (wlr-layer-shell, wlroots-only; KDE does not implement it).
// RenderDraw still works.
func (a *Adapter) ShowOverlay(core.Rect, *image.RGBA, func() bool) error {
	return core.ErrNotImplemented
}

// AccessibleTexts / RecognizeText: not implemented on Wayland yet
// (AT-SPI over D-Bus and tesseract are the candidates).
func (a *Adapter) AccessibleTexts(string) ([]core.TextItem, error) {
	return nil, core.ErrNotImplemented
}
func (a *Adapter) RecognizeText(core.Rect) ([]core.TextItem, error) {
	return nil, core.ErrNotImplemented
}
