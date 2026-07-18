//go:build !windows

package mkdircmd

import (
	"sync"

	"golang.org/x/sys/unix"
)

var umaskMu sync.Mutex

func umask() uint32 {
	umaskMu.Lock()
	defer umaskMu.Unlock()
	old := unix.Umask(0)
	unix.Umask(old)
	return uint32(old) & 0o777
}
