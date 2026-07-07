//go:build unix

package chowncmd

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/user"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"

	"github.com/qiangli/coreutils/tool"
)

type derefMode int

const (
	derefNever derefMode = iota
	derefCmdLine
	derefAlways
)

type chownOpts struct {
	recursive    bool
	verbose      bool
	changes      bool
	silent       bool
	preserveRoot bool
	deref        derefMode
	uid          int
	gid          int
	fromUid      int
	fromGid      int
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

func apply(rc *tool.RunContext, spec string, files []string, recursive, verbose, changes, silent, preserveRoot, noDerefOrNoPreserve, cmdLineH, followLOrDeref bool, fromUid, fromGid int) int {
	uid, gid, err := parseSpec(spec)
	if err != nil {
		statusError(rc, "%v", err)
		return 1
	}
	opts := chownOpts{
		recursive:    recursive,
		verbose:      verbose || changes,
		changes:      changes,
		silent:       silent,
		preserveRoot: preserveRoot,
		deref:        computeDeref(followLOrDeref, cmdLineH),
		uid:          uid,
		gid:          gid,
		fromUid:      fromUid,
		fromGid:      fromGid,
	}

	exit := 0
	for _, name := range files {
		path := rc.Path(name)
		if opts.recursive && opts.preserveRoot && path == "/" {
			fmt.Fprintf(rc.Err, "chown: it is dangerous to operate recursively on '/'\n")
			fmt.Fprintf(rc.Err, "chown: use --no-preserve-root to override this failsafe\n")
			exit = 1
			continue
		}
		if !chownTree(rc, path, name, opts) {
			exit = 1
		}
	}
	return exit
}

func chownTree(rc *tool.RunContext, root, display string, opts chownOpts) bool {
	ok := true

	if !opts.recursive {
		changed, err := chownOne(root, opts)
		if err != nil {
			chownReport(rc, display, opts, err)
			return false
		}
		chownVerbose(rc.Out, display, changed, opts)
		return true
	}

	walkErr := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			chownReport(rc, path, opts, err)
			ok = false
			return nil
		}
		isSymlink := d.Type()&fs.ModeSymlink != 0

		useChown := true
		if opts.deref == derefNever && isSymlink {
			useChown = false
		}

		var changed bool
		var cerr error
		if useChown {
			changed, cerr = chownOne(path, opts)
		} else {
			cerr = os.Lchown(path, opts.uid, opts.gid)
			changed = (cerr == nil)
		}

		if cerr != nil {
			chownReport(rc, path, opts, cerr)
			ok = false
		} else {
			chownVerbose(rc.Out, path, changed, opts)
		}
		return nil
	})
	if walkErr != nil {
		chownReport(rc, display, opts, walkErr)
		ok = false
	}
	return ok
}

func chownOne(path string, opts chownOpts) (changed bool, err error) {
	fi, statErr := os.Stat(path)
	if statErr != nil {
		return false, statErr
	}
	st, ok := fi.Sys().(*syscall.Stat_t)
	if ok {
		if opts.fromUid >= 0 && opts.fromGid >= 0 {
			if int(st.Uid) != opts.fromUid || int(st.Gid) != opts.fromGid {
				return false, nil
			}
		} else if opts.fromUid >= 0 {
			if int(st.Uid) != opts.fromUid {
				return false, nil
			}
		} else if opts.fromGid >= 0 {
			if int(st.Gid) != opts.fromGid {
				return false, nil
			}
		}
		changed := false
		if opts.uid >= 0 && int(st.Uid) != opts.uid {
			changed = true
		}
		if opts.gid >= 0 && int(st.Gid) != opts.gid {
			changed = true
		}
		if !changed {
			return false, nil
		}
	}
	err = os.Chown(path, opts.uid, opts.gid)
	return err == nil, err
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

func chownReport(rc *tool.RunContext, name string, opts chownOpts, err error) {
	if opts.silent {
		return
	}
	var pe *os.PathError
	if errors.As(err, &pe) {
		err = pe.Err
	}
	if errors.Is(err, fs.ErrNotExist) {
		fmt.Fprintf(rc.Err, "chown: cannot access '%s': %v\n", name, err)
		return
	}
	fmt.Fprintf(rc.Err, "chown: changing ownership of '%s': %v\n", name, err)
}

func parseSpec(spec string) (uid, gid int, err error) {
	uid, gid = -1, -1
	ownerStr, groupStr, hasColon := strings.Cut(spec, ":")
	var u *user.User
	if ownerStr != "" {
		u, err = user.Lookup(ownerStr)
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
		if u == nil {
			return -1, -1, fmt.Errorf("invalid spec: '%s'", spec)
		}
		if gid, err = strconv.Atoi(u.Gid); err != nil {
			return -1, -1, fmt.Errorf("invalid spec: '%s'", spec)
		}
		return uid, gid, nil
	}
	g, gerr := user.LookupGroup(groupStr)
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

func statFile(rc *tool.RunContext, path string) (*refFileInfo, error) {
	fi, err := os.Stat(rc.Path(path))
	if err != nil {
		return nil, err
	}
	st, ok := fi.Sys().(*syscall.Stat_t)
	if !ok {
		return nil, fmt.Errorf("cannot stat %s", path)
	}
	return &refFileInfo{uid: st.Uid, gid: st.Gid}, nil
}

type refFileInfo struct {
	uid uint32
	gid uint32
}

func (r *refFileInfo) ownerStr() string {
	if r == nil {
		return ""
	}
	return fmt.Sprintf("%d:%d", r.uid, r.gid)
}

func statusError(rc *tool.RunContext, format string, a ...any) int {
	fmt.Fprintf(rc.Err, "chown: "+format+"\n", a...)
	return 1
}
