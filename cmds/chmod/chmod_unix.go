//go:build unix

package chmodcmd

import (
	"fmt"
	"io"
	"os"
	"sync"

	"golang.org/x/sys/unix"

	"github.com/qiangli/coreutils/cmds/internal/hierwalk"
	"github.com/qiangli/coreutils/cmds/internal/rootguard"
	"github.com/qiangli/coreutils/tool"
)

var isFilesystemRoot = rootguard.IsRoot

// changeMode is the transition provider. The production binding is the real
// chmod(2)-equivalent operation; tests may substitute it to cover privilege,
// read-only-filesystem, and late I/O failures without requiring elevated
// credentials or unsafe mounts.
var changeMode = os.Chmod

// chmodOpts is the parsed command line plus the one value only the Unix
// build can supply: the file creation mask an omitted-who clause consults.
type chmodOpts struct {
	options
	umask uint32
}

// apply changes the mode of every operand (and, with -R, everything
// beneath directory operands). The extension traversal flags select the
// physical, command-line, and logical policies; -P/--no-dereference
// leaves a symbolic link alone, because no portable operation changes a
// link's own mode bits.
func apply(rc *tool.RunContext, change *modeChange, o options) int {
	exit := 0
	opts := chmodOpts{options: o, umask: effectiveUmask(rc)}
	opts.verbose = o.verbose || o.changes

	for _, name := range opts.files {
		root := rc.Path(name)
		if opts.recursive && opts.preserveRoot && isFilesystemRoot(root, opts.deref != derefNever) {
			reportRoot(rc, name, root)
			exit = 1
			continue
		}
		if !chmodTree(rc, change, root, name, opts) {
			exit = 1
		}
	}
	return exit
}

// hierMode maps chmod's dereference policy onto the shared traversal
// modes. The two spell the same decision from different sides: chmod
// selects the recursive link policy with -H/-L/-P, and the walker
// consumes it as the POSIX traversal mode.
func (o chmodOpts) hierMode() hierwalk.Mode {
	switch o.deref {
	case derefAlways:
		return hierwalk.Logical
	case derefCmdLine:
		return hierwalk.CommandLine
	default:
		return hierwalk.Physical
	}
}

// chmodTree changes one operand, and with -R the hierarchy below it.
//
// The recursive walk is the shared POSIX walker, which hands a
// directory to Visit only after its entries. That order is what makes
// the -R requirement reachable: the standard says -R shall change the
// directory *and all files in the hierarchy below it*, and a mode that
// removes search permission from a directory would otherwise put that
// directory's own contents out of reach of the very command that was
// told to change them (`chmod -R 000 dir`). Changing children first
// keeps every entry reachable at the moment it is changed.
func chmodTree(rc *tool.RunContext, change *modeChange, root, display string, opts chmodOpts) bool {
	if !opts.recursive {
		return chmodPath(rc, change, root, display, opts, opts.deref != derefNever)
	}
	ok := true
	walker := &hierwalk.Walker{
		Mode:      opts.hierMode(),
		Recursive: true,
		Visit: func(path, name string, isLink, followed bool) {
			// A symbolic link the traversal did not resolve stands for
			// itself, and no portable operation changes a link's own
			// mode bits. The entry is left alone rather than silently
			// redirected to a referent the selected mode says not to
			// reach.
			if isLink && !followed {
				return
			}
			if !chmodPath(rc, change, path, name, opts, true) {
				ok = false
			}
		},
		StatError: func(_, name string, err error) {
			chmodAccessError(rc, name, opts, err)
			ok = false
		},
		ReadError: func(_, name string, err error) {
			if !opts.silent {
				fmt.Fprintf(rc.Err, "chmod: cannot read directory '%s': %v\n", name, reason(err))
			}
			ok = false
		},
		Cycle: func(_, name string) {
			reportCycle(rc, name, opts)
			ok = false
		},
		EnterDir: func(path, name string, followed bool) bool {
			if !opts.preserveRoot || !isFilesystemRoot(path, followed) {
				return true
			}
			reportRoot(rc, name, path)
			ok = false
			return false
		},
	}
	walker.Walk(root, display)
	return ok
}

func reportRoot(rc *tool.RunContext, name, path string) {
	fmt.Fprintf(rc.Err, "chmod: it is dangerous to operate recursively on '%s'%s\n",
		name, rootguard.AliasSuffix(name, path))
	fmt.Fprintf(rc.Err, "chmod: use --no-preserve-root to override this failsafe\n")
}

// reportCycle diagnoses a directory that is its own ancestor without a
// symbolic link having been followed to reach it — a hierarchy that
// cannot be walked to an end.
func reportCycle(rc *tool.RunContext, name string, opts chmodOpts) {
	if opts.silent {
		return
	}
	fmt.Fprintf(rc.Err, "chmod: WARNING: Circular directory structure.\n")
	fmt.Fprintf(rc.Err, "This almost certainly means that you have a corrupted file system.\n")
	fmt.Fprintf(rc.Err, "NOTIFY YOUR SYSTEM ADMINISTRATOR.\n")
	fmt.Fprintf(rc.Err, "The following directory is part of the cycle:\n  %s\n", name)
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
	changed := oldBits != newBits
	// POSIX specifies a chmod() action for every selected file, including
	// when MODE computes to the bits it already has. The native call owns
	// permission checking, ctime, and any other filesystem side effects;
	// treating equality as success would bypass all three.
	if err := changeMode(path, bitsToFileMode(newBits)); err != nil {
		if !opts.silent {
			fmt.Fprintf(rc.Err, "chmod: changing permissions of '%s': %v\n", display, reason(err))
		}
		return false
	}
	chmodVerbose(rc.Out, display, changed, newBits, opts.verbose, opts.changes)
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

var umaskMu sync.Mutex

// effectiveUmask returns the invoking shell's virtual mask when the command is
// embedded. A standalone invocation instead snapshots the inherited process
// mask. The latter operation is process-global, so it is serialized.
func effectiveUmask(rc *tool.RunContext) uint32 {
	if rc.UmaskSet {
		return uint32(rc.Umask.Perm())
	}
	umaskMu.Lock()
	defer umaskMu.Unlock()
	old := unix.Umask(0)
	unix.Umask(old)
	return uint32(old) & 0o777
}
