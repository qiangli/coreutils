//go:build unix

package chowncmd

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

// changeOwner is the seam for the ownership syscall itself. Only root
// can change a file to another owner, so tests substitute it to observe
// which call each file takes — chown for a referent, lchown for a link
// — and to prove no call is issued when nothing would change. The real
// syscall path stays covered by the self-chown tests.
var changeOwner = func(path string, uid, gid int, follow bool) error {
	if follow {
		return os.Chown(path, uid, gid)
	}
	return os.Lchown(path, uid, gid)
}

type chownOpts struct {
	options
	uid int
	gid int
}

func apply(rc *tool.RunContext, spec string, opts options) int {
	uid, gid := opts.refUid, opts.refGid
	if !opts.hasRef {
		var err error
		if uid, gid, err = parseSpec(spec); err != nil {
			return statusError(rc, "%v", err)
		}
	}
	state := chownOpts{options: opts, uid: uid, gid: gid}
	state.verbose = opts.verbose || opts.changes

	exit := 0
	for _, name := range opts.files {
		path := rc.Path(name)
		// An operand that is a symbolic link is only resolved for the
		// guard when the traversal would resolve it too.
		if opts.recursive && opts.preserveRoot && isFilesystemRoot(path, opts.mode != hierwalk.Physical) {
			reportRoot(rc, name, path)
			exit = 1
			continue
		}
		if !chownTree(rc, path, name, state) {
			exit = 1
		}
	}
	return exit
}

// chownTree walks one operand hierarchy. Every file is attempted: a
// failure anywhere sets the exit status without stopping the walk, as
// POSIX requires.
func chownTree(rc *tool.RunContext, root, display string, opts chownOpts) bool {
	ok := true
	walker := &hierwalk.Walker{
		Mode:      opts.mode,
		Recursive: opts.recursive,
		Visit: func(path, name string, isLink bool) {
			changed, statErr, chownErr := chownOne(path, opts)
			switch {
			case statErr != nil:
				reportUnreachable(rc, name, isLink && opts.affectReferent, opts, statErr)
				ok = false
			case chownErr != nil:
				reportChange(rc, name, opts, chownErr)
				ok = false
			default:
				chownVerbose(rc.Out, name, changed, opts)
			}
		},
		StatError: func(_, name string, err error) {
			reportUnreachable(rc, name, false, opts, err)
			ok = false
		},
		ReadError: func(_, name string, err error) {
			if !opts.silent {
				fmt.Fprintf(rc.Err, "chown: cannot read directory '%s': %v\n", name, unwrapPathError(err))
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

// chownOne applies the change to one file. Reading the current
// ownership and changing it fail with different diagnostics, so they
// are returned separately.
func chownOne(path string, opts chownOpts) (changed bool, statErr, chownErr error) {
	stat := os.Lstat
	if opts.affectReferent {
		stat = os.Stat
	}
	fi, err := stat(path)
	if err != nil {
		return false, err, nil
	}
	if st, ok := fi.Sys().(*syscall.Stat_t); ok {
		if opts.fromUid >= 0 && int(st.Uid) != opts.fromUid {
			return false, nil, nil
		}
		if opts.fromGid >= 0 && int(st.Gid) != opts.fromGid {
			return false, nil, nil
		}
		// chown(2) clears the set-user-ID and set-group-ID bits of an
		// executable it changes, even when the ids it writes are the
		// ones already there. A file that already has the requested
		// ownership must therefore be left alone entirely.
		wanted := (opts.uid >= 0 && int(st.Uid) != opts.uid) ||
			(opts.gid >= 0 && int(st.Gid) != opts.gid)
		if !wanted {
			return false, nil, nil
		}
	}
	if err := changeOwner(path, opts.uid, opts.gid, opts.affectReferent); err != nil {
		return false, nil, err
	}
	return true, nil, nil
}

func chownVerbose(out io.Writer, name string, changed bool, opts chownOpts) {
	if !opts.verbose {
		return
	}
	if opts.changes && !changed {
		return
	}
	if changed {
		fmt.Fprintf(out, "changed ownership of '%s'\n", name)
	} else if !opts.changes {
		fmt.Fprintf(out, "ownership of '%s' retained\n", name)
	}
}

// reportUnreachable diagnoses a file whose current ownership could not
// be read. A symbolic link whose referent is what the change would
// reach gets the dereference wording GNU uses, because the link itself
// is present and the operand is not what is missing.
func reportUnreachable(rc *tool.RunContext, name string, dereferencing bool, opts chownOpts, err error) {
	if opts.silent {
		return
	}
	if dereferencing {
		fmt.Fprintf(rc.Err, "chown: cannot dereference '%s': %v\n", name, unwrapPathError(err))
		return
	}
	fmt.Fprintf(rc.Err, "chown: cannot access '%s': %v\n", name, unwrapPathError(err))
}

func reportChange(rc *tool.RunContext, name string, opts chownOpts, err error) {
	if opts.silent {
		return
	}
	fmt.Fprintf(rc.Err, "chown: changing ownership of '%s': %v\n", name, unwrapPathError(err))
}

func reportRoot(rc *tool.RunContext, name, path string) {
	fmt.Fprintf(rc.Err, "chown: it is dangerous to operate recursively on '%s'%s\n",
		name, rootguard.AliasSuffix(name, path))
	fmt.Fprintf(rc.Err, "chown: use --no-preserve-root to override this failsafe\n")
}

// reportCycle diagnoses a directory that is its own ancestor without a
// symbolic link having been followed to reach it — a hierarchy that
// cannot be walked to an end.
func reportCycle(rc *tool.RunContext, name string, opts chownOpts) {
	if opts.silent {
		return
	}
	fmt.Fprintf(rc.Err, "chown: WARNING: Circular directory structure.\n")
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

// parseSpec resolves [OWNER][:[GROUP]]. POSIX requires each half to be
// looked up as a name first and read as a numeric id only when no such
// account exists.
func parseSpec(spec string) (uid, gid int, err error) {
	uid, gid = -1, -1
	ownerStr, groupStr, hasColon := strings.Cut(spec, ":")
	var u *user.User
	if ownerStr != "" {
		u, err = lookupUser(ownerStr)
		switch {
		case err == nil:
			if uid, err = strconv.Atoi(u.Uid); err != nil {
				return -1, -1, fmt.Errorf("invalid user: '%s'", spec)
			}
		default:
			id, aerr := strconv.Atoi(ownerStr)
			if aerr != nil || id < 0 {
				return -1, -1, fmt.Errorf("invalid user: '%s'", spec)
			}
			uid = id
			u, _ = user.LookupId(ownerStr)
		}
	}
	if !hasColon {
		return uid, gid, nil
	}
	if groupStr == "" {
		if ownerStr == "" {
			return -1, -1, nil
		}
		// "OWNER:" means the owner's login group.
		if u == nil {
			return -1, -1, fmt.Errorf("invalid spec: '%s'", spec)
		}
		if gid, err = strconv.Atoi(u.Gid); err != nil {
			return -1, -1, fmt.Errorf("invalid spec: '%s'", spec)
		}
		return uid, gid, nil
	}
	g, gerr := lookupGroup(groupStr)
	if gerr == nil {
		if gid, err = strconv.Atoi(g.Gid); err != nil {
			return -1, -1, fmt.Errorf("invalid group: '%s'", spec)
		}
		return uid, gid, nil
	}
	id, aerr := strconv.Atoi(groupStr)
	if aerr != nil || id < 0 {
		return -1, -1, fmt.Errorf("invalid group: '%s'", spec)
	}
	return uid, id, nil
}

// statFile reads the --reference file's ids.
func statFile(rc *tool.RunContext, path string) (*refFileInfo, error) {
	fi, err := os.Stat(rc.Path(path))
	if err != nil {
		return nil, err
	}
	st, ok := fi.Sys().(*syscall.Stat_t)
	if !ok {
		return nil, fmt.Errorf("cannot stat %s", path)
	}
	return &refFileInfo{uid: int(st.Uid), gid: int(st.Gid)}, nil
}

type refFileInfo struct {
	uid int
	gid int
}

func (r *refFileInfo) ids() (uid, gid int) { return r.uid, r.gid }

func statusError(rc *tool.RunContext, format string, a ...any) int {
	fmt.Fprintf(rc.Err, "chown: "+format+"\n", a...)
	return 1
}
