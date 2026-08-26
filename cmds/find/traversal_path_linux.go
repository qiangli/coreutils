//go:build linux

package findcmd

import (
	"errors"
	"io/fs"
	"os"

	"golang.org/x/sys/unix"
)

// traversalState anchors traversal at the start point. Each operation walks
// from that descriptor with openat, closing the preceding descriptor before
// opening the next component. Descriptor use is therefore bounded regardless
// of depth, and neither PATH_MAX nor /proc availability affects traversal.
type traversalState struct {
	root    *os.File
	rootErr error
	follow  byte
}

func newTraversalState(root string, follow byte, expected fs.FileInfo) *traversalState {
	flags := unix.O_RDONLY | unix.O_DIRECTORY | unix.O_CLOEXEC
	if follow == 'P' {
		flags |= unix.O_NOFOLLOW
	}
	fd, err := unix.Open(root, flags, 0)
	var f *os.File
	if err == nil {
		f = os.NewFile(uintptr(fd), "find-traversal-root")
		opened, statErr := f.Stat()
		if statErr != nil {
			err = statErr
		} else if expected != nil && !os.SameFile(expected, opened) {
			err = errors.New("start point changed during traversal")
		}
		if err != nil {
			_ = f.Close()
			f = nil
		}
	}
	return &traversalState{root: f, rootErr: err, follow: follow}
}

func (s *traversalState) close() {
	if s != nil && s.root != nil {
		_ = s.root.Close()
	}
}

func (s *traversalState) openDir(rel []string) (*os.File, error) {
	if s.rootErr != nil {
		return nil, s.rootErr
	}
	flags := unix.O_RDONLY | unix.O_DIRECTORY | unix.O_CLOEXEC
	fd, err := unix.Openat(int(s.root.Fd()), ".", flags, 0)
	if err != nil {
		return nil, err
	}
	for _, component := range rel {
		componentFlags := flags
		if s.follow != 'L' {
			componentFlags |= unix.O_NOFOLLOW
		}
		next, openErr := unix.Openat(fd, component, componentFlags, 0)
		_ = unix.Close(fd)
		if openErr != nil {
			return nil, openErr
		}
		fd = next
	}
	return os.NewFile(uintptr(fd), "find-openat-directory"), nil
}

func (s *traversalState) readDir(rel []string, _ string) ([]os.DirEntry, error) {
	dir, err := s.openDir(rel)
	if err != nil {
		return nil, err
	}
	defer dir.Close()
	return dir.ReadDir(-1)
}

func (s *traversalState) lstatChild(rel []string, _, name string) (fs.FileInfo, error) {
	return s.statAt(rel, name, true)
}

func (s *traversalState) statChild(rel []string, _, name string) (fs.FileInfo, error) {
	return s.statAt(rel, name, false)
}

func (s *traversalState) statAt(rel []string, name string, noFollow bool) (fs.FileInfo, error) {
	parent, err := s.openDir(rel)
	if err != nil {
		return nil, err
	}
	defer parent.Close()
	flags := unix.O_PATH | unix.O_CLOEXEC
	if noFollow {
		flags |= unix.O_NOFOLLOW
	}
	fd, err := unix.Openat(int(parent.Fd()), name, flags, 0)
	if err != nil {
		return nil, err
	}
	f := os.NewFile(uintptr(fd), "find-openat-entry")
	defer f.Close()
	return f.Stat()
}
