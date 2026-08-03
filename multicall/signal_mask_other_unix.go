//go:build !windows && !darwin && !linux

package multicall

import (
	"os/signal"
	"syscall"
)

// Non-shipping Unix fallback. The release/crossvet matrix exercises the
// kernel-level Darwin and Linux implementations above.
func restoreDefaultAndUnblock(sig syscall.Signal) error {
	signal.Reset(sig)
	return nil
}
