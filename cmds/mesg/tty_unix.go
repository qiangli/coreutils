//go:build unix

package mesgcmd

import (
	"errors"
	"os"

	"github.com/qiangli/coreutils/tool"
	"golang.org/x/term"
)

// defaultTTYName resolves the controlling terminal. POSIX mesg operates on the
// terminal attached to standard input, falling back to stdout and stderr, so a
// caller that redirected only one stream still gets the right device.
func defaultTTYName(rc *tool.RunContext) (string, error) {
	// RunContext streams are the command's standard streams. Reading the
	// process globals here would inspect the embedding shell instead, and a
	// character-device check alone would misclassify /dev/null as a terminal.
	for _, stream := range []any{rc.In, rc.Out, rc.Err} {
		f, ok := stream.(*os.File)
		if !ok || !term.IsTerminal(int(f.Fd())) {
			continue
		}
		if name, err := ttyPath(f); err == nil && name != "" {
			return name, nil
		}
	}
	return "", errors.New("not a terminal")
}
