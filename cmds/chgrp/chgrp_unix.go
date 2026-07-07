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

func apply(rc *tool.RunContext, spec string, files []string, recursive, verbose, changes, silent, preserveRoot, noDerefOrNoPreserve, cmdLineH, followLOrDeref bool) int {
	opts := chgrpOpts{
		recursive:    recursive,
		verbose:      verbose || changes,
		changes:      changes,
		silent:       silent,
		preserveRoot: preserveRoot,
		deref:        computeDeref(followLOrDeref, cmdLineH),
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
		changed, err := chgrpOne(root, opts)
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
			changed, cerr = chgrpOne(path, opts)
		} else {
			cerr = os.Lchown(path, -1, opts.targetGid)
			changed = (cerr == nil)
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

func chgrpOne(path string, opts chgrpOpts) (changed bool, err error) {
	fi, statErr := os.Stat(path)
	if statErr != nil {
		return false, statErr
	}
	st, ok := fi.Sys().(*syscall.Stat_t)
	if ok && int(st.Gid) == opts.targetGid {
		return false, os.Chown(path, -1, opts.targetGid)
	}
	err = os.Chown(path, -1, opts.targetGid)
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
