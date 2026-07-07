//go:build !unix

package mknodcmd

import "errors"

func makeNode(_ string, _ nodeKind, _, _, _ uint32) error {
	return errors.New("not supported on this platform: special files require Unix mknod support")
}
