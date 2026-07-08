//go:build !windows

package rmdircmd

// Unix: ENOTEMPTY/EEXIST in rmdir.go cover the non-empty case.
func isNonEmptySys(error) bool { return false }
