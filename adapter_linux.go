//go:build linux

package windowctl

import linuxadapter "github.com/Jiafei-Queen/windowctl/internal/adapter/linux"

func newPlatformAdapter() Adapter { return linuxadapter.New() }
