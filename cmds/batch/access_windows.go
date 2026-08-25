//go:build windows

package batchcmd

import "github.com/qiangli/coreutils/tool"

func checkBatchAccess(_ *tool.RunContext) int { return 0 }
