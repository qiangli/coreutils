//go:build !unix

package mvcmd

import (
	"errors"
	"os"
)

func preserveOwner(dst string, fi os.FileInfo) error {
	return errors.ErrUnsupported
}

func preserveFileOwner(dst *os.File, fi os.FileInfo) error { return errors.ErrUnsupported }

func preserveLinkOwner(dst string, fi os.FileInfo) error { return errors.ErrUnsupported }
