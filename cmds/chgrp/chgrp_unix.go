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
var changeGroup = func(path string, uid, gid int, follow bool) error {
	if follow {
		return os.Chown(path, uid, gid)
	}
	return os.Lchown(path, uid, gid)
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
		Visit: func(path, name string, isLink, _ bool) {
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
	// The exact current uid and gid are required for the specified chown()
	// equivalent. A platform that cannot expose them must fail closed.
	stat := os.Lstat
	if opts.affectReferent {
		stat = os.Stat
	}
	fi, err := stat(path)
	if err != nil {
		return false, -1, err, nil
	}
	st, ok := fi.Sys().(*syscall.Stat_t)
	if !ok {
		return false, -1, errors.New("ownership metadata is unavailable on this platform"), nil
	}
	uid, uidOK := representableNumericID(uint64(st.Uid))
	held, gidOK := representableNumericID(uint64(st.Gid))
	if !uidOK || !gidOK {
		return false, -1, errors.New("ownership metadata is not representable on this platform"), nil
	}
	if opts.fromUid >= 0 && uid != opts.fromUid {
		return false, held, nil, nil
	}
	if opts.fromGid >= 0 && held != opts.fromGid {
		return false, held, nil, nil
	}
	changed = held != opts.gid
	// POSIX specifies an action equivalent to chown(path, current_uid,
	// requested_gid), not the extension spelling chown(path, -1, gid).
	// Passing the observed owner also preserves the standard's exact
	// permission checking and set-ID consequences.
	if err := changeGroup(path, uid, opts.gid, opts.affectReferent); err != nil {
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
	g, lookupErr := lookupGroup(spec)
	if lookupErr == nil {
		gid, ok := parseNumericID(g.Gid)
		if !ok {
			return -1, fmt.Errorf("invalid group: '%s'", spec)
		}
		return gid, nil
	}
	var unknown user.UnknownGroupError
	if !errors.As(lookupErr, &unknown) {
		return -1, fmt.Errorf("cannot resolve group '%s': %v", spec, lookupErr)
	}
	id, ok := parseNumericID(spec)
	if !ok {
		return -1, fmt.Errorf("invalid group: '%s'", spec)
	}
	return id, nil
}

// parseNumericID accepts the POSIX numeric-ID spelling: one or more decimal
// digits in the usable 32-bit uid_t/gid_t range that the host Go int can
// represent. The all-ones value is the chown(2) "leave unchanged" sentinel
// and therefore cannot name an ID to set.
func parseNumericID(s string) (int, bool) {
	if s == "" {
		return -1, false
	}
	for i := range len(s) {
		if s[i] < '0' || s[i] > '9' {
			return -1, false
		}
	}
	v, err := strconv.ParseUint(s, 10, 32)
	if err != nil {
		return -1, false
	}
	return representableNumericID(v)
}

func representableNumericID(v uint64) (int, bool) {
	maxInt := uint64(^uint(0) >> 1)
	if v == 1<<32-1 || v > maxInt {
		return -1, false
	}
	return int(v), true
}

func parseFromSpec(spec string) (uid, gid int, err error) {
	uid, gid = -1, -1
	if spec == "" {
		return uid, gid, nil
	}
	ownerStr, groupStr, hasColon := strings.Cut(spec, ":")
	if ownerStr != "" {
		u, uerr := lookupUser(ownerStr)
		if uerr == nil {
			var ok bool
			if uid, ok = parseNumericID(u.Uid); !ok {
				return -1, -1, fmt.Errorf("invalid user: '%s'", spec)
			}
		} else {
			var unknown user.UnknownUserError
			if !errors.As(uerr, &unknown) {
				return -1, -1, fmt.Errorf("cannot resolve user '%s': %v", ownerStr, uerr)
			}
			var ok bool
			if uid, ok = parseNumericID(ownerStr); !ok {
				return -1, -1, fmt.Errorf("invalid user: '%s'", spec)
			}
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
	resolved := path
	if path != "" {
		resolved = rc.Path(path)
	}
	fi, err := os.Stat(resolved)
	if err != nil {
		return nil, err
	}
	st, ok := fi.Sys().(*syscall.Stat_t)
	if !ok {
		return nil, fmt.Errorf("cannot stat %s", path)
	}
	gid, ok := representableNumericID(uint64(st.Gid))
	if !ok {
		return nil, fmt.Errorf("cannot represent group id for %s", path)
	}
	return &refFileInfo{gid: gid}, nil
}

type refFileInfo struct {
	gid int
}

func (r *refFileInfo) ids() (uid, gid int) { return -1, r.gid }

func statusError(rc *tool.RunContext, format string, a ...any) int {
	fmt.Fprintf(rc.Err, "chgrp: "+format+"\n", a...)
	return 1
}
