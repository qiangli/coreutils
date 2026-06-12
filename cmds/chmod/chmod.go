// Package chmodcmd implements chmod(1) per the GNU coreutils manual:
// change file mode bits, with octal and symbolic modes and -R.
//
// Unix only: Windows has no POSIX mode bits, and mapping modes onto the
// read-only attribute would change the documented meaning, so the
// Windows build fails loudly instead (see chmod_windows.go).
//
// Portions adapted from https://github.com/u-root/u-root cmds/core/chmod (BSD-3-Clause).
// Changes: rewired to tool framework; symbolic-mode parser extended to
// full GNU clause grammar (comma-separated clauses, multiple operators
// per clause, rwxXst perms, u/g/o permission copying, umask handling
// for empty who, setuid/setgid/sticky); octal modes up to 7777 with the
// GNU keep-directory-setid rule for fewer than 5 digits.
package chmodcmd

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/qiangli/coreutils/tool"
)

var cmd = &tool.Tool{
	Name:     "chmod",
	Synopsis: "Change the mode of each FILE to MODE.",
	Usage: "chmod [OPTION]... MODE[,MODE]... FILE...\n" +
		"   or: chmod [OPTION]... OCTAL-MODE FILE...",
}

// Run is wired in init: a literal would create an initialization cycle.
func init() { cmd.Run = run; tool.Register(cmd) }

func run(rc *tool.RunContext, args []string) int {
	modeArg, rest := extractDashMode(args)
	fs := tool.NewFlags(cmd.Name)
	recursive := fs.BoolP("recursive", "R", false, "change files and directories recursively")
	operands, code := tool.Parse(rc, cmd, fs, rest)
	if code >= 0 {
		return code
	}
	if modeArg != "" {
		operands = append([]string{modeArg}, operands...)
	}
	if len(operands) == 0 {
		return tool.UsageError(rc, cmd, "missing operand")
	}
	if len(operands) == 1 {
		return tool.UsageError(rc, cmd, "missing operand after '%s'", operands[0])
	}
	change, err := parseMode(operands[0])
	if err != nil {
		fmt.Fprintf(rc.Err, "chmod: invalid mode: '%s'\n", operands[0])
		return 1
	}
	return apply(rc, change, operands[1:], *recursive)
}

// extractDashMode rescues dash-prefixed mode operands (chmod -w FILE,
// chmod -rx FILE) before pflag sees them. An argument qualifies when
// every character after the dash belongs to the mode alphabet — which
// excludes every flag chmod defines (-R, --recursive, --help, ...).
func extractDashMode(args []string) (mode string, rest []string) {
	rest = make([]string, 0, len(args))
	for i, a := range args {
		if a == "--" {
			rest = append(rest, args[i:]...)
			break
		}
		if mode == "" && len(a) > 1 && a[0] == '-' && a[1] != '-' && isModeBody(a[1:]) {
			mode = a
			continue
		}
		rest = append(rest, a)
	}
	return mode, rest
}

func isModeBody(s string) bool {
	for i := 0; i < len(s); i++ {
		if !strings.ContainsRune("ugoarwxXst+-=,", rune(s[i])) {
			return false
		}
	}
	return true
}

const (
	whoU = 1 << 2
	whoG = 1 << 1
	whoO = 1 << 0
)

type symOp struct {
	who      uint32 // bitset of whoU/whoG/whoO; 0 never stored (empty -> all)
	explicit bool   // who was written (umask does not apply)
	op       byte   // '+', '-', '='
	perm     uint32 // rwx bits (r=4 w=2 x=1)
	copyFrom byte   // 'u', 'g', 'o' or 0
	condX    bool   // X: execute only for directories / already-executable
	setid    bool   // s
	sticky   bool   // t
}

type modeChange struct {
	octal  bool
	val    uint32
	digits int
	ops    []symOp
}

var errInvalidMode = errors.New("invalid mode")

func parseMode(s string) (*modeChange, error) {
	if s == "" {
		return nil, errInvalidMode
	}
	octal := true
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '7' {
			octal = false
			break
		}
	}
	if octal {
		v, err := strconv.ParseUint(s, 8, 32)
		if err != nil || v > 0o7777 {
			return nil, errInvalidMode
		}
		return &modeChange{octal: true, val: uint32(v), digits: len(s)}, nil
	}

	mc := &modeChange{}
	for _, clause := range strings.Split(s, ",") {
		i := 0
		var who uint32
		explicit := false
	wholoop:
		for ; i < len(clause); i++ {
			switch clause[i] {
			case 'u':
				who, explicit = who|whoU, true
			case 'g':
				who, explicit = who|whoG, true
			case 'o':
				who, explicit = who|whoO, true
			case 'a':
				who, explicit = who|whoU|whoG|whoO, true
			default:
				break wholoop
			}
		}
		if i >= len(clause) {
			return nil, errInvalidMode // who with no operator
		}
		for i < len(clause) {
			op := clause[i]
			if op != '+' && op != '-' && op != '=' {
				return nil, errInvalidMode
			}
			i++
			so := symOp{who: who, explicit: explicit, op: op}
			// Permission copy: exactly one of u/g/o, alone until the
			// next operator or the end of the clause.
			if i < len(clause) && (clause[i] == 'u' || clause[i] == 'g' || clause[i] == 'o') &&
				(i+1 == len(clause) || clause[i+1] == '+' || clause[i+1] == '-' || clause[i+1] == '=') {
				so.copyFrom = clause[i]
				i++
			} else {
			permloop:
				for i < len(clause) {
					switch clause[i] {
					case 'r':
						so.perm |= 4
					case 'w':
						so.perm |= 2
					case 'x':
						so.perm |= 1
					case 'X':
						so.condX = true
					case 's':
						so.setid = true
					case 't':
						so.sticky = true
					case '+', '-', '=':
						break permloop
					default:
						return nil, errInvalidMode
					}
					i++
				}
			}
			mc.ops = append(mc.ops, so)
		}
	}
	if len(mc.ops) == 0 {
		return nil, errInvalidMode
	}
	return mc, nil
}

// apply computes the new mode bits (07777 region) from the old ones.
// um is the process umask (0777 region), consulted only for clauses
// with no explicit who, exactly as the GNU manual specifies.
func (mc *modeChange) apply(old uint32, isDir bool, um uint32) uint32 {
	if mc.octal {
		v := mc.val
		// GNU: a numeric mode of 4 or fewer digits leaves a directory's
		// setuid/setgid bits alone (they can be set, not cleared).
		if isDir && mc.digits < 5 {
			v |= old & 0o6000
		}
		return v
	}
	cur := old
	for _, so := range mc.ops {
		perm := so.perm
		switch so.copyFrom {
		case 'u':
			perm = (cur >> 6) & 7
		case 'g':
			perm = (cur >> 3) & 7
		case 'o':
			perm = cur & 7
		}
		if so.condX && (isDir || cur&0o111 != 0) {
			perm |= 1
		}
		who := so.who
		if who == 0 {
			who = whoU | whoG | whoO
		}
		var bits uint32
		if who&whoU != 0 {
			bits |= perm << 6
			if so.setid {
				bits |= 0o4000
			}
		}
		if who&whoG != 0 {
			bits |= perm << 3
			if so.setid {
				bits |= 0o2000
			}
		}
		if who&whoO != 0 {
			bits |= perm
		}
		if so.sticky {
			bits |= 0o1000
		}
		if !so.explicit {
			bits &^= um // bits set in the umask are not affected
		}
		switch so.op {
		case '+':
			cur |= bits
		case '-':
			cur &^= bits
		case '=':
			var clear uint32
			if who&whoU != 0 {
				clear |= 0o4700
			}
			if who&whoG != 0 {
				clear |= 0o2070
			}
			if who&whoO != 0 {
				clear |= 0o0007
			}
			if !so.explicit || who&whoO != 0 {
				clear |= 0o1000
			}
			cur = cur&^clear | bits
		}
	}
	return cur
}

// fileModeToBits converts an os.FileMode to POSIX 07777 bits.
func fileModeToBits(m os.FileMode) uint32 {
	bits := uint32(m.Perm())
	if m&os.ModeSetuid != 0 {
		bits |= 0o4000
	}
	if m&os.ModeSetgid != 0 {
		bits |= 0o2000
	}
	if m&os.ModeSticky != 0 {
		bits |= 0o1000
	}
	return bits
}

// bitsToFileMode converts POSIX 07777 bits to an os.FileMode.
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

// reason unwraps os wrapper errors so diagnostics read like GNU's.
func reason(err error) error {
	var pe *os.PathError
	if errors.As(err, &pe) {
		return pe.Err
	}
	return err
}
