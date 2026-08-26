//go:build !linux

package findcmd

import (
	"io/fs"
	"os"
	"path/filepath"
)

// Non-Linux platforms retain pathname traversal until an equivalent,
// independently tested descriptor-relative facility is available.
type traversalState struct{}

func newTraversalState(_ string, _ byte, _ fs.FileInfo) *traversalState { return &traversalState{} }
func (*traversalState) close()                                          {}

func (*traversalState) readDir(_ []string, fallback string) ([]os.DirEntry, error) {
	return os.ReadDir(fallback)
}

func (*traversalState) lstatChild(_ []string, fallback, name string) (fs.FileInfo, error) {
	return os.Lstat(filepath.Join(fallback, name))
}

func (*traversalState) statChild(_ []string, fallback, name string) (fs.FileInfo, error) {
	return os.Stat(filepath.Join(fallback, name))
}
