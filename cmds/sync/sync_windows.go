//go:build windows

package synccmd

import "errors"

// syncAll: Windows has no whole-system sync syscall an unprivileged
// process can issue (FlushFileBuffers needs a handle). Per-FILE fsync
// still works — only the bare invocation is refused.
func syncAll() error {
	return errors.New("not supported on windows: whole-system sync has no Windows equivalent; pass FILE operands to sync specific files")
}
