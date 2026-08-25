//go:build !unix && !windows

package writecmd

import (
	"errors"
	"io"
	"os"
	"time"

	"github.com/qiangli/coreutils/tool"
)

// Platforms outside the unix and windows families (js/wasm, plan9) have no
// terminal device this tool can name. write already refuses on them
// (platform_other.go); this keeps the package building.
func defaultSenderTTY(*tool.RunContext) string { return "" }

func defaultOpenSenderControlTTY(rc *tool.RunContext, _ string) (io.WriteCloser, error) {
	if rc != nil && rc.Err != nil {
		return nopWriteCloser{rc.Err}, nil
	}
	return nopWriteCloser{os.Stderr}, nil
}

func terminalFile(*os.File) bool                  { return false }
func defaultTerminalDevice(string) bool           { return false }
func defaultSessionActive(int) bool               { return false }
func defaultSessionOwnsTerminal(int, string) bool { return false }

func defaultGetVEOL(io.Reader) byte { return 0 }

func duplicateInputFile(*os.File) (*os.File, error) {
	return nil, errors.New("write: input duplication is unavailable on this platform")
}

func waitInputReadable(*os.File, time.Duration) (bool, error) { return true, nil }
