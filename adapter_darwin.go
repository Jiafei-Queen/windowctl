//go:build darwin

package windowctl

import darwinadapter "github.com/Jiafei-Queen/windowctl/internal/adapter/darwin"

func newPlatformAdapter() Adapter { return darwinadapter.New() }
