//go:build !aix && !linux && !darwin && !freebsd && !openbsd && !netbsd && !dragonfly && !solaris

package mvcmd

import (
	"errors"
	"os"
)

func preserveLinkMode(dst string, fi os.FileInfo) error { return errors.ErrUnsupported }
