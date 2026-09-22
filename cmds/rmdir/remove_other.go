//go:build !windows

package rmdircmd

import (
	"os"

	"github.com/qiangli/coreutils/cmds/internal/pathops"
	"github.com/qiangli/coreutils/tool"
)

func removeDirectory(path string, _ os.FileInfo) error { return pathops.Remove(path) }

func terminalDotDotError(_ *tool.RunContext, _ string) (error, bool) { return nil, false }
