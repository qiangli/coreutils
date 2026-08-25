//go:build unix

package mesgcmd

import (
	"errors"
	"os"

	"github.com/qiangli/coreutils/tool"
	"golang.org/x/term"
)

// defaultTTYName resolves the first terminal attached to standard input,
// standard output, or standard error, in that order, as POSIX mesg requires.
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
