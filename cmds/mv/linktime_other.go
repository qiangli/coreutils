//go:build !aix && !linux && !darwin && !freebsd && !openbsd && !netbsd && !dragonfly && !solaris

package mvcmd

import "os"

func preserveLinkTimes(dst string, fi os.FileInfo) error { return nil }
