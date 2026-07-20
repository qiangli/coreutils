// Package pwdcmd implements pwd(1) per the GNU coreutils manual:
// print the name of the current working directory.
//
// The "current working directory" is the invocation's rc.Dir, never the
// process cwd (the embedding shell owns its own cwd). Logical mode uses
// PWD from the invocation environment when it is a valid name for rc.Dir;
// physical mode and invalid logical names resolve every symlink in rc.Dir.
// When both -L and -P are given, the last one takes precedence.
package pwdcmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/qiangli/coreutils/tool"
)

var cmd = &tool.Tool{
	Name:     "pwd",
	Synopsis: "Print the full filename of the current working directory.",
	Usage:    "pwd [OPTION]...",
}

// Run is wired in init: a literal would create an initialization
// cycle (run's flag-error paths reference cmd).
func init() { cmd.Run = run; tool.Register(cmd) }

func run(rc *tool.RunContext, args []string) int {
	args = tool.AliasHelpVersion(args)
	fs := tool.NewFlags(cmd.Name)
	fs.BoolP("logical", "L", false, "print the logical working directory, even with symlinks")
	fs.BoolP("physical", "P", false, "print the physical directory, with all symlinks resolved")
	operands, code := tool.Parse(rc, cmd, fs, args)
	if code >= 0 {
		return code
	}
	if len(operands) > 0 {
		// GNU pwd warns and proceeds; the operands have no meaning.
		fmt.Fprintln(rc.Err, "pwd: ignoring non-option arguments")
	}
	if rc.Dir == "" {
		fmt.Fprintln(rc.Err, "pwd: cannot determine current directory")
		return 1
	}
	if lastLP(args) == 'L' {
		if logical := logicalDir(rc); logical != "" {
			fmt.Fprintln(rc.Out, logical)
			return 0
		}
	}

	resolved, err := physicalDir(rc.Dir)
	if err != nil {
		fmt.Fprintf(rc.Err, "pwd: %v\n", err)
		return 1
	}
	fmt.Fprintln(rc.Out, resolved)
	return 0
}

// logicalDir returns PWD only when it satisfies the POSIX invariants for a
// logical working-directory name and identifies rc.Dir.
func logicalDir(rc *tool.RunContext) string {
	pwd := rc.Getenv("PWD")
	if !filepath.IsAbs(pwd) || hasDotComponent(pwd) {
		return ""
	}
	pwdInfo, err := os.Stat(pwd)
	if err != nil {
		return ""
	}
	dirInfo, err := os.Stat(rc.Dir)
	if err != nil || !os.SameFile(pwdInfo, dirInfo) {
		return ""
	}
	return pwd
}

func hasDotComponent(path string) bool {
	for _, component := range strings.Split(filepath.ToSlash(path), "/") {
		if component == "." || component == ".." {
			return true
		}
	}
	return false
}

func physicalDir(dir string) (string, error) {
	resolved, err := filepath.EvalSymlinks(dir)
	if err != nil {
		return "", err
	}
	if !filepath.IsAbs(resolved) {
		return "", fmt.Errorf("cannot determine absolute path for %q", dir)
	}
	return resolved, nil
}

// lastLP scans args for the last occurrence of -L/-P (or their long
// forms) — pflag reports values but not order, and GNU specifies that
// the last of the two wins. Default is 'L'.
func lastLP(args []string) byte {
	mode := byte('L')
	for _, a := range args {
		switch {
		case a == "--":
			return mode
		case isLogicalLong(a):
			mode = 'L'
		case isPhysicalLong(a):
			mode = 'P'
		case a == "--logical":
			mode = 'L'
		case a == "--physical":
			mode = 'P'
		case len(a) > 1 && a[0] == '-' && a[1] != '-':
			for _, c := range a[1:] {
				switch c {
				case 'L':
					mode = 'L'
				case 'P':
					mode = 'P'
				}
			}
		}
	}
	return mode
}

func isLogicalLong(arg string) bool {
	return longBoolFlagHasValue(arg, "logical")
}

func isPhysicalLong(arg string) bool {
	return longBoolFlagHasValue(arg, "physical")
}

func longBoolFlagHasValue(arg, name string) bool {
	if !strings.HasPrefix(arg, "--") || arg == "--" || len(arg) <= 2 {
		return false
	}
	if arg[2] == '-' {
		return false
	}
	nameAndValue := arg[2:]
	flagName, rawValue, hasValue := strings.Cut(nameAndValue, "=")
	if !strings.HasPrefix(name, flagName) {
		return false
	}
	if !hasValue {
		return true
	}
	if rawValue == "" {
		return false
	}
	value, err := strconv.ParseBool(rawValue)
	return err == nil && value
}
