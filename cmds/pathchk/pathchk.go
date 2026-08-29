package pathchkcmd

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/qiangli/coreutils/tool"
)

var cmd = &tool.Tool{
	Name:     "pathchk",
	Synopsis: "Check whether file names are valid or portable.",
	Usage:    "pathchk [OPTION]... NAME...",
}

const (
	posixPathMax = 256
	posixNameMax = 14
)

func init() { cmd.Run = run; tool.Register(cmd) }

func run(rc *tool.RunContext, args []string) int {
	fs := tool.NewFlags(cmd.Name)
	posix := fs.BoolP("posix", "p", false, "check for most POSIX systems")
	special := fs.BoolP("posix-special", "P", false, "check for empty names and leading hyphens")
	portability := fs.Bool("portability", false, "check both POSIX and special portability")
	paths, code := tool.ParseRequireOrder(rc, cmd, fs, args)
	if code >= 0 {
		return code
	}
	if len(paths) == 0 {
		return tool.UsageError(rc, cmd, "missing operand")
	}
	status := 0
	for _, p := range paths {
		ok := true
		if *portability || *posix {
			ok = checkPOSIX(rc, p) && ok
		}
		if *portability || *special {
			ok = checkSpecial(rc, p) && ok
		}
		// -P adds the special portability checks to the ordinary
		// filesystem checks. Only -p (and --portability, which includes it)
		// replaces those checks with the POSIX portability limits.
		if !*portability && !*posix {
			ok = checkDefault(rc, p) && ok
		}
		if !ok {
			status = 1
		}
	}
	return status
}

func checkDefault(rc *tool.RunContext, p string) bool {
	return checkDefaultWith(rc, p, filesystemLimits)
}

// limitLookup reports dir's {PATH_MAX} and {NAME_MAX}. A non-positive limit
// with a nil error means the filesystem reports no limit for that variable
// (pathconf's indeterminate result: -1 with errno unchanged), and the
// corresponding length check is skipped rather than failed.
type limitLookup func(string) (pathMax, nameMax int, err error)

func checkDefaultWith(rc *tool.RunContext, p string, limits limitLookup) bool {
	if p == "" {
		fmt.Fprintf(rc.Err, "pathchk: %q: No such file or directory\n", p)
		return false
	}
	base, components, trailing := pathParts(rc, p)
	pathLimit, nameLimit, err := limits(base)
	if err != nil {
		fmt.Fprintf(rc.Err, "pathchk: %q: cannot determine filesystem limits: %v\n", p, err)
		return false
	}
	// PATH_MAX includes the terminating NUL byte, unlike NAME_MAX.
	if pathLimit > 0 && len(p) >= pathLimit {
		fmt.Fprintf(rc.Err, "pathchk: path %q has length %d; exceeds limit %d\n", p, len(p), pathLimit)
		return false
	}
	current := base
	missing := false
	for i, c := range components {
		if c == "" {
			continue
		}
		if !missing {
			if _, searchErr := os.Stat(rawPathJoin(current, ".")); searchErr != nil {
				fmt.Fprintf(rc.Err, "pathchk: %q: directory is not searchable at %q\n", p, current)
				return false
			}
		}
		if nameLimit > 0 && len(c) > nameLimit {
			fmt.Fprintf(rc.Err, "pathchk: name %q has length %d; exceeds limit %d\n", c, len(c), nameLimit)
			return false
		}
		if missing {
			continue
		}
		candidate := rawPathJoin(current, c)
		st, lstatErr := os.Lstat(candidate)
		if os.IsNotExist(lstatErr) {
			missing = true
			continue
		}
		if lstatErr != nil {
			if errors.Is(lstatErr, os.ErrPermission) {
				fmt.Fprintf(rc.Err, "pathchk: %q: directory is not searchable at %q\n", p, current)
				return false
			}
			fmt.Fprintf(rc.Err, "pathchk: %q: byte sequence is not valid or component cannot be accessed at %q: %v\n", p, candidate, lstatErr)
			return false
		}
		last := i == len(components)-1 && !trailing
		if last {
			continue
		}
		st, statErr := os.Stat(candidate)
		if statErr != nil {
			fmt.Fprintf(rc.Err, "pathchk: %q: cannot access directory at %q: %v\n", p, candidate, statErr)
			return false
		}
		if !st.IsDir() {
			fmt.Fprintf(rc.Err, "pathchk: %q: not a directory at %q\n", p, candidate)
			return false
		}
		current = candidate
		_, nameLimit, err = limits(current)
		if err != nil {
			fmt.Fprintf(rc.Err, "pathchk: %q: cannot determine filesystem limits at %q: %v\n", p, current, err)
			return false
		}
	}
	return true
}

// rawPathJoin appends one component without filepath.Join's lexical Clean.
// Cleaning is incorrect for pathname validation: `symlink/..` must follow the
// symlink before resolving `..`, exactly as the kernel pathname walk does.
func rawPathJoin(dir, component string) string {
	if strings.HasSuffix(dir, string(filepath.Separator)) {
		return dir + component
	}
	return dir + string(filepath.Separator) + component
}

func pathParts(rc *tool.RunContext, p string) (base string, components []string, trailing bool) {
	separator := string(filepath.Separator)
	normalized := filepath.FromSlash(p)
	trailing = strings.HasSuffix(normalized, separator)
	if filepath.IsAbs(normalized) {
		volume := filepath.VolumeName(normalized)
		base = volume + separator
		normalized = strings.TrimPrefix(normalized[len(volume):], separator)
	} else {
		base = rc.Dir
		if base == "" {
			base = "."
		}
	}
	return base, strings.Split(normalized, separator), trailing
}

func checkPOSIX(rc *tool.RunContext, p string) bool {
	if p == "" {
		// -p checks only the portable pathname character set and the
		// _POSIX_{PATH,NAME}_MAX limits. The separate -P option owns the
		// empty-pathname rejection; an empty operand violates no -p rule.
		return true
	}
	// _POSIX_PATH_MAX counts the terminating NUL byte, so a pathname of
	// exactly posixPathMax characters needs posixPathMax+1 bytes of storage
	// and is already too long. _POSIX_NAME_MAX, checked below, does not.
	if len(p) >= posixPathMax {
		fmt.Fprintf(rc.Err, "pathchk: path %q has length %d; exceeds POSIX limit %d\n", p, len(p), posixPathMax)
		return false
	}
	for _, c := range strings.Split(p, "/") {
		if len(c) > posixNameMax {
			fmt.Fprintf(rc.Err, "pathchk: name %q has length %d; exceeds POSIX limit %d\n", c, len(c), posixNameMax)
			return false
		}
		if !portableChars(c) {
			fmt.Fprintf(rc.Err, "pathchk: %q contains a nonportable character\n", c)
			return false
		}
	}
	return true
}

func checkSpecial(rc *tool.RunContext, p string) bool {
	if p == "" {
		fmt.Fprintln(rc.Err, "pathchk: empty file name")
		return false
	}
	for _, c := range strings.Split(p, "/") {
		if strings.HasPrefix(c, "-") {
			fmt.Fprintf(rc.Err, "pathchk: %q has a component beginning with a hyphen\n", p)
			return false
		}
	}
	return true
}

func portableChars(s string) bool {
	for _, r := range s {
		if r >= 'A' && r <= 'Z' || r >= 'a' && r <= 'z' || r >= '0' && r <= '9' {
			continue
		}
		switch r {
		case '.', '_', '-':
			continue
		default:
			return false
		}
	}
	return true
}
