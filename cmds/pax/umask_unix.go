//go:build unix

package paxcmd

import (
	"os"
	"sync"

	"github.com/qiangli/coreutils/tool"
	"golang.org/x/sys/unix"
)

var paxUmaskMu sync.Mutex

func invocationUmask(rc *tool.RunContext) os.FileMode {
	if rc.UmaskSet {
		return rc.Umask.Perm()
	}
	paxUmaskMu.Lock()
	defer paxUmaskMu.Unlock()
	old := unix.Umask(0)
	unix.Umask(old)
	return os.FileMode(old) & os.ModePerm
}
