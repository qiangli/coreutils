// Package rootguard provides filesystem-identity checks for recursive
// commands' --preserve-root failsafes.
package rootguard

import (
	"fmt"
	"os"
	"path/filepath"
)

// SameFile reports whether path and reference identify the same filesystem
// object. followFinal controls whether a final-component symbolic link in
// path is followed; reference is always followed.
//
// An inaccessible operand is not classified as the reference. The command's
// normal access path remains responsible for reporting that error.
func SameFile(path, reference string, followFinal bool) bool {
	stat := os.Lstat
	if followFinal {
		stat = os.Stat
	}
	fi, err := stat(path)
	if err != nil {
		return false
	}
	ref, err := os.Stat(reference)
	if err != nil {
		return false
	}
	return os.SameFile(fi, ref)
}

// IsRoot reports whether path identifies its filesystem volume's root.
func IsRoot(path string, followFinal bool) bool {
	return SameFile(path, RootPath(path), followFinal)
}

// RootPath returns the filesystem volume root against which path is checked.
// It is separate from IsRoot so root derivation can be tested without
// inspecting or operating on the host root.
func RootPath(path string) string {
	return filepath.VolumeName(filepath.Clean(path)) + string(filepath.Separator)
}

// AliasSuffix returns the diagnostic suffix used when operand identifies a
// volume root without being spelled as that literal root.
func AliasSuffix(operand, resolvedPath string) string {
	root := RootPath(resolvedPath)
	if operand == root {
		return ""
	}
	return fmt.Sprintf(" (same as '%s')", root)
}
