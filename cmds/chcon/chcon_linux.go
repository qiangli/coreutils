//go:build linux

package chconcmd

import (
	"fmt"
	"io/fs"
	"path/filepath"

	"golang.org/x/sys/unix"

	"github.com/qiangli/coreutils/tool"
)

const selinuxXattr = "security.selinux"

func applyContext(rc *tool.RunContext, op chconOp) int {
	if op.mode == modeReference {
		context, err := getFileContext(rc.Path(op.reference), !op.noDereference)
		if err != nil {
			fmt.Fprintf(rc.Err, "chcon: cannot get security context of reference file '%s': %v\n", op.reference, tool.SysErr(err))
			return 1
		}
		op.context = context
	}

	exit := 0
	for _, name := range op.files {
		path := rc.Path(name)
		if op.recursive && op.preserveRoot && path == "/" {
			fmt.Fprintln(rc.Err, "chcon: it is dangerous to operate recursively on '/'")
			fmt.Fprintln(rc.Err, "chcon: use --no-preserve-root to override this failsafe")
			exit = 1
			continue
		}
		if !chconTree(rc, path, name, op) {
			exit = 1
		}
	}
	return exit
}

func chconTree(rc *tool.RunContext, root, display string, op chconOp) bool {
	if !op.recursive {
		context, err := targetContext(root, op, !op.noDereference)
		if err != nil {
			reportChconErr(rc, display, contextForError(op, context), err)
			return false
		}
		if err := setFileContext(root, context, !op.noDereference); err != nil {
			reportChconErr(rc, display, context, err)
			return false
		}
		chconVerbose(rc, display, context, op)
		return true
	}

	ok := true
	walkErr := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			reportChconErr(rc, path, contextForError(op, op.context), err)
			ok = false
			return nil
		}
		follow := true
		if op.noDereference && d.Type()&fs.ModeSymlink != 0 {
			follow = false
		}
		context, cerr := targetContext(path, op, follow)
		if cerr != nil {
			reportChconErr(rc, path, contextForError(op, context), cerr)
			ok = false
			return nil
		}
		if serr := setFileContext(path, context, follow); serr != nil {
			reportChconErr(rc, path, context, serr)
			ok = false
			return nil
		}
		chconVerbose(rc, path, context, op)
		return nil
	})
	if walkErr != nil {
		reportChconErr(rc, display, contextForError(op, op.context), walkErr)
		ok = false
	}
	return ok
}

func targetContext(path string, op chconOp, follow bool) (string, error) {
	if op.mode != modeComponents {
		return op.context, nil
	}
	current, err := getFileContext(path, follow)
	if err != nil {
		return "", err
	}
	return mergeContext(current, op.parts)
}

func getFileContext(path string, follow bool) (string, error) {
	size := 255
	for {
		buf := make([]byte, size)
		var n int
		var err error
		if follow {
			n, err = unix.Getxattr(path, selinuxXattr, buf)
		} else {
			n, err = unix.Lgetxattr(path, selinuxXattr, buf)
		}
		if err == unix.ERANGE {
			size *= 2
			continue
		}
		if err != nil {
			return "", err
		}
		return string(buf[:n]), nil
	}
}

func setFileContext(path, context string, follow bool) error {
	if follow {
		return unix.Setxattr(path, selinuxXattr, []byte(context), 0)
	}
	return unix.Lsetxattr(path, selinuxXattr, []byte(context), 0)
}

func chconVerbose(rc *tool.RunContext, name, context string, op chconOp) {
	if op.verbose {
		fmt.Fprintf(rc.Out, "changed context of '%s' to '%s'\n", name, context)
	}
}

func contextForError(op chconOp, context string) string {
	if context != "" {
		return context
	}
	if op.mode == modeComponents {
		return "<computed>"
	}
	return op.context
}
