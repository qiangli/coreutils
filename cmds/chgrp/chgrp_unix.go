//go:build unix

package chgrpcmd

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

type chgrpOpts struct {
	recursive    bool
	verbose      bool
	changes      bool
	silent       bool
	preserveRoot bool
	deref        derefMode
	targetGid    int
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

func apply(rc *tool.RunContext, spec string, files []string, recursive, verbose, changes, silent, preserveRoot, noDereference, noTraverse, cmdLineH, followLOrDeref bool, fromUid, fromGid int) int {
	opts := chgrpOpts{
		recursive:    recursive,
		verbose:      verbose || changes,
		changes:      changes,
		silent:       silent,
		preserveRoot: preserveRoot,
		deref:        computeDeref(followLOrDeref, cmdLineH),
		fromUid:      fromUid,
		fromGid:      fromGid,
	}
	if noDereference || noTraverse {
		opts.deref = derefNever
	}

	gid := -1
	g, err := user.LookupGroup(spec)
	if err == nil {
		if gid, err = strconv.Atoi(g.Gid); err != nil {
			statusError(rc, "invalid group: '%s'", spec)
			return 1
		}
	} else {
		id, aerr := strconv.Atoi(spec)
		if aerr != nil || id < 0 {
			statusError(rc, "invalid group: '%s'", spec)
			return 1
		}
		gid = id
	}
	opts.targetGid = gid

	exit := 0
	for _, name := range files {
		path := rc.Path(name)
		if opts.recursive && opts.preserveRoot && path == "/" {
			fmt.Fprintf(rc.Err, "chgrp: it is dangerous to operate recursively on '/'\n")
			fmt.Fprintf(rc.Err, "chgrp: use --no-preserve-root to override this failsafe\n")
			exit = 1
			continue
		}
		if !chgrpTree(rc, path, name, opts) {
			exit = 1
		}
	}
	return exit
}

func chgrpTree(rc *tool.RunContext, root, display string, opts chgrpOpts) bool {
	ok := true

	if !opts.recursive {
		changed, err := chgrpOne(root, opts, opts.deref != derefNever)
		if err != nil {
			chgrpReport(rc, display, opts, err)
			return false
		}
		chgrpVerbose(rc.Out, display, changed, opts)
		return true
	}

	walkErr := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			chgrpReport(rc, path, opts, err)
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
			changed, cerr = chgrpOne(path, opts, true)
		} else {
			changed, cerr = chgrpOne(path, opts, false)
		}

		if cerr != nil {
			chgrpReport(rc, path, opts, cerr)
			ok = false
		} else {
			chgrpVerbose(rc.Out, path, changed, opts)
		}
		return nil
	})
	if walkErr != nil {
		chgrpReport(rc, display, opts, walkErr)
		ok = false
	}
	return ok
}

func chgrpOne(path string, opts chgrpOpts, follow bool) (changed bool, err error) {
	stat := os.Stat
	chown := os.Chown
	if !follow {
		stat = os.Lstat
		chown = os.Lchown
	}
	fi, statErr := stat(path)
	if statErr != nil {
		return false, statErr
	}
	st, ok := fi.Sys().(*syscall.Stat_t)
	if ok {
		if opts.fromUid >= 0 && int(st.Uid) != opts.fromUid {
			return false, nil
		}
		if opts.fromGid >= 0 && int(st.Gid) != opts.fromGid {
			return false, nil
		}
		if int(st.Gid) == opts.targetGid {
			return false, nil
		}
	}
	err = chown(path, -1, opts.targetGid)
	return err == nil, err
}

func chgrpVerbose(out io.Writer, name string, changed bool, opts chgrpOpts) {
	if !opts.verbose {
		return
	}
	if opts.changes && !changed {
		return
	}
	if changed {
		fmt.Fprintf(out, "changed group of '%s'\n", name)
	} else if !opts.changes {
		fmt.Fprintf(out, "group of '%s' retained as %d\n", name, opts.targetGid)
	}
}

func chgrpReport(rc *tool.RunContext, name string, opts chgrpOpts, err error) {
	if opts.silent {
		return
	}
	var pe *os.PathError
	if errors.As(err, &pe) {
		err = pe.Err
	}
	if errors.Is(err, fs.ErrNotExist) {
		fmt.Fprintf(rc.Err, "chgrp: cannot access '%s': %v\n", name, err)
		return
	}
	fmt.Fprintf(rc.Err, "chgrp: changing group of '%s': %v\n", name, err)
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
	return &refFileInfo{gid: st.Gid}, nil
}

func parseFromSpec(spec string) (uid, gid int, err error) {
	uid, gid = -1, -1
	if spec == "" {
		return uid, gid, nil
	}
	ownerStr, groupStr, hasColon := strings.Cut(spec, ":")
	if ownerStr != "" {
		u, uerr := user.Lookup(ownerStr)
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
	if !hasColon {
		return uid, gid, nil
	}
	if groupStr == "" {
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

type refFileInfo struct {
	gid uint32
}

func (r *refFileInfo) gidStr() string {
	if r == nil {
		return ""
	}
	return strconv.FormatUint(uint64(r.gid), 10)
}

func statusError(rc *tool.RunContext, format string, a ...any) int {
	fmt.Fprintf(rc.Err, "chgrp: "+format+"\n", a...)
	return 1
}
