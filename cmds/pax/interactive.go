package paxcmd

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"strings"
)

var errInteractiveRename = errors.New("interactive rename failed")

// interactiveRenamer owns the one read-write controlling-terminal session
// used by -i. Keeping one buffered reader for the entire invocation is
// essential: creating a new reader for every member can read ahead and discard
// later responses already queued by a terminal or scripted PTY.
type interactiveRenamer struct {
	tty io.ReadWriteCloser
	in  *bufio.Reader
}

var openInteractiveTTY = defaultOpenInteractiveTTY

func openInteractiveRenamer() (*interactiveRenamer, error) {
	tty, err := openInteractiveTTY()
	if err != nil {
		return nil, fmt.Errorf("open /dev/tty: %w", err)
	}
	return &interactiveRenamer{tty: tty, in: bufio.NewReader(tty)}, nil
}

// rename prompts after all -s substitutions. A blank response skips the
// member, a single dot retains the substituted name, and every other complete
// line becomes the member's new name. EOF is an error even when preceded by a
// partial response: POSIX requires a line from /dev/tty, not a truncated name.
func (r *interactiveRenamer) rename(name string) (newName string, keep bool, err error) {
	if _, err := fmt.Fprintf(r.tty, "pax: rename %s? ", name); err != nil {
		return "", false, fmt.Errorf("%w: write /dev/tty: %v", errInteractiveRename, err)
	}
	line, err := r.in.ReadString('\n')
	if err != nil {
		return "", false, fmt.Errorf("%w: read /dev/tty: %v", errInteractiveRename, err)
	}
	line = strings.TrimSuffix(line, "\n")
	line = strings.TrimSuffix(line, "\r")
	switch line {
	case "":
		return "", false, nil
	case ".":
		return name, true, nil
	default:
		return line, true, nil
	}
}

func (r *interactiveRenamer) Close() error { return r.tty.Close() }

func renameInteractively(o *options, name string) (string, bool, error) {
	if o.renamer == nil {
		return name, true, nil
	}
	return o.renamer.rename(name)
}
