//go:build !aix && !darwin && !dragonfly && !freebsd && !linux && !netbsd && !openbsd && !solaris

package lncmd

import "os"

func hardLinkPhysical(targetPath, destPath string) error {
	return os.Link(targetPath, destPath)
}
