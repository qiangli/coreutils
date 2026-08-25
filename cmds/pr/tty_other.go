//go:build !unix && !windows

package prcmd

import (
	"errors"
	"io"
)

// Platforms outside the unix and windows families (js/wasm, plan9) have no
// terminal device this tool can name; fail closed like tty_windows.go.
func defaultOpenControlTTY() (io.ReadCloser, error) {
	return nil, errors.New("controlling terminal (/dev/tty) is not available on this platform")
}
