//go:build !unix

package lncmd

import (
	"errors"
	"os"
)

// Non-Unix targets have no unlink(2). Fail closed for directories before
// using the platform file-removal operation, which must not emulate rmdir.
func unlinkDestination(path string) error {
	fi, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if fi.IsDir() {
		return errors.New("is a directory")
	}
	return os.Remove(path)
}
