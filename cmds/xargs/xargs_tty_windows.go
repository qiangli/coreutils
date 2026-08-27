//go:build windows

package xargscmd

import (
	"fmt"
	"io"
)

// defaultTTYOpener: Windows has no POSIX controlling terminal, and no
// cancellable console-input implementation exists here (see cmds/more's
// tty_windows.go for the repo's established precedent of an explicit
// refusal over an unverified guess). -p fails loudly naming the reason
// instead of silently reading from the wrong place or hanging.
func defaultTTYOpener() (io.ReadCloser, error) {
	return nil, fmt.Errorf("xargs -p (interactive mode) is not supported on Windows: no controlling terminal")
}
