// Canonicalization engine shared (by duplication — cmds packages do
// not import each other) with cmds/readlink. Implements the three
// gnulib canonicalize modes the GNU manual defines for realpath and
// readlink -f/-e/-m: symlinks are expanded as encountered (physical
// resolution), ".." applies to the resolved path so far, and the
// existence requirement varies by mode.
package realpathcmd

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/qiangli/coreutils/tool"
)

const (
	canonExisting   = iota // every component must exist (-e)
	canonAllButLast        // all but the final component must exist (default / readlink -f)
	canonMissing           // no component need exist (-m)
)

// maxSymlinks mirrors the Linux kernel's ELOOP threshold.
const maxSymlinks = 40

var (
	errNoEnt        = errors.New("no such file or directory")
	errTooManyLinks = errors.New("too many levels of symbolic links")
	errNotDir       = errors.New("not a directory")
	errNoWorkDir    = errors.New("relative operand but no absolute working directory")
)

// absOperand makes operand absolute against the invocation working
// directory WITHOUT cleaning: ".." must survive so it can be resolved
// physically (after symlink expansion), not lexically.
func absOperand(rc *tool.RunContext, operand string) (string, error) {
	if operand == "" {
		return "", errNoEnt // GNU: the empty name never exists
	}
	if filepath.IsAbs(operand) {
		return operand, nil
	}
	if !filepath.IsAbs(rc.Dir) {
		return "", errNoWorkDir
	}
	return rc.Dir + string(filepath.Separator) + operand, nil
}

// canonicalize resolves absPath (which must be absolute) per mode.
func canonicalize(absPath string, mode int) (string, error) {
	vol := filepath.VolumeName(absPath)
	resolved := vol + string(filepath.Separator)
	parts := splitPath(absPath[len(vol):])
	links := 0
	for len(parts) > 0 {
		c := parts[0]
		parts = parts[1:]
		switch c {
		case ".":
			continue
		case "..":
			// resolved holds no symlinks, so its lexical parent is
			// also its physical parent. The root is its own parent.
			resolved = filepath.Dir(resolved)
			continue
		}
		next := joinOne(resolved, c)
		fi, err := os.Lstat(next)
		if err != nil {
			switch mode {
			case canonMissing:
				resolved = next
				continue
			case canonAllButLast:
				if len(parts) == 0 {
					return next, nil
				}
				return "", err
			default:
				return "", err
			}
		}
		if fi.Mode()&os.ModeSymlink != 0 {
			links++
			if links > maxSymlinks {
				return "", errTooManyLinks
			}
			target, err := os.Readlink(next)
			if err != nil {
				return "", err
			}
			if filepath.IsAbs(target) {
				tvol := filepath.VolumeName(target)
				resolved = tvol + string(filepath.Separator)
				parts = append(splitPath(target[len(tvol):]), parts...)
			} else {
				parts = append(splitPath(target), parts...)
			}
			continue
		}
		resolved = next
	}
	return resolved, nil
}

func splitPath(p string) []string {
	return strings.FieldsFunc(p, func(r rune) bool {
		return r == '/' || r == filepath.Separator
	})
}

func joinOne(dir, name string) string {
	if strings.HasSuffix(dir, string(filepath.Separator)) {
		return dir + name
	}
	return dir + string(filepath.Separator) + name
}

// pathErrText unwraps *fs.PathError so diagnostics read
// "realpath: OPERAND: no such file or directory" without repeating
// the resolved path inside the message.
func pathErrText(err error) string {
	var pe *fs.PathError
	if errors.As(err, &pe) {
		return pe.Err.Error()
	}
	return err.Error()
}
