// Package mkdircmd implements mkdir(1) per the GNU coreutils manual:
// create the DIRECTORY(ies), if they do not already exist.
//
// Portions adapted from https://github.com/u-root/u-root
// cmds/core/mkdir (BSD-3-Clause).
// Changes: rewired to the tool framework; per-component -p creation
// so -v reports each created ancestor; -m applies to the final
// directory only (intermediates get the default mode); -m refused on
// windows (no POSIX mode bits); symbolic modes follow chmod syntax.
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
	"strings"
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
	rc       *tool.RunContext
	parents  bool
	verbose  bool
	useMode  bool
	mode     os.FileMode
	symbolic *mkdirMode
	failed   bool
	deps     mkdirDeps
	mask     uint32
	maskSet  bool
}

type mkdirDeps struct {
	stat  func(string) (os.FileInfo, error)
	mkdir func(string, os.FileMode) error
	chmod func(string, os.FileMode) error
}

var defaultMkdirDeps = mkdirDeps{
	stat:  os.Stat,
	mkdir: os.Mkdir,
	chmod: os.Chmod,
}

func run(rc *tool.RunContext, args []string) int {
	fs := tool.NewFlags(cmd.Name)
	parents := fs.BoolP("parents", "p", false, "no error if existing, make parent directories as needed")
	modeStr := fs.StringP("mode", "m", "", "set file mode (octal, as in chmod), not a=rwx - umask")
	verbose := fs.BoolP("verbose", "v", false, "print a message for each created directory")
	contextStr := fs.StringP("context", "Z", "", "set SELinux security context (no-op without SELinux)")
	// GNU mkdir accepts -Z without an argument (equivalent to the default
	// SELinux context), as well as --context[=CTX].  Keep the value-bearing
	// form for --context=CTX while allowing the short option by itself.
	fs.Lookup("context").NoOptDefVal = "default"
	var operands []string
	var code int
	if envPresent(rc.Env, "POSIXLY_CORRECT") {
		// POSIX Utility Syntax Guideline 9 ends option recognition at the
		// first operand.  Consequently later -p and -- spellings are directory
		// pathnames, not options or an option terminator.
		operands, code = tool.ParseRequireOrder(rc, cmd, fs, args)
	} else {
		// Preserve GNU option permutation outside POSIX mode.
		operands, code = tool.Parse(rc, cmd, fs, args)
	}
	if code >= 0 {
		return code
	}
	_ = contextStr // deterministic no-op on non-SELinux platforms
	if len(operands) == 0 {
		return tool.UsageError(rc, cmd, "missing operand")
	}

	m := &maker{rc: rc, parents: *parents, verbose: *verbose, deps: defaultMkdirDeps}
	if fs.Changed("mode") {
		if runtime.GOOS == "windows" {
			return tool.NotSupported(rc, cmd, "-m/--mode on windows (no POSIX mode bits; mapping to read-only would change the documented meaning)")
		}
		mode, symbolic, errCode := parseMode(rc, *modeStr)
		if errCode >= 0 {
			return errCode
		}
		m.useMode = true
		m.mode = mode
		m.symbolic = symbolic
	}

	for _, op := range operands {
		m.make(op)
	}
	if m.failed {
		return 1
	}
	return 0
}

func envPresent(env []string, key string) bool {
	prefix := key + "="
	for _, entry := range env {
		if strings.HasPrefix(entry, prefix) {
			return true
		}
	}
	return false
}

// parseMode accepts octal MODE arguments (including setuid/setgid/
// sticky digits) and the symbolic mode grammar shared with chmod's
// documented behavior. Returns (mode, symbolic, -1) on success, or a
// zero mode and an exit code on failure.
func parseMode(rc *tool.RunContext, s string) (os.FileMode, *mkdirMode, int) {
	if isOctalMode(s) {
		numeric := s
		operatorPlus := s[0] == '+'
		if operatorPlus {
			numeric = s[1:]
		}
		n, err := strconv.ParseUint(numeric, 8, 32)
		if err != nil || n > 0o7777 {
			return 0, nil, tool.UsageError(rc, cmd, "invalid mode '%s'", s)
		}
		if operatorPlus {
			// GNU operator-numeric +MODE adds MODE to mkdir's a=rwx
			// point of departure rather than replacing it.
			n |= 0o777
		}
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
		return mode, nil, -1
	}
	if s == "" || allDigits(s) || !validSymbolicMode(s) {
		return 0, nil, tool.UsageError(rc, cmd, "invalid mode '%s'", s)
	}
	m, ok := parseSymbolicMode(s)
	if !ok {
		return 0, nil, tool.UsageError(rc, cmd, "invalid mode '%s'", s)
	}
	return 0, m, -1
}

func isOctalMode(s string) bool {
	if s == "" {
		return false
	}
	start := 0
	// GNU accepts a leading plus on a numeric mkdir mode. Retain that
	// documented extension outside the Issue 7 evidence surface.
	if s[0] == '+' {
		start = 1
		if len(s) == 1 {
			return false
		}
	}
	for i := start; i < len(s); i++ {
		if s[i] < '0' || s[i] > '7' {
			return false
		}
	}
	return true
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
		// POSIX pathname resolution: "a/b/" (and "a/b/.") name the same
		// directory as "a/b". Without trimming, filepath.Dir("a/b/") is
		// "a/b", so the parent recursion would create the FINAL component
		// as an intermediate — giving it the u+wx intermediate mode and
		// silently skipping -m.
		op = trimTrailingSep(op)
		ok, createdFinal := m.makeAll(op, true)
		if ok && createdFinal {
			m.applyFinalMode(op, createdFinal)
		}
		return
	}
	createMode, _ := m.finalMode()
	if err := m.deps.mkdir(m.rc.Path(op), createMode); err != nil {
		m.errf("cannot create directory '%s': %s", op, reason(err))
		return
	}
	m.verbosef("mkdir: created directory '%s'", op)
	m.applyFinalMode(op, true)
}

// trimTrailingSep drops trailing path separators (and a trailing "/."
// component) so the -p walk treats "a/b/" and "a/b/." as naming the
// directory a/b. It never trims below a root or volume root, and it
// leaves "." and ".." intact (dot-dot resolution stays with the OS).
func trimTrailingSep(op string) string {
	vol := len(filepath.VolumeName(op))
	for {
		switch {
		case len(op) > vol+1 && os.IsPathSeparator(op[len(op)-1]):
			op = op[:len(op)-1]
		case len(op) >= vol+2 && op[len(op)-1] == '.' && os.IsPathSeparator(op[len(op)-2]):
			op = op[:len(op)-1]
		default:
			return op
		}
	}
}

// makeAll is the -p path: create each missing component, reporting
// (ok, createdFinal). Existing directories are not an error; -m is
// applied by the caller, and only when the final component was
// actually created (GNU: -m affects the final directory only).
func (m *maker) makeAll(op string, final bool) (ok, createdSelf bool) {
	full := m.rc.Path(op)
	if fi, err := m.deps.stat(full); err == nil {
		if fi.IsDir() {
			return true, false
		}
		m.errf("cannot create directory '%s': File exists", op)
		return false, false
	}
	parent := filepath.Dir(op)
	if parent != op && parent != "." {
		if ok, _ := m.makeAll(parent, false); !ok {
			return false, false
		}
	}
	createMode := os.FileMode(0o777)
	if final {
		createMode, _ = m.finalMode()
	} else if runtime.GOOS != "windows" {
		createMode = m.parentMode()
	}
	if err := m.deps.mkdir(full, createMode); err != nil {
		if errors.Is(err, fs.ErrExist) {
			// A path may appear between the Stat and Mkdir calls. Treat the
			// race as success only when it is now a directory; a regular file
			// or dangling symlink still makes mkdir -p fail.
			if fi, statErr := m.deps.stat(full); statErr == nil && fi.IsDir() {
				return true, false
			}
		}
		m.errf("cannot create directory '%s': %s", op, reason(err))
		return false, false
	}
	if !final {
		if runtime.GOOS == "windows" {
			// Windows directory access is ACL-owned. os.Chmod only toggles a
			// read-only attribute, so applying a POSIX virtual mode here would
			// silently give the mask a different meaning.
			m.verbosef("mkdir: created directory '%s'", op)
			return true, true
		}
		// POSIX requires -p ancestors to retain owner write and search so
		// creation can descend even when the process umask masks those bits.
		// The same mode was supplied to mkdir above, so it is never exposed
		// more broadly than requested. A corrective chmod restores only bits
		// removed by a stricter host-process umask and preserves any special
		// bits the filesystem inherited from the parent directory.
		if !m.correctCreatedMode(op, m.parentMode(), true) {
			return false, false
		}
	}
	m.verbosef("mkdir: created directory '%s'", op)
	return true, true
}

// applyFinalMode sets the requested final directory mode. With -m MODE this is
// chmod-like and umask-independent. Without -m, embedded invocations with a
// virtual RunContext umask still need an explicit chmod because the process
// umask is not the shell's umask.
func (m *maker) applyFinalMode(op string, created bool) {
	if !created {
		return
	}
	mode, correct := m.finalMode()
	if !correct {
		return
	}
	m.correctCreatedMode(op, mode, !m.useMode)
}

// finalMode returns the mode to supply to mkdir and whether a post-create
// check/correction is required. A virtual umask is a Unix permission concept;
// on Windows the default creation path remains ACL-owned and -m is refused
// before a maker is constructed.
func (m *maker) finalMode() (os.FileMode, bool) {
	if !m.useMode {
		if !m.rc.UmaskSet || runtime.GOOS == "windows" {
			return 0o777, false
		}
		return os.FileMode(0o777 &^ m.umask()), true
	}
	mode := m.mode
	if m.symbolic != nil {
		// Symbolic mkdir modes are applied to the default creation mode,
		// not to the mode left after the kernel has applied the umask. This
		// matters for implicit-who clauses such as +x.
		mode = bitsToFileMode(m.symbolic.apply(0o777, m.umask()))
	}
	return mode, true
}

func (m *maker) parentMode() os.FileMode {
	return os.FileMode((0o777 &^ m.umask()) | 0o300)
}

// correctCreatedMode restores permission bits a host process umask removed
// from the already-restrictive mkdir mode. When no explicit -m controls this
// directory, retain special bits inherited from its parent instead of clearing
// them as a permission-only chmod would.
func (m *maker) correctCreatedMode(op string, requested os.FileMode, preserveInherited bool) bool {
	full := m.rc.Path(op)
	fi, err := m.deps.stat(full)
	if err != nil {
		m.errf("cannot set permissions of '%s': %s", op, reason(err))
		return false
	}
	target := requested
	if preserveInherited {
		target |= fi.Mode() & (os.ModeSetuid | os.ModeSetgid | os.ModeSticky)
	}
	const controlled = os.ModePerm | os.ModeSetuid | os.ModeSetgid | os.ModeSticky
	if fi.Mode()&controlled == target&controlled {
		return true
	}
	if err := m.deps.chmod(full, target); err != nil {
		m.errf("cannot set permissions of '%s': %s", op, reason(err))
		return false
	}
	return true
}

func (m *maker) umask() uint32 {
	if m.rc.UmaskSet {
		return uint32(m.rc.Umask.Perm()) & 0o777
	}
	if !m.maskSet {
		m.mask = processUmask()
		m.maskSet = true
	}
	return m.mask
}

type mkdirOp struct {
	who                  uint32
	explicit             bool
	op                   byte
	perm                 uint32
	copy                 byte
	condX, setID, sticky bool
}
type mkdirMode struct{ ops []mkdirOp }

const (
	modeU = 1 << iota
	modeG
	modeO
)

func validSymbolicMode(s string) bool {
	return strings.IndexFunc(s, func(r rune) bool { return !strings.ContainsRune("ugoarwxXst+-,=", r) }) == -1
}

func parseSymbolicMode(s string) (*mkdirMode, bool) {
	var m mkdirMode
	for _, clause := range strings.Split(s, ",") {
		i, who := 0, uint32(0)
		explicit := false
		for i < len(clause) {
			var bit uint32
			switch clause[i] {
			case 'u':
				bit = modeU
			case 'g':
				bit = modeG
			case 'o':
				bit = modeO
			case 'a':
				bit = modeU | modeG | modeO
			default:
				goto operators
			}
			who |= bit
			explicit = true
			i++
		}
	operators:
		if i == len(clause) {
			return nil, false
		}
		for i < len(clause) {
			op := clause[i]
			if op != '+' && op != '-' && op != '=' {
				return nil, false
			}
			i++
			o := mkdirOp{who: who, explicit: explicit, op: op}
			if i < len(clause) && (clause[i] == 'u' || clause[i] == 'g' || clause[i] == 'o') && (i+1 == len(clause) || strings.ContainsRune("+-=", rune(clause[i+1]))) {
				o.copy = clause[i]
				i++
			} else {
				for i < len(clause) && !strings.ContainsRune("+-=", rune(clause[i])) {
					switch clause[i] {
					case 'r':
						o.perm |= 4
					case 'w':
						o.perm |= 2
					case 'x':
						o.perm |= 1
					case 'X':
						o.condX = true
					case 's':
						o.setID = true
					case 't':
						o.sticky = true
					default:
						return nil, false
					}
					i++
				}
			}
			m.ops = append(m.ops, o)
		}
	}
	return &m, len(m.ops) > 0
}

func (m *mkdirMode) apply(cur, um uint32) uint32 {
	for _, o := range m.ops {
		perm := o.perm
		switch o.copy {
		case 'u':
			perm = (cur >> 6) & 7
		case 'g':
			perm = (cur >> 3) & 7
		case 'o':
			perm = cur & 7
		}
		if o.condX {
			perm |= 1
		}
		who := o.who
		if who == 0 {
			who = modeU | modeG | modeO
		}
		bits := uint32(0)
		if who&modeU != 0 {
			bits |= perm << 6
			if o.setID {
				bits |= 0o4000
			}
		}
		if who&modeG != 0 {
			bits |= perm << 3
			if o.setID {
				bits |= 0o2000
			}
		}
		if who&modeO != 0 {
			bits |= perm
		}
		if o.sticky {
			bits |= 0o1000
		}
		if !o.explicit {
			bits &^= um
		}
		switch o.op {
		case '+':
			cur |= bits
		case '-':
			cur &^= bits
		case '=':
			clear := uint32(0)
			if who&modeU != 0 {
				clear |= 0o4700
			}
			if who&modeG != 0 {
				clear |= 0o2070
			}
			if who&modeO != 0 {
				clear |= 0o0007
			}
			if !o.explicit || who&modeO != 0 {
				clear |= 0o1000
			}
			cur = cur&^clear | bits
		}
	}
	return cur
}

func bitsToFileMode(b uint32) os.FileMode {
	m := os.FileMode(b & 0o777)
	if b&0o4000 != 0 {
		m |= os.ModeSetuid
	}
	if b&0o2000 != 0 {
		m |= os.ModeSetgid
	}
	if b&0o1000 != 0 {
		m |= os.ModeSticky
	}
	return m
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
