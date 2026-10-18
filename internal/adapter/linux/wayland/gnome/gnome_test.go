//go:build linux

package gnome

import (
	"testing"

	"github.com/muthuishere/windowctl/internal/core"
)

func TestParseWindows(t *testing.T) {
	payload := `{"ok":true,"windows":[
		{"id":"3","title":"计算器","app":"org.gnome.Calculator","pid":9275,
		 "x":100,"y":100,"width":360,"height":616},
		{"id":"7","title":"Text Editor","app":"org.gnome.TextEditor","pid":42,
		 "x":0,"y":0,"width":100,"height":50}]}`

	ws, err := parseWindows(payload)
	if err != nil {
		t.Fatalf("parseWindows: %v", err)
	}
	if len(ws) != 2 {
		t.Fatalf("got %d windows, want 2", len(ws))
	}
	want := core.Window{
		ID: "3", Title: "计算器", App: "org.gnome.Calculator", PID: 9275,
		Bounds: core.Rect{X: 100, Y: 100, W: 360, H: 616},
	}
	if ws[0] != want {
		t.Errorf("window[0] = %+v, want %+v", ws[0], want)
	}
}

func TestParseWindowsError(t *testing.T) {
	if _, err := parseWindows(`{"ok":false,"error":"boom"}`); err == nil {
		t.Error("expected error for ok=false report")
	}
	if _, err := parseWindows(`not json`); err == nil {
		t.Error("expected error for invalid json")
	}
}

func TestParseMonitors(t *testing.T) {
	payload := `{"ok":true,"monitors":[
		{"x":0,"y":0,"width":2336,"height":1261,"scale":1.25,
		 "primary":true,"focused":true,"active":false},
		{"x":2336,"y":0,"width":1920,"height":1080,"scale":1,
		 "primary":false,"focused":false,"active":true}]}`

	ms, err := parseMonitors(payload)
	if err != nil {
		t.Fatalf("parseMonitors: %v", err)
	}
	if len(ms) != 2 {
		t.Fatalf("got %d monitors, want 2", len(ms))
	}
	want := core.Monitor{
		ID: 1, X: 0, Y: 0, Width: 2336, Height: 1261,
		Primary: true, Focused: true, Active: false,
	}
	if ms[0] != want {
		t.Errorf("monitor[0] = %+v, want %+v", ms[0], want)
	}
	if ms[1].Active != true {
		t.Errorf("monitor[1].Active = %v, want true", ms[1].Active)
	}
}

func TestParseScale(t *testing.T) {
	for _, tc := range []struct {
		payload string
		want    float64
		wantErr bool
	}{
		{`{"ok":true,"scale":2}`, 2, false},
		{`{"ok":true,"scale":1.25}`, 1.25, false},
		// Missing/zero scale falls back to 1 rather than failing.
		{`{"ok":true}`, 1, false},
		{`{"ok":false,"error":"denied"}`, 0, true},
	} {
		got, err := parseScale(tc.payload)
		if (err != nil) != tc.wantErr {
			t.Errorf("parseScale(%s) err = %v, wantErr %v", tc.payload, err, tc.wantErr)
			continue
		}
		if err == nil && got != tc.want {
			t.Errorf("parseScale(%s) = %v, want %v", tc.payload, got, tc.want)
		}
	}
}

func TestIsNotFound(t *testing.T) {
	cases := map[string]bool{
		"window not found: 3": true,
		"window not found: 9": true,
		"":                    false,
		"some other failure":  false,
	}
	for msg, want := range cases {
		if got := isNotFound(msg); got != want {
			t.Errorf("isNotFound(%q) = %v, want %v", msg, got, want)
		}
	}
}

func TestParseStringVariant(t *testing.T) {
	cases := []struct {
		in   string
		want []string
	}{
		{"@as []", []string{}},
		{"[]", []string{}},
		{"['windowctl@local']", []string{"windowctl@local"}},
		{"['a@x', 'b@y']", []string{"a@x", "b@y"}},
		{"garbage", nil},
	}
	for _, tc := range cases {
		got := parseStringVariant(tc.in)
		if tc.want == nil {
			if got != nil {
				t.Errorf("parseStringVariant(%q) = %v, want nil", tc.in, got)
			}
			continue
		}
		if len(got) != len(tc.want) {
			t.Errorf("parseStringVariant(%q) = %v, want %v", tc.in, got, tc.want)
			continue
		}
		for i := range got {
			if got[i] != tc.want[i] {
				t.Errorf("parseStringVariant(%q)[%d] = %q, want %q", tc.in, i, got[i], tc.want[i])
			}
		}
	}
}
