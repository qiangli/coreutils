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
// which call each file takes — chown for a referent, lchown for a link.
// The real syscall path stays covered by the self-chown tests.
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
		if !chownTree(rc, path, name, state, &outputFailed) {
			exit = 1
		}
	}
	return exit
}

// chownTree walks one operand hierarchy. Every file is attempted: a
// failure anywhere sets the exit status without stopping the walk, as
// POSIX requires.
func chownTree(rc *tool.RunContext, root, display string, opts chownOpts, outputFailed *bool) bool {
	ok := true
	walker := &hierwalk.Walker{
		Mode:      opts.mode,
		Recursive: opts.recursive,
		Visit: func(path, name string, isLink, _ bool) {
			changed, statErr, chownErr := chownOne(path, opts)
			switch {
			case statErr != nil:
				reportUnreachable(rc, name, isLink && opts.affectReferent, opts, statErr)
				ok = false
			case chownErr != nil:
				reportChange(rc, name, opts, chownErr)
				ok = false
			default:
				// An output failure must affect the status, but ownership work
				// continues for the rest of the hierarchy. Avoid retrying a
				// writer that has already failed and report the error once.
				if !*outputFailed {
					if err := chownVerbose(rc.Out, name, changed, opts); err != nil {
						fmt.Fprintf(rc.Err, "chown: write error: %v\n", err)
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
	// Reporting and --from both require exact native ownership metadata. A
	// platform that cannot expose it must fail closed.
	stat := os.Lstat
	if opts.affectReferent {
		stat = os.Stat
	}
	fi, err := stat(path)
	if err != nil {
		return false, err, nil
	}
	st, ok := fi.Sys().(*syscall.Stat_t)
	if !ok {
		return false, errors.New("ownership metadata is unavailable on this platform"), nil
	}
	currentUID, uidOK := representableNumericID(uint64(st.Uid))
	currentGID, gidOK := representableNumericID(uint64(st.Gid))
	if !uidOK || !gidOK {
		return false, errors.New("ownership metadata is not representable on this platform"), nil
	}
	if opts.fromUid >= 0 && currentUID != opts.fromUid {
		return false, nil, nil
	}
	if opts.fromGid >= 0 && currentGID != opts.fromGid {
		return false, nil, nil
	}
	changed = (opts.uid >= 0 && currentUID != opts.uid) ||
		(opts.gid >= 0 && currentGID != opts.gid)
	if err := changeOwner(path, opts.uid, opts.gid, opts.affectReferent); err != nil {
		return false, nil, err
	}
	return changed, nil, nil
}

func chownVerbose(out io.Writer, name string, changed bool, opts chownOpts) error {
	if !opts.verbose {
		return nil
	}
	if opts.changes && !changed {
		return nil
	}
	if changed {
		_, err := fmt.Fprintf(out, "changed ownership of '%s'\n", name)
		return err
	} else if !opts.changes {
		_, err := fmt.Fprintf(out, "ownership of '%s' retained\n", name)
		return err
	}
	return nil
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
			var ok bool
			if uid, ok = parseNumericID(u.Uid); !ok {
				return -1, -1, fmt.Errorf("invalid user: '%s'", spec)
			}
		default:
			var unknown user.UnknownUserError
			if !errors.As(err, &unknown) {
				return -1, -1, fmt.Errorf("cannot resolve user '%s': %v", ownerStr, err)
			}
			id, ok := parseNumericID(ownerStr)
			if !ok {
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
		var ok bool
		if gid, ok = parseNumericID(u.Gid); !ok {
			return -1, -1, fmt.Errorf("invalid spec: '%s'", spec)
		}
		return uid, gid, nil
	}
	g, gerr := lookupGroup(groupStr)
	if gerr == nil {
		var ok bool
		if gid, ok = parseNumericID(g.Gid); !ok {
			return -1, -1, fmt.Errorf("invalid group: '%s'", spec)
		}
		return uid, gid, nil
	}
	var unknown user.UnknownGroupError
	if !errors.As(gerr, &unknown) {
		return -1, -1, fmt.Errorf("cannot resolve group '%s': %v", groupStr, gerr)
	}
	id, ok := parseNumericID(groupStr)
	if !ok {
		return -1, -1, fmt.Errorf("invalid group: '%s'", spec)
	}
	return uid, id, nil
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

// statFile reads the --reference file's ids.
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
	uid, uidOK := representableNumericID(uint64(st.Uid))
	gid, gidOK := representableNumericID(uint64(st.Gid))
	if !uidOK || !gidOK {
		return nil, fmt.Errorf("cannot represent ownership ids for %s", path)
	}
	return &refFileInfo{uid: uid, gid: gid}, nil
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
