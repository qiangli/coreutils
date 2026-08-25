//go:build unix

package cpcmd

import (
	"os"
	"sync"

	"github.com/qiangli/coreutils/tool"
	"golang.org/x/sys/unix"
)

var umaskMu sync.Mutex

func invocationUmask(rc *tool.RunContext) os.FileMode {
	if rc.UmaskSet {
		return rc.Umask.Perm()
	}
	// A false UmaskSet denotes the standalone, process-owned invocation.
	// Capture its inherited mask once before cp starts creating anything.
	umaskMu.Lock()
	defer umaskMu.Unlock()
	old := unix.Umask(0)
	unix.Umask(old)
	return os.FileMode(old) & os.ModePerm
}
