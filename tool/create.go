package tool

import (
	"errors"
	"io/fs"
	"os"
)

// OpenFile opens path like os.OpenFile. When an embedding shell supplied a
// virtual umask and creation is requested, a new file is created atomically at
// a restrictive mode and then changed through its open descriptor to the exact
// requested mode after masking. This avoids both process-global umask races and
// a window where the file is more permissive than requested.
func (rc *RunContext) OpenFile(path string, flag int, perm fs.FileMode) (*os.File, error) {
	if !rc.UmaskSet || flag&os.O_CREATE == 0 {
		return os.OpenFile(path, flag, perm)
	}

	f, err := os.OpenFile(path, flag|os.O_EXCL, 0o600)
	if err == nil {
		if err := f.Chmod(perm &^ rc.Umask.Perm()); err != nil {
			_ = f.Close()
			return nil, err
		}
		return f, nil
	}
	if !errors.Is(err, fs.ErrExist) || flag&os.O_EXCL != 0 {
		return nil, err
	}
	// The operand already existed, so do not change its permissions. Remove
	// O_CREATE to prevent a concurrent unlink from turning this retry into an
	// unmasked creation.
	return os.OpenFile(path, flag&^os.O_CREATE, perm)
}
