//go:build !unix

package atcmd

import "github.com/qiangli/coreutils/tool"

func checkAtAccess(_ *tool.RunContext) int { return 0 }
