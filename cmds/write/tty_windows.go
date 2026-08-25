//go:build windows

package writecmd

import (
	"io"

	"github.com/qiangli/coreutils/tool"
)

// Windows consoles have no device path of the utmp `ut_line` shape, so there
// is nothing truthful to name. run() refuses before this matters (see
// platform_windows.go); the function exists so the package compiles and so a
// future Windows path cannot silently inherit a Unix assumption.
func defaultSenderTTY(*tool.RunContext) string { return "" }

func defaultOpenSenderControlTTY(rc *tool.RunContext) io.Writer {
	if rc != nil && rc.Err != nil {
		return rc.Err
	}
	return os.Stderr
}

func defaultGetVEOL(io.Reader) byte { return 0 }

func defaultUnblockIn(io.Reader) {}
