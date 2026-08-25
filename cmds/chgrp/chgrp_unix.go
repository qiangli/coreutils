//go:build unix

package chgrpcmd

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/user"
	"strconv"
	"strings"
	"syscall"

	"github.com/qiangli/coreutils/cmds/internal/hierwalk"
	"github.com/qiangli/coreutils/cmds/internal/rootguard"
	"github.com/qiangli/coreutils/tool"
)

var isFilesystemRoot = rootguard.IsRoot

// lookupUser and lookupGroup are seams for the name-before-numeric
// resolution order: a host cannot be made to hold an account whose name
// is a decimal number, and that is exactly the case POSIX pins down.
var (
	lookupUser  = user.Lookup
	lookupGroup = user.LookupGroup
)

// changeGroup is the seam for the ownership syscall itself. Only root
// can move a file into a group the caller is not a member of, so tests
// substitute it to observe which call each file takes — chown for a
// referent, lchown for a link. The real syscall path stays covered by
// the self-chgrp tests.
var changeGroup = func(path string, gid int, follow bool) error {
	if follow {
		return os.Chown(path, -1, gid)
	}
	return os.Lchown(path, -1, gid)
}

type chgrpOpts struct {
	options
	gid int
}

func apply(rc *tool.RunContext, spec string, opts options) int {
	gid := opts.refGid
	if !opts.hasRef {
		var err error
		if gid, err = parseGroup(spec); err != nil {
			return statusError(rc, "%v", err)
		}
	}
	state := chgrpOpts{options: opts, gid: gid}
	state.verbose = opts.verbose || opts.changes

	exit := 0
	outputFailed := false
	for _, name := range opts.files {
		path := rc.Path(name)
		// An operand that is a symbolic link is only resolved for the
		// guard when the traversal would resolve it too.
		if opts.recursive && opts.preserveRoot && isFilesystemRoot(path, opts.mode != hierwalk.Physical) {
			reportRoot(rc, name, path)
			exit = 1
			continue
		}
		if !chgrpTree(rc, path, name, state, &outputFailed) {
			exit = 1
		}
	}
	return exit
}

// chgrpTree walks one operand hierarchy. Every file is attempted: a
// failure anywhere sets the exit status without stopping the walk, as
// POSIX requires.
func chgrpTree(rc *tool.RunContext, root, display string, opts chgrpOpts, outputFailed *bool) bool {
	ok := true
	walker := &hierwalk.Walker{
		Mode:      opts.mode,
		Recursive: opts.recursive,
		Visit: func(path, name string, isLink bool) {
			changed, held, statErr, chgrpErr := chgrpOne(path, opts)
			switch {
			case statErr != nil:
				reportUnreachable(rc, name, isLink && opts.affectReferent, opts, statErr)
				ok = false
			case chgrpErr != nil:
				reportChange(rc, name, opts, chgrpErr)
				ok = false
			default:
				// An output failure must affect the status, but ownership work
				// continues for the rest of the hierarchy. Avoid retrying a
				// writer that has already failed and report the error once.
				if !*outputFailed {
					if err := chgrpVerbose(rc.Out, name, changed, held, opts); err != nil {
						fmt.Fprintf(rc.Err, "chgrp: write error: %v\n", err)
						*outputFailed = true
						ok = false
					}
				}
			}
		},
		StatError: func(_, name string, err error) {
			reportUnreachable(rc, name, false, opts, err)
			ok = false
		},
		ReadError: func(_, name string, err error) {
			if !opts.silent {
				fmt.Fprintf(rc.Err, "chgrp: cannot read directory '%s': %v\n", name, unwrapPathError(err))
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

// chgrpOne applies the change to one file. Reading the current
// ownership and changing it fail with different diagnostics, so they
// are returned separately. held is the group the file still has when no
// change was made, which is what the -v report has to name: with
// --from in play that is not the requested group.
func chgrpOne(path string, opts chgrpOpts) (changed bool, held int, statErr, chgrpErr error) {
	// If the platform does not expose syscall.Stat_t, conservatively report
	// that the requested operation changed the file. The syscall is required
	// either way; changed only controls -c/-v reporting.
	changed = true
	stat := os.Lstat
	if opts.affectReferent {
		stat = os.Stat
	}
	fi, err := stat(path)
	if err != nil {
		return false, -1, err, nil
	}
	if st, ok := fi.Sys().(*syscall.Stat_t); ok {
		held = int(st.Gid)
		if opts.fromUid >= 0 && int(st.Uid) != opts.fromUid {
			return false, held, nil, nil
		}
		if opts.fromGid >= 0 && held != opts.fromGid {
			return false, held, nil, nil
		}
		changed = held != opts.gid
	}
	if err := changeGroup(path, opts.gid, opts.affectReferent); err != nil {
		return false, held, nil, err
	}
	return changed, opts.gid, nil, nil
}

// chgrpVerbose reports one file. held is the group the file kept, not
// the group that was asked for: a file --from skipped keeps a group the
// command line never named, and reporting the requested id there would
// state something untrue about the file.
func chgrpVerbose(out io.Writer, name string, changed bool, held int, opts chgrpOpts) error {
	if !opts.verbose {
		return nil
	}
	if opts.changes && !changed {
		return nil
	}
	if changed {
		_, err := fmt.Fprintf(out, "changed group of '%s'\n", name)
		return err
	} else if !opts.changes {
		_, err := fmt.Fprintf(out, "group of '%s' retained as %d\n", name, held)
		return err
	}
	return nil
}

// reportUnreachable diagnoses a file whose current ownership could not
// be read. A symbolic link whose referent is what the change would
// reach gets the dereference wording GNU uses, because the link itself
// is present and the operand is not what is missing.
func reportUnreachable(rc *tool.RunContext, name string, dereferencing bool, opts chgrpOpts, err error) {
	if opts.silent {
		return
	}
	if dereferencing {
		fmt.Fprintf(rc.Err, "chgrp: cannot dereference '%s': %v\n", name, unwrapPathError(err))
		return
	}
	fmt.Fprintf(rc.Err, "chgrp: cannot access '%s': %v\n", name, unwrapPathError(err))
}

func reportChange(rc *tool.RunContext, name string, opts chgrpOpts, err error) {
	if opts.silent {
		return
	}
	fmt.Fprintf(rc.Err, "chgrp: changing group of '%s': %v\n", name, unwrapPathError(err))
}

func reportRoot(rc *tool.RunContext, name, path string) {
	fmt.Fprintf(rc.Err, "chgrp: it is dangerous to operate recursively on '%s'%s\n",
		name, rootguard.AliasSuffix(name, path))
	fmt.Fprintf(rc.Err, "chgrp: use --no-preserve-root to override this failsafe\n")
}

// reportCycle diagnoses a directory that is its own ancestor without a
// symbolic link having been followed to reach it — a hierarchy that
// cannot be walked to an end.
func reportCycle(rc *tool.RunContext, name string, opts chgrpOpts) {
	if opts.silent {
		return
	}
	fmt.Fprintf(rc.Err, "chgrp: WARNING: Circular directory structure.\n")
	fmt.Fprintf(rc.Err, "This almost certainly means that you have a corrupted file system.\n")
	fmt.Fprintf(rc.Err, "NOTIFY YOUR SYSTEM ADMINISTRATOR.\n")
	fmt.Fprintf(rc.Err, "The following directory is part of the cycle:\n  %s\n", name)
}

func unwrapPathError(err error) error {
	var pe *os.PathError
	if errors.As(err, &pe) {
		return pe.Err
	}
	return err
}

// parseGroup resolves the GROUP operand. POSIX requires it to be looked
// up as a group name first and read as a numeric id only when no such
// group exists.
func parseGroup(spec string) (int, error) {
	if g, err := lookupGroup(spec); err == nil {
		gid, cerr := strconv.Atoi(g.Gid)
		if cerr != nil {
			return -1, fmt.Errorf("invalid group: '%s'", spec)
		}
		return gid, nil
	}
	id, err := strconv.Atoi(spec)
	if err != nil || id < 0 {
		return -1, fmt.Errorf("invalid group: '%s'", spec)
	}
	return id, nil
}

func parseFromSpec(spec string) (uid, gid int, err error) {
	uid, gid = -1, -1
	if spec == "" {
		return uid, gid, nil
	}
	ownerStr, groupStr, hasColon := strings.Cut(spec, ":")
	if ownerStr != "" {
		u, uerr := lookupUser(ownerStr)
		switch {
		case uerr == nil:
			if uid, err = strconv.Atoi(u.Uid); err != nil {
				return -1, -1, fmt.Errorf("invalid user: '%s'", spec)
			}
		default:
			id, aerr := strconv.Atoi(ownerStr)
			if aerr != nil || id < 0 {
				return -1, -1, fmt.Errorf("invalid user: '%s'", spec)
			}
			uid = id
		}
	}
	if !hasColon || groupStr == "" {
		return uid, gid, nil
	}
	gid, err = parseGroup(groupStr)
	if err != nil {
		return -1, -1, fmt.Errorf("invalid group: '%s'", spec)
	}
	return uid, gid, nil
}

// statFile reads the --reference file's ids. They are carried as
// numbers, never re-spelled as a GROUP operand: a numeric id must not
// be looked up as a name, or a host with a group literally named "20"
// would silently redirect the change.
func statFile(rc *tool.RunContext, path string) (*refFileInfo, error) {
	fi, err := os.Stat(rc.Path(path))
	if err != nil {
		return nil, err
	}
	st, ok := fi.Sys().(*syscall.Stat_t)
	if !ok {
		return nil, fmt.Errorf("cannot stat %s", path)
	}
	return &refFileInfo{gid: int(st.Gid)}, nil
}

type refFileInfo struct {
	gid int
}

func (r *refFileInfo) ids() (uid, gid int) { return -1, r.gid }

func statusError(rc *tool.RunContext, format string, a ...any) int {
	fmt.Fprintf(rc.Err, "chgrp: "+format+"\n", a...)
	return 1
}
