//go:build !windows

package tool

import "os"

func lstat(path string) (os.FileInfo, error) { return os.Lstat(path) }

func stat(path string) (os.FileInfo, error) { return os.Stat(path) }
