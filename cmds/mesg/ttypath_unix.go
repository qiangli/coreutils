//go:build unix

package mesgcmd

import (
	"errors"
	"os"
	"path/filepath"
)

// ttyPath maps an open terminal to its device path by matching device numbers
// under /dev. os.File has no portable ttyname(3), and guessing /dev/tty would
// name the wrong device when the caller's streams point elsewhere.
func ttyPath(f *os.File) (string, error) {
	fi, err := f.Stat()
	if err != nil {
		return "", err
	}
	want := deviceOf(fi)
	for _, dir := range []string{"/dev/pts", "/dev"} {
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, e := range entries {
			info, err := e.Info()
			if err != nil || info.Mode()&os.ModeCharDevice == 0 {
				continue
			}
			if deviceOf(info) == want {
				return filepath.Join(dir, e.Name()), nil
			}
		}
	}
	return "", errors.New("cannot resolve terminal device")
}
