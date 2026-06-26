package tool

import (
	"io/fs"
	"os"
	"time"
)

// LocalFS is a lightweight passthrough virtual filesystem that delegates
// every operation to the local OS filesystem. It provides path translation
// so callers can work with Unix-style paths and the VFS handles the
// platform-level mapping: on Windows, /foo becomes C:\foo for the system
// drive, and C:\foo is presented back as /foo. On Unix it is identity.
//
// LocalFS is a concrete type, not an interface — tools reference it
// directly. The overhead is zero: every method translates paths then calls
// the corresponding os.* function.
type LocalFS struct {
	sysDrive string
}

// NewLocalFS returns a LocalFS with auto-detected system drive.
func NewLocalFS() *LocalFS {
	return &LocalFS{sysDrive: systemDrive()}
}

// SysDrive returns the system-drive root in VFS form ("/" on Unix, the
// drive root like "C:\" on Windows).
func (fs *LocalFS) SysDrive() string {
	return fs.sysDrive
}

// ToOS translates a VFS-style path to the native OS form.
func (fs *LocalFS) ToOS(path string) string {
	return toOSPath(path)
}

// FromOS translates a native OS path to VFS form.
func (fs *LocalFS) FromOS(path string) string {
	return fromOSPath(path)
}

// Open opens the named file for reading.
func (fs *LocalFS) Open(path string) (*os.File, error) {
	return os.Open(toOSPath(path))
}

// Create creates or truncates the named file.
func (fs *LocalFS) Create(path string) (*os.File, error) {
	return os.Create(toOSPath(path))
}

// OpenFile is the generalized open call.
func (fs *LocalFS) OpenFile(path string, flag int, perm fs.FileMode) (*os.File, error) {
	return os.OpenFile(toOSPath(path), flag, perm)
}

// Stat returns the FileInfo structure describing the named file.
func (fs *LocalFS) Stat(path string) (os.FileInfo, error) {
	return os.Stat(toOSPath(path))
}

// Lstat returns the FileInfo for the named file; if it is a symlink the
// returned FileInfo describes the symlink, not the target.
func (fs *LocalFS) Lstat(path string) (os.FileInfo, error) {
	return os.Lstat(toOSPath(path))
}

// ReadDir reads the named directory, returning all its entries sorted by
// filename.
func (fs *LocalFS) ReadDir(path string) ([]os.DirEntry, error) {
	return os.ReadDir(toOSPath(path))
}

// ReadFile reads the named file and returns the contents.
func (fs *LocalFS) ReadFile(path string) ([]byte, error) {
	return os.ReadFile(toOSPath(path))
}

// Mkdir creates a new directory with the specified permission bits.
func (fs *LocalFS) Mkdir(path string, perm fs.FileMode) error {
	return os.Mkdir(toOSPath(path), perm)
}

// MkdirAll creates a directory named path, along with any necessary
// parents.
func (fs *LocalFS) MkdirAll(path string, perm fs.FileMode) error {
	return os.MkdirAll(toOSPath(path), perm)
}

// Remove removes the named file or (empty) directory.
func (fs *LocalFS) Remove(path string) error {
	return os.Remove(toOSPath(path))
}

// RemoveAll removes path and any children it contains.
func (fs *LocalFS) RemoveAll(path string) error {
	return os.RemoveAll(toOSPath(path))
}

// Rename renames (moves) oldpath to newpath.
func (fs *LocalFS) Rename(oldpath, newpath string) error {
	return os.Rename(toOSPath(oldpath), toOSPath(newpath))
}

// Chmod changes the mode of the named file.
func (fs *LocalFS) Chmod(path string, mode fs.FileMode) error {
	return os.Chmod(toOSPath(path), mode)
}

// Chtimes changes the access and modification times of the named file.
func (fs *LocalFS) Chtimes(path string, atime, mtime time.Time) error {
	return os.Chtimes(toOSPath(path), atime, mtime)
}

// Symlink creates newname as a symbolic link to oldname.
func (fs *LocalFS) Symlink(oldname, newname string) error {
	return os.Symlink(toOSPath(oldname), toOSPath(newname))
}

// Readlink returns the destination of the named symbolic link.
func (fs *LocalFS) Readlink(path string) (string, error) {
	target, err := os.Readlink(toOSPath(path))
	if err != nil {
		return "", err
	}
	return fromOSPath(target), nil
}
