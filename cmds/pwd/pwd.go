// Package pwdcmd implements pwd(1) per the GNU coreutils manual:
// print the name of the current working directory.
//
// The "current working directory" is the invocation's rc.Dir, never the
// process cwd (the embedding shell owns its own cwd). Logical mode uses
// PWD from the invocation environment when it is a valid name for rc.Dir;
// physical mode and invalid logical names resolve every symlink in rc.Dir —
// through the kernel when rc.Dir IS the process cwd (DirIsProcessCwd), and
// through EvalSymlinks otherwise. When both -L and -P are given, the last
// one takes precedence.
package pwdcmd

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"

	"github.com/qiangli/coreutils/tool"
)

var cmd = &tool.Tool{
	Name:     "pwd",
	Synopsis: "Print the full filename of the current working directory.",
	Usage:    "pwd [OPTION]...",
}

// processGetwd reports the process's PHYSICAL working directory, the way
// GNU pwd -P gets it: from the kernel, whose getcwd(2) never reports a
// symlink component.
//
// It must NOT be os.Getwd. On every Unix that call opens with a "clumsy but
// widespread kludge" (its own words): if the process environment's $PWD is
// absolute and names the current directory, $PWD is returned verbatim. After
// a logical `cd` through a symlink $PWD holds exactly the symlinked spelling
// that -P exists to resolve, so os.Getwd hands back the logical answer. It
// also reads the PROCESS environment, which a tool must never consult — the
// invocation environment is rc.Env.
//
// os.Getwd stays as the fallback for the case the kernel cannot answer: a
// current directory whose absolute name overruns the getcwd buffer. There
// os.Getwd's slow ".."-walk reconstructs a physical name that the syscall
// alone cannot produce.
var processGetwd = physicalGetwd

func physicalGetwd() (string, error) {
	if dir, err := syscall.Getwd(); err == nil && filepath.IsAbs(dir) {
		return dir, nil
	}
	return os.Getwd()
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
	defaultMode := byte('P')
	for _, env := range rc.Env {
		if strings.HasPrefix(env, "POSIXLY_CORRECT=") {
			defaultMode = 'L'
			break
		}
	}
	if lastLP(args, defaultMode) == 'L' {
		if logical := logicalDir(rc); logical != "" {
			return writeDirectory(rc, logical)
		}
	}

	resolved, err := physicalDir(rc)
	if err != nil {
		fmt.Fprintf(rc.Err, "pwd: %v\n", err)
		return 1
	}
	return writeDirectory(rc, resolved)
}

func writeDirectory(rc *tool.RunContext, dir string) int {
	if _, err := fmt.Fprintln(rc.Out, dir); err != nil {
		_, _ = fmt.Fprintf(rc.Err, "pwd: write error: %v\n", err)
		return 1
	}
	return 0
}

// logicalDir returns PWD only when it satisfies the POSIX invariants for a
// logical working-directory name and identifies rc.Dir.
func logicalDir(rc *tool.RunContext) string {
	pwd := rc.Getenv("PWD")
	if !filepath.IsAbs(pwd) || hasDotComponent(pwd) {
		return ""
	}
	if rc.DirIsProcessCwd {
		// PWD is only authoritative while it still names the process cwd. It
		// may have been removed or repointed after a logical cd; comparing it
		// with "." detects both cases without trusting rc.Dir's spelling.
		pwdInfo, err := os.Stat(pwd)
		if err == nil {
			cwdInfo, cwdErr := os.Stat(".")
			if cwdErr == nil && os.SameFile(pwdInfo, cwdInfo) {
				return pwd
			}
			return ""
		}
		// A kernel-owned current directory can have a valid logical name whose
		// absolute spelling exceeds PATH_MAX. Preserve that established case;
		// shorter missing or inaccessible names must fall back to physical pwd.
		if pwd == rc.Dir && errors.Is(err, syscall.ENAMETOOLONG) {
			return pwd
		}
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

func physicalDir(rc *tool.RunContext) (string, error) {
	if rc.DirIsProcessCwd {
		resolved, err := processGetwd()
		if err != nil {
			return "", err
		}
		if !filepath.IsAbs(resolved) {
			return "", fmt.Errorf("cannot determine absolute path for %q", rc.Dir)
		}
		return resolved, nil
	}
	resolved, err := filepath.EvalSymlinks(rc.Dir)
	if err != nil {
		return "", err
	}
	if !filepath.IsAbs(resolved) {
		return "", fmt.Errorf("cannot determine absolute path for %q", rc.Dir)
	}
	return resolved, nil
}

// lastLP scans args for the last occurrence of -L/-P (or their long
// forms) — pflag reports values but not order, and GNU specifies that
// the last of the two wins.
func lastLP(args []string, defaultMode byte) byte {
	mode := defaultMode
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
