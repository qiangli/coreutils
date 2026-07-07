// Package mkdircmd implements mkdir(1) per the GNU coreutils manual:
// create the DIRECTORY(ies), if they do not already exist.
//
// Portions adapted from https://github.com/u-root/u-root
// cmds/core/mkdir (BSD-3-Clause).
// Changes: rewired to the tool framework; per-component -p creation
// so -v reports each created ancestor; -m applies to the final
// directory only (intermediates get the default mode); -m refused on
// windows (no POSIX mode bits); symbolic modes refused (octal only).
// -Z/--context accepted as no-op on non-SELinux platforms.
package mkdircmd

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"unicode"

	"github.com/qiangli/coreutils/tool"
)

var cmd = &tool.Tool{
	Name:     "mkdir",
	Synopsis: "Create the DIRECTORY(ies), if they do not already exist.",
	Usage:    "mkdir [OPTION]... DIRECTORY...",
}

// Run is wired in init: a literal would create an initialization
// cycle (run's flag-error paths reference cmd).
func init() { cmd.Run = run; tool.Register(cmd) }

type maker struct {
	rc      *tool.RunContext
	parents bool
	verbose bool
	useMode bool
	mode    os.FileMode
	failed  bool
}

func run(rc *tool.RunContext, args []string) int {
	fs := tool.NewFlags(cmd.Name)
	parents := fs.BoolP("parents", "p", false, "no error if existing, make parent directories as needed")
	modeStr := fs.StringP("mode", "m", "", "set file mode (octal, as in chmod), not a=rwx - umask")
	verbose := fs.BoolP("verbose", "v", false, "print a message for each created directory")
	contextStr := fs.StringP("context", "Z", "", "set SELinux security context (no-op without SELinux)")
	operands, code := tool.Parse(rc, cmd, fs, args)
	if code >= 0 {
		return code
	}
	_ = contextStr // deterministic no-op on non-SELinux platforms
	if len(operands) == 0 {
		return tool.UsageError(rc, cmd, "missing operand")
	}

	m := &maker{rc: rc, parents: *parents, verbose: *verbose}
	if fs.Changed("mode") {
		if runtime.GOOS == "windows" {
			return tool.NotSupported(rc, cmd, "-m/--mode on windows (no POSIX mode bits; mapping to read-only would change the documented meaning)")
		}
		mode, errCode := parseMode(rc, *modeStr)
		if errCode >= 0 {
			return errCode
		}
		m.useMode = true
		m.mode = mode
	}

	for _, op := range operands {
		m.make(op)
	}
	if m.failed {
		return 1
	}
	return 0
}

// parseMode accepts octal MODE arguments (including setuid/setgid/
// sticky digits). Symbolic modes are documented GNU behavior this
// implementation deliberately does not cover. Returns (mode, -1) on
// success, or (0, exitCode).
func parseMode(rc *tool.RunContext, s string) (os.FileMode, int) {
	if n, err := strconv.ParseUint(s, 8, 32); err == nil && n <= 0o7777 {
		mode := os.FileMode(n & 0o777)
		if n&0o1000 != 0 {
			mode |= os.ModeSticky
		}
		if n&0o2000 != 0 {
			mode |= os.ModeSetgid
		}
		if n&0o4000 != 0 {
			mode |= os.ModeSetuid
		}
		return mode, -1
	}
	if s == "" || allDigits(s) {
		return 0, tool.UsageError(rc, cmd, "invalid mode '%s'", s)
	}
	return 0, tool.NotSupported(rc, cmd, fmt.Sprintf("symbolic mode '%s' for -m/--mode (only octal modes)", s))
}

func allDigits(s string) bool {
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return s != ""
}

func (m *maker) make(op string) {
	if op == "" {
		m.errf("cannot create directory '': No such file or directory")
		return
	}
	if m.parents {
		ok, createdFinal := m.makeAll(op)
		if ok && createdFinal {
			m.applyMode(op)
		}
		return
	}
	if err := os.Mkdir(m.rc.Path(op), 0o777); err != nil {
		m.errf("cannot create directory '%s': %s", op, reason(err))
		return
	}
	m.verbosef("mkdir: created directory '%s'", op)
	m.applyMode(op)
}

// makeAll is the -p path: create each missing component, reporting
// (ok, createdFinal). Existing directories are not an error; -m is
// applied by the caller, and only when the final component was
// actually created (GNU: -m affects the final directory only).
func (m *maker) makeAll(op string) (ok, createdSelf bool) {
	full := m.rc.Path(op)
	if fi, err := os.Stat(full); err == nil {
		if fi.IsDir() {
			return true, false
		}
		m.errf("cannot create directory '%s': File exists", op)
		return false, false
	}
	parent := filepath.Dir(op)
	if parent != op && parent != "." {
		if ok, _ := m.makeAll(parent); !ok {
			return false, false
		}
	}
	if err := os.Mkdir(full, 0o777); err != nil {
		if errors.Is(err, fs.ErrExist) {
			return true, false
		}
		m.errf("cannot create directory '%s': %s", op, reason(err))
		return false, false
	}
	m.verbosef("mkdir: created directory '%s'", op)
	return true, true
}

// applyMode sets -m MODE on the (just created) final directory, as
// with chmod: umask does not apply.
func (m *maker) applyMode(op string) {
	if !m.useMode {
		return
	}
	if err := os.Chmod(m.rc.Path(op), m.mode); err != nil {
		m.errf("cannot set permissions of '%s': %s", op, reason(err))
	}
}

func (m *maker) errf(format string, a ...any) {
	fmt.Fprintf(m.rc.Err, "mkdir: "+format+"\n", a...)
	m.failed = true
}

func (m *maker) verbosef(format string, a ...any) {
	if m.verbose {
		fmt.Fprintf(m.rc.Out, format+"\n", a...)
	}
}

// reason unwraps err to its root cause and capitalizes the first
// letter, matching the strerror() shape GNU diagnostics use.
func reason(err error) string {
	var pe *os.PathError
	if errors.As(err, &pe) {
		err = pe.Err
	}
	var se *os.SyscallError
	if errors.As(err, &se) {
		err = se.Err
	}
	s := err.Error()
	if s == "" {
		return s
	}
	r := []rune(s)
	r[0] = unicode.ToUpper(r[0])
	return string(r)
}
