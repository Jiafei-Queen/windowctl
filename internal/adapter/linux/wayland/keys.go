//go:build linux

package wayland

import (
	"fmt"

	"github.com/muthuishere/windowctl/internal/core"
)

// evdevKeycodes maps windowctl's normalized key names to Linux evdev
// keycodes (linux/input-event-codes.h), which is the only unit
// `ydotool key` accepts. Unlike xdotool's keysyms, evdev keycodes are
// position-based: the mapping is only correct for a US-layout keyboard
// (ydotool itself is layout-dependent by design — see its README).
//
// Single characters (a-z, 0-9) and the unshifted punctuation runes map
// directly; shifted US-layout symbols (!@#$…) are handled via
// usShiftedSymbols by holding Shift with the base key.
var evdevKeycodes = map[string]int{
	// Letters (KEY_A=30 .. KEY_Z=44)
	"a": 30, "b": 48, "c": 46, "d": 32, "e": 18, "f": 33, "g": 34,
	"h": 35, "i": 23, "j": 36, "k": 37, "l": 38, "m": 50, "n": 49,
	"o": 24, "p": 25, "q": 16, "r": 19, "s": 31, "t": 20, "u": 22,
	"v": 47, "w": 17, "x": 45, "y": 21, "z": 44,
	// Digits (KEY_1=2 .. KEY_0=11)
	"1": 2, "2": 3, "3": 4, "4": 5, "5": 6,
	"6": 7, "7": 8, "8": 9, "9": 10, "0": 11,
	// Function keys (KEY_F1=59 .. KEY_F10=68, F11=87, F12=88)
	"f1": 59, "f2": 60, "f3": 61, "f4": 62, "f5": 63, "f6": 64,
	"f7": 65, "f8": 66, "f9": 67, "f10": 68, "f11": 87, "f12": 88,
	// Named keys
	"enter": 28, "tab": 15, "esc": 1, "space": 57,
	"up": 103, "down": 108, "left": 105, "right": 106,
	"delete": 111, "backspace": 14,
	"home": 102, "end": 107, "pageup": 104, "pagedown": 109,
	// Unshifted punctuation (US layout)
	"-": 12, "=": 13, "[": 26, "]": 27, "\\": 43,
	";": 39, "'": 40, "`": 41,
	",": 51, ".": 52, "/": 53,
}

// usShiftedSymbols maps US-layout shifted punctuation to the unshifted
// key that produces it when Shift is held. PressChord translates these
// into base-key + Shift instead of a direct keycode.
var usShiftedSymbols = map[string]string{
	"!": "1", "@": "2", "#": "3", "$": "4", "%": "5",
	"^": "6", "&": "7", "*": "8", "(": "9", ")": "0",
	"_": "-", "+": "=",
	"{": "[", "}": "]", "|": "\\",
	":": ";", "\"": "'", "~": "`",
	"<": ",", ">": ".", "?": "/",
}

// evdev modifier keycodes: shift=42, ctrl=29, alt=56, meta/super=125.
const (
	keycodeShift = 42
	keycodeCtrl  = 29
	keycodeAlt   = 56
	keycodeSuper = 125
)

// chordKeyEvents renders a Chord as the ydotool key event sequence:
// modifier keycodes down, the key down/up, then modifiers up in
// reverse order (e.g. "125:1 29:1 46:1 46:0 29:0 125:0").
//
// Shifted US punctuation resolves to its base key with Shift added;
// chord.Shift is honored on top of that. Unmapped keys return an
// ErrNotImplemented-wrapped error naming the key.
func chordKeyEvents(chord core.Chord) ([]string, error) {
	key := chord.Key
	shift := chord.Shift
	if base, ok := usShiftedSymbols[key]; ok {
		key = base
		shift = true
	}
	code, ok := evdevKeycodes[key]
	if !ok {
		return nil, fmt.Errorf("windowctl: no evdev keycode for key %q (Wayland input via ydotool is US-layout keycode-based): %w",
			chord.Key, core.ErrNotImplemented)
	}

	// Deterministic order: super, ctrl, alt, shift down — reversed up.
	var down []int
	var up []int
	addMod := func(code int) {
		down = append(down, code)
		up = append([]int{code}, up...)
	}
	if chord.Cmd {
		addMod(keycodeSuper)
	}
	if chord.Ctrl {
		addMod(keycodeCtrl)
	}
	if chord.Alt {
		addMod(keycodeAlt)
	}
	if shift {
		addMod(keycodeShift)
	}

	events := make([]string, 0, len(down)+len(up)+2)
	for _, c := range down {
		events = append(events, fmt.Sprintf("%d:1", c))
	}
	events = append(events, fmt.Sprintf("%d:1", code), fmt.Sprintf("%d:0", code))
	for _, c := range up {
		events = append(events, fmt.Sprintf("%d:0", c))
	}
	return events, nil
}
