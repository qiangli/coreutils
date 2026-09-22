//go:build !windows

package localecmd

import "github.com/qiangli/coreutils/tool"

func defaultHostLocaleProvider(*tool.RunContext) *hostLocaleProvider { return nil }
