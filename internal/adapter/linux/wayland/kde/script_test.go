//go:build linux

package kde

import (
	"encoding/json"
	"strings"
	"testing"
)

// TestScriptPreamble pins the preamble contract: the tag lands as a JS
// number, the report helper targets this process's bus name and
// interface, and the payload is tag-prefixed so late callbacks can be
// discarded.
func TestScriptPreamble(t *testing.T) {
	p := scriptPreamble(42)
	if !strings.Contains(p, "var __wctl_tag = 42;") {
		t.Errorf("preamble missing numeric tag:\n%s", p)
	}
	if !strings.Contains(p, `"`+reportIface+`"`) {
		t.Errorf("preamble missing report interface:\n%s", p)
	}
	if !strings.Contains(p, "__wctl_tag + \"|\" + JSON.stringify(obj)") {
		t.Errorf("preamble missing tag-prefixed payload:\n%s", p)
	}
	if !strings.Contains(p, `"`+reportPath+`"`) {
		t.Errorf("preamble missing report path:\n%s", p)
	}
}

// TestJsStringEscaping covers the JSON→JS literal contract, including
// the U+2028/U+2029 escape that JSON allows raw but JS string literals
// (pre-ES2019) do not. Window titles come straight from the compositor
// and can contain anything, so injection safety is load-bearing.
func TestJsStringEscaping(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{`plain`, `"plain"`},
		{`with "quotes"`, `"with \"quotes\""`},
		{"back\\slash", `"back\\slash"`},
		{"new\nline", `"new\nline"`},
		{"tab\tchar", `"tab\tchar"`},
		{"{uuid-with-braces}", `"{uuid-with-braces}"`},
		{"unicode ✓ ünïcödé", `"unicode ✓ ünïcödé"`},
		// The JS-incompatible raw separators must be escaped.
		{"a\u2028b", `"a\u2028b"`},
		{"a\u2029b", `"a\u2029b"`},
		// Template-literal syntax is inert inside a double-quoted
		// literal: backtick/`${}`/single-quote pass through raw (a
		// JSON string is a plain JS string, not a template).
		{"`tpl` ${x} 'sq'", "\"`tpl` ${x} 'sq'\""},
	}
	for _, tc := range cases {
		if got := jsString(tc.in); got != tc.want {
			t.Errorf("jsString(%q) = %s, want %s", tc.in, got, tc.want)
		}
	}
}

// TestJsStringRoundtrip pins the contract that actually matters:
// decoding the rendered literal must recover the original string for
// arbitrary input — the render is only safe if it is reversible.
func TestJsStringRoundtrip(t *testing.T) {
	for _, in := range []string{
		"", `plain`, `a"b`, `c\d`, "e\nf", "\u2028\u2029",
		"`tpl ${x}`", "'sq'", "{uuid}", "日本語 ✓", "\x00 control",
		strings.Repeat(`quote "`, 100),
	} {
		var got string
		if err := json.Unmarshal([]byte(jsString(in)), &got); err != nil {
			t.Fatalf("jsString(%q) is not valid JSON: %v", in, err)
		}
		if got != in {
			t.Errorf("roundtrip %q → %q ≠ original", in, got)
		}
	}
}

// TestMoveBodyInjection verifies the Move script assembles the
// parameters the JS body reads as top-level vars — the var block and
// the body must agree on names, and the UUID must be a JS string.
func TestMoveBodyInjection(t *testing.T) {
	body := "var __wctl_uuid = " + jsString("{abc}") + ";\nvar __wctl_x = 10;\nvar __wctl_y = 20;\nvar __wctl_w = 30;\nvar __wctl_h = 40;\n" + moveBody
	for _, want := range []string{
		`var __wctl_uuid = "{abc}";`,
		"var __wctl_x = 10;",
		"var __wctl_y = 20;",
		"var __wctl_w = 30;",
		"var __wctl_h = 40;",
		// The verified spelling: whole-literal frameGeometry assignment.
		"w.frameGeometry = { x: __wctl_x, y: __wctl_y, width: __wctl_w, height: __wctl_h };",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("assembled move script missing %q:\n%s", want, body)
		}
	}
}

// TestListWindowsBodyShape guards the enumeration contract: z-order
// source, the normalWindow filter, and the braced-UUID ID form — plus
// the desktop-scope narrowing driven by the injected switches.
func TestListWindowsBodyShape(t *testing.T) {
	for _, want := range []string{
		"var so = workspace.stackingOrder;",
		"!w.normalWindow || w.deleted || w.outline",
		"id: String(w.internalId)",
		"title: w.caption",
		"app: w.resourceClass",
		"var g = w.frameGeometry;",
		"workspace.currentDesktop",
		"!w.onAllDesktops",
		"String(ds[j].id) === String(cur.id)",
		"__wctl_skip_minimized && w.minimized",
	} {
		if !strings.Contains(listWindowsBody, want) {
			t.Errorf("listWindowsBody missing %q", want)
		}
	}
}

// TestListWindowsBodyIsFrontmostFirst pins the ordering the public
// layer's stampFocused depends on (it marks the first window lying on
// the focused monitor). stackingOrder is bottom-to-top, so the loop
// must walk it backwards — and the active window must be hoisted to
// the front, because KWin's active window is not necessarily the
// topmost one (verified: a window was active while another sat above
// it in the stacking order).
func TestListWindowsBodyIsFrontmostFirst(t *testing.T) {
	for _, want := range []string{
		"for (var i = so.length - 1; i >= 0; i--)",
		"var active = workspace.activeWindow;",
		"out.unshift(out.splice(k, 1)[0]);",
	} {
		if !strings.Contains(listWindowsBody, want) {
			t.Errorf("listWindowsBody missing %q", want)
		}
	}
	// The ascending walk is the bug this replaced: it made the public
	// layer stamp Focused on the bottom-most window. Only the list
	// body is checked — findWindowBody (prepended to it) legitimately
	// walks stackingOrder forward to look a window up by ID.
	listBody := strings.TrimPrefix(listWindowsBody, findWindowBody)
	for _, bad := range []string{
		"i = 0; i < so.length",
		"i = 0; i < workspace.stackingOrder.length",
	} {
		if strings.Contains(listBody, bad) {
			t.Errorf("listWindowsBody still walks stackingOrder bottom-to-top (%q)", bad)
		}
	}
}

// TestListWindowsPrologueInjection verifies the switches arrive as the
// top-level boolean vars the body reads.
func TestListWindowsPrologueInjection(t *testing.T) {
	if got := listWindowsPrologue(true, false); got != "var __wctl_current_desktop_only = true;\nvar __wctl_skip_minimized = false;\n" {
		t.Errorf("prologue(true,false) = %q", got)
	}
	if got := listWindowsPrologue(false, true); got != "var __wctl_current_desktop_only = false;\nvar __wctl_skip_minimized = true;\n" {
		t.Errorf("prologue(false,true) = %q", got)
	}
}

// TestFocusBodyShape pins the focus mechanism (writable
// workspace.activeWindow).
func TestFocusBodyShape(t *testing.T) {
	if !strings.Contains(focusBody, "workspace.activeWindow = w;") {
		t.Errorf("focusBody missing activeWindow assignment:\n%s", focusBody)
	}
}

// TestMonitorsBodyShape pins the monitor fields, the name-based output
// identity comparison, and the Primary/Active/Focused semantics.
func TestMonitorsBodyShape(t *testing.T) {
	for _, want := range []string{
		"workspace.screens.length",
		"s.devicePixelRatio",
		"s.name === primaryName",
		"workspace.cursorPos",
		"activeW.frameGeometry",
	} {
		if !strings.Contains(monitorsBody, want) {
			t.Errorf("monitorsBody missing %q", want)
		}
	}
}

func TestReportBusNameIsPerProcess(t *testing.T) {
	name := reportBusName()
	if !strings.HasPrefix(name, reportIface+".") {
		t.Fatalf("report bus name %q must start with %q", name, reportIface+".")
	}
	// The PID suffix is what keeps concurrent windowctl processes from
	// fighting over the bus name.
	if strings.Count(name, ".") < 3 {
		t.Fatalf("report bus name %q must carry a pid suffix", name)
	}
}
