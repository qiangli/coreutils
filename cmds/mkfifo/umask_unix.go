//go:build unix

package mkfifocmd

import (
	"sync"

	"github.com/qiangli/coreutils/tool"
	"golang.org/x/sys/unix"
)

var umaskMu sync.Mutex

func effectiveUmask(rc *tool.RunContext) uint32 {
	if rc.UmaskSet {
		return uint32(rc.Umask.Perm())
	}
	// UmaskSet=false is the RunContext contract for a standalone,
	// process-owned invocation. Embedded shells always supply their virtual
	// mask above, so this brief read cannot race an embedding host's work.
	umaskMu.Lock()
	defer umaskMu.Unlock()
	old := unix.Umask(0)
	unix.Umask(old)
	return uint32(old) & 0o777
}
