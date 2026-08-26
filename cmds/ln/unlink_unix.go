//go:build unix

package lncmd

import "golang.org/x/sys/unix"

// unlinkDestination deliberately has unlink(2), not os.Remove, semantics.
// os.Remove retries directories with rmdir, while POSIX ln -f requires the
// destination-removal step to be equivalent to unlink().
func unlinkDestination(path string) error {
	return unix.Unlink(path)
}
