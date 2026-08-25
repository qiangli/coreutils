//go:build windows

package writecmd

import (
	"errors"
	"io"
	"os"
	"time"

	"github.com/qiangli/coreutils/tool"
)

// Windows consoles have no device path of the utmp `ut_line` shape, so there
// is nothing truthful to name. run() refuses before this matters (see
// platform_windows.go); the function exists so the package compiles and so a
// future Windows path cannot silently inherit a Unix assumption.
func defaultSenderTTY(*tool.RunContext) string { return "" }

func defaultOpenSenderControlTTY(rc *tool.RunContext) (io.WriteCloser, error) {
	if rc != nil && rc.Err != nil {
		return nopWriteCloser{rc.Err}, nil
	}
	return nopWriteCloser{os.Stderr}, nil
}

func defaultGetVEOL(io.Reader) byte { return 0 }

func duplicateInputFile(*os.File) (*os.File, error) {
	return nil, errors.New("write: input duplication is unavailable on windows")
}

func waitInputReadable(*os.File, time.Duration) (bool, error) { return true, nil }
