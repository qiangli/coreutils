//go:build !unix && !windows

package writecmd

import (
	"io"

	"github.com/qiangli/coreutils/tool"
)

// Platforms outside the unix and windows families (js/wasm, plan9) have no
// terminal device this tool can name. write already refuses on them
// (platform_other.go); this keeps the package building.
func defaultSenderTTY(*tool.RunContext) string { return "" }

func defaultOpenSenderControlTTY(rc *tool.RunContext) io.Writer {
	if rc != nil && rc.Err != nil {
		return rc.Err
	}
	return os.Stderr
}

func defaultGetVEOL(io.Reader) byte { return 0 }

func defaultUnblockIn(io.Reader) {}
