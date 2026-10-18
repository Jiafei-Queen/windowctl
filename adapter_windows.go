//go:build windows

package windowctl

import winadapter "github.com/Jiafei-Queen/windowctl/internal/adapter/windows"

func newPlatformAdapter() Adapter { return winadapter.New() }
