//go:build !windows

package lncmd

import "os"

// createSymlink is symlink(2): the link content is the operand exactly as
// spelled, and there is no privilege gate to fall back from.
func createSymlink(linkTarget, destPath, _ string) error {
	return os.Symlink(linkTarget, destPath)
}
