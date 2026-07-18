//go:build windows

package whocmd

import (
	"os"
	"time"

	"github.com/qiangli/coreutils/tool"
	"golang.org/x/sys/windows"
)

func stdinTTY(rc *tool.RunContext) (string, bool) {
	f, ok := rc.In.(*os.File)
	if !ok {
		return "", false
	}
	var mode uint32
	if err := windows.GetConsoleMode(windows.Handle(f.Fd()), &mode); err != nil {
		return "", false
	}
	return "CON", true
}

func accessTime(path string) (time.Time, bool) {
	fi, err := os.Stat(path)
	if err != nil {
		return time.Time{}, false
	}
	return fi.ModTime(), true
}
