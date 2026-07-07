//go:build unix

package chmodcmd

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"golang.org/x/sys/unix"

	"github.com/qiangli/coreutils/tool"
)

type derefMode int

const (
	derefNever derefMode = iota
	derefCmdLine
	derefAlways
)

type chmodOpts struct {
	recursive    bool
	verbose      bool
	changes      bool
	silent       bool
	preserveRoot bool
	deref        derefMode
	umask        uint32
}

func computeDeref(followLOrDeref, cmdLineH bool) derefMode {
	if followLOrDeref {
		return derefAlways
	}
	if cmdLineH {
		return derefCmdLine
	}
	return derefNever
}

// apply changes the mode of every operand (and, with -R, everything
// beneath directory operands). Symlink traversal follows GNU's -H/-L/-P
// shape; --no-dereference/-P skip symlink operands because symlink mode
// bits are not changed by POSIX chmod.
func apply(rc *tool.RunContext, change *modeChange, files []string, recursive, verbose, changes, silent, preserveRoot, noDereference, noTraverse, cmdLineH, followLOrDeref bool) int {
	um := umask()
	exit := 0
	opts := chmodOpts{
		recursive:    recursive,
		verbose:      verbose || changes,
		changes:      changes,
		silent:       silent,
		preserveRoot: preserveRoot,
		deref:        computeDeref(followLOrDeref, cmdLineH),
		umask:        um,
	}
	if noDereference || noTraverse {
		opts.deref = derefNever
	}

	for _, name := range files {
		root := rc.Path(name)
		if opts.recursive && opts.preserveRoot && root == "/" {
			fmt.Fprintf(rc.Err, "chmod: it is dangerous to operate recursively on '/'\n")
			fmt.Fprintf(rc.Err, "chmod: use --no-preserve-root to override this failsafe\n")
			exit = 1
			continue
		}
		if !chmodTree(rc, change, root, name, opts) {
			exit = 1
		}
	}
	return exit
}

func chmodTree(rc *tool.RunContext, change *modeChange, root, display string, opts chmodOpts) bool {
	if !opts.recursive {
		return chmodPath(rc, change, root, display, opts, opts.deref != derefNever)
	}
	seen := map[string]bool{}
	return chmodWalk(rc, change, root, display, opts, true, seen)
}

func chmodWalk(rc *tool.RunContext, change *modeChange, path, display string, opts chmodOpts, commandLine bool, seen map[string]bool) bool {
	fi, err := os.Lstat(path)
	if err != nil {
		chmodAccessError(rc, display, opts, err)
		return false
	}
	if fi.Mode()&os.ModeSymlink != 0 {
		follow := opts.deref == derefAlways || (commandLine && opts.deref == derefCmdLine)
		if !follow {
			return true
		}
		fi, err = os.Stat(path)
		if err != nil {
			chmodAccessError(rc, display, opts, err)
			return false
		}
		if !fi.IsDir() {
			return chmodPath(rc, change, path, display, opts, true)
		}
	}

	ok := chmodPath(rc, change, path, display, opts, true)
	if !fi.IsDir() {
		return ok
	}
	real, err := filepath.EvalSymlinks(path)
	if err == nil {
		if seen[real] {
			return ok
		}
		seen[real] = true
	}
	entries, err := os.ReadDir(path)
	if err != nil {
		chmodAccessError(rc, display, opts, err)
		return false
	}
	for _, entry := range entries {
		childDisplay := filepath.Join(display, entry.Name())
		if !chmodWalk(rc, change, filepath.Join(path, entry.Name()), childDisplay, opts, false, seen) {
			ok = false
		}
	}
	return ok
}

func chmodPath(rc *tool.RunContext, change *modeChange, path, display string, opts chmodOpts, follow bool) bool {
	stat := os.Stat
	if !follow {
		stat = os.Lstat
	}
	fi, err := stat(path)
	if err != nil {
		chmodAccessError(rc, display, opts, err)
		return false
	}
	if fi.Mode()&os.ModeSymlink != 0 {
		return true
	}
	oldBits := fileModeToBits(fi.Mode())
	newBits := change.apply(oldBits, fi.IsDir(), opts.umask)
	if oldBits != newBits {
		if err := os.Chmod(path, bitsToFileMode(newBits)); err != nil {
			if !opts.silent {
				fmt.Fprintf(rc.Err, "chmod: changing permissions of '%s': %v\n", display, reason(err))
			}
			return false
		}
		chmodVerbose(rc.Out, display, true, newBits, opts.verbose, opts.changes)
	} else {
		chmodVerbose(rc.Out, display, false, newBits, opts.verbose, opts.changes)
	}
	return true
}

func chmodAccessError(rc *tool.RunContext, name string, opts chmodOpts, err error) {
	if !opts.silent {
		fmt.Fprintf(rc.Err, "chmod: cannot access '%s': %v\n", name, reason(err))
	}
}

func chmodVerbose(out io.Writer, name string, changed bool, newBits uint32, verbose, changes bool) {
	if !verbose {
		return
	}
	if changes && !changed {
		return
	}
	if changed {
		fmt.Fprintf(out, "mode of '%s' changed to %04o\n", name, newBits)
	} else if !changes {
		fmt.Fprintf(out, "mode of '%s' retained as %04o\n", name, newBits)
	}
}

func chmodOne(path string, fi os.FileInfo, change *modeChange, um uint32) error {
	newBits := change.apply(fileModeToBits(fi.Mode()), fi.IsDir(), um)
	return os.Chmod(path, bitsToFileMode(newBits))
}

// umask reads the process umask (set-and-restore — the only portable
// way POSIX offers). Needed for symbolic clauses with no explicit who.
func umask() uint32 {
	old := unix.Umask(0)
	unix.Umask(old)
	return uint32(old) & 0o777
}
