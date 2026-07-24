//go:build !unix

package cpcmd

import "os"

func isSpecial(mode os.FileMode) bool {
	return mode&(os.ModeNamedPipe|os.ModeSocket|os.ModeDevice|os.ModeCharDevice) != 0
}

func (c *copier) copySpecial(src, dst string, fi os.FileInfo) {
	c.errf("cannot copy special file '%s': unsupported on this platform", src)
}
