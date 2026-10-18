package windowctl_test

import (
	"fmt"

	"github.com/Jiafei-Queen/windowctl"
)

// Examples in this file double as the importability check for the public
// Go API: they live in package windowctl_test, which means they import
// "github.com/Jiafei-Queen/windowctl" the same way an external consumer
// would. Compilation success demonstrates the requirement from
// docs/specs/07-go-library-api.md that consumers see the documented types
// and functions without pulling in CLI dependencies.

func ExampleListWindows() {
	ws, err := windowctl.ListWindows(windowctl.Filter{Title: "chrome"})
	if err != nil {
		return
	}
	for _, w := range ws {
		fmt.Println(w.Title)
	}
}

func ExampleListMonitors() {
	ms, err := windowctl.ListMonitors()
	if err != nil {
		return
	}
	for _, m := range ms {
		fmt.Printf("monitor %d: %dx%d\n", m.ID, m.Width, m.Height)
	}
}

func ExampleMoveZone() {
	monitorID := 1
	_ = windowctl.MoveZone(
		windowctl.Match{Title: "chrome"},
		&monitorID,
		"2B",
	)
}

func ExampleMoveCoords() {
	_ = windowctl.MoveCoords(
		windowctl.Match{Title: "chrome"},
		nil,
		windowctl.Rect{X: 0, Y: 0, W: 960, H: 1080},
	)
}

func ExampleFocus() {
	_ = windowctl.Focus(windowctl.Match{Title: "jira"})
}
