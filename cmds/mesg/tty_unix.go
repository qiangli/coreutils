//go:build unix

package mesgcmd

import (
	"errors"
	"os"

	"github.com/qiangli/coreutils/tool"
)

// defaultTTYName resolves the controlling terminal. POSIX mesg operates on the
// terminal attached to standard input, falling back to stdout and stderr, so a
// caller that redirected only one stream still gets the right device.
func defaultTTYName(rc *tool.RunContext) (string, error) {
	for _, f := range []*os.File{os.Stdin, os.Stdout, os.Stderr} {
		fi, err := f.Stat()
		if err != nil {
			continue
		}
		if fi.Mode()&os.ModeCharDevice == 0 {
			continue
		}
		if name, err := ttyPath(f); err == nil && name != "" {
			return name, nil
		}
	}
	return "", errors.New("not a terminal")
}
