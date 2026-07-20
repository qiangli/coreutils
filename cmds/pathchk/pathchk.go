package pathchkcmd

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
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
	paths, code := tool.Parse(rc, cmd, fs, args)
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
	if p == "" {
		// POSIX reserves the empty-pathname check for -P. Historically,
		// neither the default checks nor -p diagnose an empty operand.
		return true
	}
	limit := defaultPathMax()
	nameLimit := 255
	// PATH_MAX includes the terminating NUL byte, unlike NAME_MAX.
	if len(p) >= limit {
		fmt.Fprintf(rc.Err, "pathchk: path %q has length %d; exceeds limit %d\n", p, len(p), limit)
		return false
	}
	for _, c := range strings.Split(p, string(filepath.Separator)) {
		if len(c) > nameLimit {
			fmt.Fprintf(rc.Err, "pathchk: name %q has length %d; exceeds limit %d\n", c, len(c), nameLimit)
			return false
		}
	}
	if bad, reason := invalidDirectoryPrefix(filepath.Dir(rc.Path(p))); bad != "" {
		fmt.Fprintf(rc.Err, "pathchk: %q: %s at %q\n", p, reason, bad)
		return false
	}
	return true
}

func defaultPathMax() int {
	switch runtime.GOOS {
	case "windows":
		return 260
	case "darwin", "dragonfly", "freebsd", "ios", "netbsd", "openbsd":
		return 1024
	default:
		return 4096
	}
}

// invalidDirectoryPrefix checks existing prefixes from the root down. Once a
// prefix does not exist, the rest of the pathname is valid if those missing
// directories were to be created, so there is nothing further to inspect.
func invalidDirectoryPrefix(dir string) (path, reason string) {
	var prefixes []string
	for dir != "" && dir != "." {
		prefixes = append(prefixes, dir)
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	for i := len(prefixes) - 1; i >= 0; i-- {
		prefix := prefixes[i]
		if _, err := os.Lstat(prefix); os.IsNotExist(err) {
			return "", ""
		} else if err != nil {
			return prefix, "cannot access directory"
		}
		st, err := os.Stat(prefix)
		if err != nil {
			return prefix, "cannot access directory"
		}
		if !st.IsDir() {
			return prefix, "not a directory"
		}
		// Looking up "." requires search permission on prefix without
		// requiring read permission to enumerate it.
		searchProbe := prefix + string(filepath.Separator) + "."
		if _, err := os.Stat(searchProbe); err != nil {
			return prefix, "directory is not searchable"
		}
	}
	return "", ""
}

func checkPOSIX(rc *tool.RunContext, p string) bool {
	if p == "" {
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
			fmt.Fprintf(rc.Err, "pathchk: %q has leading hyphen\n", c)
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
