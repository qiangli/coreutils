//go:build !linux && !darwin && !freebsd && !openbsd && !netbsd && !dragonfly && !solaris

package mvcmd

import (
	"errors"
	"os"
)

func preserveFileTimes(dst *os.File, fi os.FileInfo) error { return errors.ErrUnsupported }
