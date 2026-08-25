//go:build !unix

package mvcmd

import "os"

func preserveOwner(dst string, fi os.FileInfo) error {
	return nil
}
