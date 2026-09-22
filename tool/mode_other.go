//go:build !windows

package tool

import "os"

func lstat(path string) (os.FileInfo, error) { return os.Lstat(path) }
