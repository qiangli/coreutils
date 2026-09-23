//go:build !unix && !windows

package tool

import (
	"errors"
	"os"
	"os/exec"
)

func installOwnedFrame(*exec.Cmd, *os.File) (uint64, func(), error) {
	return 0, nil, errors.ErrUnsupported
}
