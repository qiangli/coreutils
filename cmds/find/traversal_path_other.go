//go:build !linux

package findcmd

import (
	"io/fs"
	"os"
	"path/filepath"

	"github.com/qiangli/coreutils/tool"
)

// Non-Linux platforms retain pathname traversal until an equivalent,
// independently tested descriptor-relative facility is available.
type traversalState struct{}

func newTraversalState(_ string, _ byte, _ fs.FileInfo) *traversalState { return &traversalState{} }
func (*traversalState) close()                                          {}

func (*traversalState) readDir(_ []string, fallback string) ([]os.DirEntry, error) {
	return os.ReadDir(fallback)
}

// The child stats go through tool, which reports the mode chmod set on a
// host that keeps it outside the filesystem — -perm, -executable and -type
// would otherwise match against a mode nobody set. On Linux the
// descriptor-relative traversal beside this file needs no such thing,
// because the filesystem holds the mode itself.
func (*traversalState) lstatChild(_ []string, fallback, name string) (fs.FileInfo, error) {
	return tool.Lstat(filepath.Join(fallback, name))
}

func (*traversalState) statChild(_ []string, fallback, name string) (fs.FileInfo, error) {
	return tool.Stat(filepath.Join(fallback, name))
}
