//go:build !windows

package telemetry

import (
	"os"

	"golang.org/x/sys/unix"
)

// A PTY harness starts its child as a new session leader so it can attach a
// private controlling terminal. A command launched by an attended interactive
// shell belongs to the shell's existing session. This detects the former even
// when the harness forgot to stamp an environment marker.
func isolatedTerminalSession() bool {
	sid, err := unix.Getsid(0)
	return err == nil && sid == os.Getpid()
}
