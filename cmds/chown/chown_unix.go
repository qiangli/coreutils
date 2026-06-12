//go:build unix

package chowncmd

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/user"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/qiangli/coreutils/tool"
)

// apply resolves the [OWNER][:[GROUP]] spec and changes ownership of
// every operand (recursively with -R). Command-line operands are
// dereferenced; symlinks met during recursion get lchown (GNU -P
// default traversal).
func apply(rc *tool.RunContext, spec string, files []string, recursive bool) int {
	uid, gid, err := parseSpec(spec)
	if err != nil {
		fmt.Fprintf(rc.Err, "chown: %v\n", err)
		return 1
	}
	exit := 0
	for _, name := range files {
		if !chownTree(rc, "chown", "changing ownership of", rc.Path(name), name, uid, gid, recursive) {
			exit = 1
		}
	}
	return exit
}

// parseSpec resolves [OWNER][:[GROUP]] to numeric ids; -1 = unchanged.
// Names are looked up first (os/user); pure-numeric strings that are
// not known names are used as raw ids, as GNU does.
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
			u, _ = user.LookupId(ownerStr) // may stay nil; only needed for 'OWNER:'
		}
	}
	if !hasColon {
		return uid, gid, nil
	}
	if groupStr == "" {
		if ownerStr == "" {
			return -1, -1, nil // ':' alone changes nothing
		}
		// 'OWNER:' means OWNER's login group.
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

// chownTree is shared with chgrp via copy — both packages keep their
// own copy to stay self-contained (no helpers outside cmds/<name>/).
func chownTree(rc *tool.RunContext, toolName, verb, root, display string, uid, gid int, recursive bool) bool {
	ok := true
	if !recursive {
		if err := os.Chown(root, uid, gid); err != nil {
			reportChownErr(rc, toolName, verb, display, err)
			return false
		}
		return true
	}
	walkErr := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			reportChownErr(rc, toolName, verb, path, err)
			ok = false
			return nil
		}
		var cerr error
		if d.Type()&fs.ModeSymlink != 0 {
			cerr = os.Lchown(path, uid, gid) // -P traversal: never follow
		} else {
			cerr = os.Chown(path, uid, gid)
		}
		if cerr != nil {
			reportChownErr(rc, toolName, verb, path, cerr)
			ok = false
		}
		return nil
	})
	if walkErr != nil {
		reportChownErr(rc, toolName, verb, display, walkErr)
		ok = false
	}
	return ok
}

func reportChownErr(rc *tool.RunContext, toolName, verb, name string, err error) {
	var pe *os.PathError
	if errors.As(err, &pe) {
		err = pe.Err
	}
	if errors.Is(err, fs.ErrNotExist) {
		fmt.Fprintf(rc.Err, "%s: cannot access '%s': %v\n", toolName, name, err)
		return
	}
	fmt.Fprintf(rc.Err, "%s: %s '%s': %v\n", toolName, verb, name, err)
}
