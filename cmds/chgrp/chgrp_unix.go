//go:build unix

package chgrpcmd

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/user"
	"path/filepath"
	"strconv"

	"github.com/qiangli/coreutils/tool"
)

// apply resolves GROUP (name first, then numeric id, as GNU does) and
// changes the group of every operand (recursively with -R).
func apply(rc *tool.RunContext, spec string, files []string, recursive bool) int {
	gid := -1
	g, err := user.LookupGroup(spec)
	if err == nil {
		if gid, err = strconv.Atoi(g.Gid); err != nil {
			fmt.Fprintf(rc.Err, "chgrp: invalid group: '%s'\n", spec)
			return 1
		}
	} else {
		id, aerr := strconv.Atoi(spec)
		if aerr != nil || id < 0 {
			fmt.Fprintf(rc.Err, "chgrp: invalid group: '%s'\n", spec)
			return 1
		}
		gid = id
	}
	exit := 0
	for _, name := range files {
		if !chgrpTree(rc, rc.Path(name), name, gid, recursive) {
			exit = 1
		}
	}
	return exit
}

// chgrpTree mirrors chowncmd's walker; each package keeps its own copy
// to stay self-contained (no helpers outside cmds/<name>/).
func chgrpTree(rc *tool.RunContext, root, display string, gid int, recursive bool) bool {
	ok := true
	if !recursive {
		if err := os.Chown(root, -1, gid); err != nil {
			report(rc, display, err)
			return false
		}
		return true
	}
	walkErr := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			report(rc, path, err)
			ok = false
			return nil
		}
		var cerr error
		if d.Type()&fs.ModeSymlink != 0 {
			cerr = os.Lchown(path, -1, gid) // -P traversal: never follow
		} else {
			cerr = os.Chown(path, -1, gid)
		}
		if cerr != nil {
			report(rc, path, cerr)
			ok = false
		}
		return nil
	})
	if walkErr != nil {
		report(rc, display, walkErr)
		ok = false
	}
	return ok
}

func report(rc *tool.RunContext, name string, err error) {
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
