//go:build !unix

package mvcmd

import (
	"fmt"
	"os"
)

func copySpecialNode(dp string, fi os.FileInfo) error {
	return fmt.Errorf("unsupported special file type")
}
