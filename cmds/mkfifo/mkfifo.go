// Package mkfifocmd implements mkfifo(1): create named pipes.
//
// The native operation is split behind build tags so non-Unix platforms
// fail loudly instead of approximating FIFO semantics.
// -Z/--context accepted as no-op on non-SELinux platforms.
package mkfifocmd

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/qiangli/coreutils/tool"
)

var cmd = &tool.Tool{
	Name:     "mkfifo",
	Synopsis: "Create named pipes (FIFOs).",
	Usage:    "mkfifo [OPTION]... NAME...",
}

// Run is wired in init: a literal would create an initialization cycle.
func init() { cmd.Run = run; tool.Register(cmd) }

func run(rc *tool.RunContext, args []string) int {
	fs := tool.NewFlags(cmd.Name)
	modeStr := fs.StringP("mode", "m", "", "set file mode (octal, as in chmod), not a=rw - umask")
	contextStr := fs.StringP("context", "Z", "", "set SELinux security context (no-op without SELinux)")
	var operands []string
	var code int
	if envPresent(rc.Env, "POSIXLY_CORRECT") {
		operands, code = tool.ParseRequireOrder(rc, cmd, fs, args)
	} else {
		operands, code = tool.Parse(rc, cmd, fs, args)
	}
	if code >= 0 {
		return code
	}
	_ = contextStr // deterministic no-op on non-SELinux platforms
	if len(operands) == 0 {
		fmt.Fprintln(rc.Err, "mkfifo: missing operand")
		fmt.Fprintln(rc.Err, "Try 'mkfifo --help' for more information.")
		return 1
	}

	mode := uint32(0o666)
	useMode := fs.Changed("mode")
	if useMode {
		var errCode int
		mode, errCode = parseMode(rc, *modeStr)
		if errCode >= 0 {
			return errCode
		}
	} else if rc.UmaskSet {
		// The native syscall cannot see the embedding shell's virtual mask.
		// Restrict the creation mode up front; the chmod below restores bits
		// removed by an unrelated, stricter host-process mask.
		mode &^= uint32(rc.Umask.Perm())
	}

	status := 0
	for _, name := range operands {
		if err := makeFIFO(rc.Path(name), mode); err != nil {
			fmt.Fprintf(rc.Err, "mkfifo: cannot create fifo '%s': %s\n", name, tool.SysErrString(err))
			status = 1
			continue
		}
		if useMode || rc.UmaskSet {
			if err := os.Chmod(rc.Path(name), fileMode(mode)); err != nil {
				fmt.Fprintf(rc.Err, "mkfifo: cannot set permissions of '%s': %s\n", name, tool.SysErrString(err))
				status = 1
			}
		}
	}
	return status
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

func parseMode(rc *tool.RunContext, s string) (uint32, int) {
	n, err := strconv.ParseUint(s, 8, 32)
	if err == nil && n <= 0o7777 {
		return uint32(n), -1
	}
	if s == "" || allDigits(s) {
		fmt.Fprintln(rc.Err, "mkfifo: invalid mode")
		return 0, 1
	}
	change, ok := parseSymbolicMode(s)
	if !ok {
		fmt.Fprintln(rc.Err, "mkfifo: invalid mode")
		return 0, 1
	}
	mask := uint32(0)
	if change.needsUmask() {
		mask = effectiveUmask(rc)
	}
	// POSIX specifies a symbolic mkfifo mode relative to an assumed a=rw
	// initial mode. As with chmod, clauses that omit who may not change bits
	// selected by the process (or embedding shell's virtual) umask.
	return change.apply(0o666, mask), -1
}

func (m *symbolicMode) needsUmask() bool {
	for _, change := range m.ops {
		if !change.explicitWho {
			return true
		}
	}
	return false
}

func allDigits(s string) bool {
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return s != ""
}

const (
	whoUser = 1 << iota
	whoGroup
	whoOther
)

type modeOp struct {
	who                 uint32
	explicitWho         bool
	op                  byte
	perm                uint32
	copy                byte
	conditionalX, setID bool
	sticky              bool
}

type symbolicMode struct {
	ops []modeOp
}

func parseSymbolicMode(s string) (*symbolicMode, bool) {
	var mode symbolicMode
	for _, clause := range strings.Split(s, ",") {
		i, who := 0, uint32(0)
		for i < len(clause) {
			switch clause[i] {
			case 'u':
				who |= whoUser
			case 'g':
				who |= whoGroup
			case 'o':
				who |= whoOther
			case 'a':
				who |= whoUser | whoGroup | whoOther
			default:
				goto operators
			}
			i++
		}
	operators:
		if i == len(clause) {
			return nil, false
		}
		explicitWho := who != 0
		for i < len(clause) {
			op := clause[i]
			if op != '+' && op != '-' && op != '=' {
				return nil, false
			}
			i++
			change := modeOp{who: who, explicitWho: explicitWho, op: op}
			if i < len(clause) && strings.ContainsRune("ugo", rune(clause[i])) &&
				(i+1 == len(clause) || strings.ContainsRune("+-=", rune(clause[i+1]))) {
				change.copy = clause[i]
				i++
			} else {
				for i < len(clause) && !strings.ContainsRune("+-=", rune(clause[i])) {
					switch clause[i] {
					case 'r':
						change.perm |= 4
					case 'w':
						change.perm |= 2
					case 'x':
						change.perm |= 1
					case 'X':
						change.conditionalX = true
					case 's':
						change.setID = true
					case 't':
						change.sticky = true
					default:
						return nil, false
					}
					i++
				}
			}
			mode.ops = append(mode.ops, change)
		}
	}
	return &mode, len(mode.ops) > 0
}

func (m *symbolicMode) apply(current, mask uint32) uint32 {
	for _, change := range m.ops {
		perm := change.perm
		switch change.copy {
		case 'u':
			perm = current >> 6 & 7
		case 'g':
			perm = current >> 3 & 7
		case 'o':
			perm = current & 7
		}
		if change.conditionalX && current&0o111 != 0 {
			perm |= 1
		}

		who := change.who
		if who == 0 {
			who = whoUser | whoGroup | whoOther
		}
		bits, clear := uint32(0), uint32(0)
		if who&whoUser != 0 {
			bits |= perm << 6
			clear |= 0o4700
			if change.setID {
				bits |= 0o4000
			}
		}
		if who&whoGroup != 0 {
			bits |= perm << 3
			clear |= 0o2070
			if change.setID {
				bits |= 0o2000
			}
		}
		if who&whoOther != 0 {
			bits |= perm
			clear |= 0o1007
		}
		if change.sticky {
			bits |= 0o1000
		}
		if !change.explicitWho {
			bits &^= mask
		}

		switch change.op {
		case '+':
			current |= bits
		case '-':
			current &^= bits
		case '=':
			current = current&^clear | bits
		}
	}
	return current
}

func fileMode(n uint32) os.FileMode {
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
	return mode
}
