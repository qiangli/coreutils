//go:build !unix

package cpcmd

import "os"

// preserveOwner reports false where POSIX ownership cannot be
// duplicated (windows has no uid/gid). -p still preserves mode and
// timestamps; the caller clears S_ISUID/S_ISGID, which are no-op
// bits on these platforms.
func preserveOwner(dst string, fi os.FileInfo) bool { return false }
