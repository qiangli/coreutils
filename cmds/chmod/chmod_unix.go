//go:build unix

package chmodcmd

import (
	"sync"

	"golang.org/x/sys/unix"

	"github.com/qiangli/coreutils/tool"
)

// readOnlyHost: Unix has real mode bits; hostMode passes them through.
const readOnlyHost = false

var umaskMu sync.Mutex

// effectiveUmask returns the invoking shell's virtual mask when the command is
// embedded. A standalone invocation instead snapshots the inherited process
// mask. The latter operation is process-global, so it is serialized.
func effectiveUmask(rc *tool.RunContext) uint32 {
	if rc.UmaskSet {
		return uint32(rc.Umask.Perm())
	}
	umaskMu.Lock()
	defer umaskMu.Unlock()
	old := unix.Umask(0)
	unix.Umask(old)
	return uint32(old) & 0o777
}
