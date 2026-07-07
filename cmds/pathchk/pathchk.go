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
		if !*portability && !*posix && !*special {
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
		fmt.Fprintln(rc.Err, "pathchk: empty file name")
		return false
	}
	limit := 4096
	nameLimit := 255
	if runtime.GOOS == "windows" {
		limit = 260
	}
	if len(p) > limit {
		fmt.Fprintf(rc.Err, "pathchk: path %q has length %d; exceeds limit %d\n", p, len(p), limit)
		return false
	}
	for _, c := range strings.Split(p, string(filepath.Separator)) {
		if len(c) > nameLimit {
			fmt.Fprintf(rc.Err, "pathchk: name %q has length %d; exceeds limit %d\n", c, len(c), nameLimit)
			return false
		}
	}
	dir := filepath.Dir(rc.Path(p))
	if dir != "." && dir != "" {
		if st, err := os.Stat(dir); err == nil && !st.IsDir() {
			fmt.Fprintf(rc.Err, "pathchk: %q is not a directory\n", dir)
			return false
		}
	}
	return true
}

func checkPOSIX(rc *tool.RunContext, p string) bool {
	if p == "" {
		fmt.Fprintln(rc.Err, "pathchk: empty file name")
		return false
	}
	if len(p) > posixPathMax {
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
