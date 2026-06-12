//go:build !unix

package cpcmd

import "os"

// preserveOwner is a no-op where POSIX ownership does not apply
// (windows has no uid/gid; -p still preserves mode and timestamps).
func preserveOwner(dst string, fi os.FileInfo) {}
