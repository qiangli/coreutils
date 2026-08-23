//go:build windows

package loggercmd

import (
	"errors"

	"github.com/qiangli/coreutils/tool"
)

// Windows has no syslog. The Windows Event Log is a different model with a
// different record shape (it wants a registered event source and message-table
// resources, not a free-text line at a facility.level), so mapping logger onto
// it would invent a record the caller did not ask for and cannot predict.
//
// Refusing loudly is the honest answer: `logger -s` users still get their
// stderr copy from a caller who checks the exit status, and nobody is told a
// message reached a log that never received it.
func dialSystemLog(*tool.RunContext, priority, string) (sink, error) {
	return nil, errors.New("the system log is not available on Windows")
}
