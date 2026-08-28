// Package chmodcmd implements chmod(1), with POSIX.1-2008 Issue 7 mode
// semantics and a separately documented set of compatibility extensions.
//
// Unix only: Windows has no POSIX mode bits, and mapping modes onto the
// read-only attribute would change the documented meaning, so the
// non-Unix build fails loudly instead (see chmod_other.go).
//
// Portions adapted from https://github.com/u-root/u-root cmds/core/chmod (BSD-3-Clause).
// Changes: rewired to tool framework; symbolic-mode parser extended to
// full POSIX clause grammar (comma-separated clauses, multiple operators
// per clause, rwxXst perms, u/g/o permission copying, umask handling
// for empty who, and setuid/setgid/sticky); octal modes up to 7777 are
// absolute, and X always examines the invocation's unmodified file mode.
package chmodcmd

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/qiangli/coreutils/tool"
)

// derefMode is the symbolic-link policy selected by the -H/-L/-P and
// --dereference/--no-dereference extensions. It is declared here rather
// than beside the Unix walk so the non-Unix build parses the same
// command line and refuses on the same terms.
type derefMode int

const (
	// derefNever is -P: a symbolic link is neither followed nor changed.
	derefNever derefMode = iota
	// derefCmdLine is -H: a link named as an operand is followed.
	derefCmdLine
	// derefAlways is -L/--dereference: every link is followed.
	derefAlways
)

// options is the parsed command line handed to the platform apply. It
// mirrors the shape cmds/chgrp and cmds/chown use, so the three
// utilities' recursion and diagnostics stay comparable.
type options struct {
	files        []string
	recursive    bool
	verbose      bool
	changes      bool
	silent       bool
	preserveRoot bool
	deref        derefMode
}

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
	verbose := fs.BoolP("verbose", "v", false, "output a diagnostic for every file processed")
	changes := fs.BoolP("changes", "c", false, "like verbose but report only when a change is made")
	silent := fs.BoolP("silent", "f", false, "suppress most error messages")
	fs.Bool("quiet", false, "suppress most error messages")
	preserveRoot := fs.Bool("preserve-root", false, "fail to operate recursively on '/'")
	fs.Bool("no-preserve-root", false, "do not treat '/' specially (the default)")
	reference := fs.String("reference", "", "use RFILE's mode instead of MODE")
	fs.Bool("dereference", false, "affect the referent of each symbolic link (the default)")
	fs.Bool("no-dereference", false, "do not affect the referent of symbolic links")
	fs.BoolP("P", "P", false, "never follow symbolic links (with -R)")
	fs.BoolP("H", "H", false, "follow command-line symbolic links (with -R)")
	fs.BoolP("L", "L", false, "follow every symbolic link encountered (with -R)")
	operands, code := tool.ParseRequireOrder(rc, cmd, fs, rest)
	if code >= 0 {
		return code
	}

	opts := options{
		recursive:    *recursive,
		verbose:      *verbose,
		changes:      *changes,
		silent:       *silent || isBool(fs, "quiet"),
		preserveRoot: *preserveRoot,
		deref:        derefFlags(rest, *recursive),
	}

	if *reference != "" {
		if modeArg != "" {
			return tool.UsageError(rc, cmd, "cannot combine --reference and MODE")
		}
		if len(operands) == 0 {
			return tool.UsageError(rc, cmd, "missing operand")
		}
		fi, err := os.Stat(rc.Path(*reference))
		if err != nil {
			fmt.Fprintf(rc.Err, "chmod: cannot stat '%s': %v\n", *reference, err)
			return 1
		}
		// --reference means "use RFILE's mode" exactly. Reuse the absolute
		// octal engine rather than reparsing a symbolic expression.
		refMode := fmt.Sprintf("%05o", fileModeToBits(fi.Mode()))
		change, err := parseMode(refMode)
		if err != nil {
			fmt.Fprintf(rc.Err, "chmod: invalid mode from reference: '%s'\n", refMode)
			return 1
		}
		opts.files = operands
		return apply(rc, change, opts)
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
	opts.files = operands[1:]
	return apply(rc, change, opts)
}

func isBool(fs interface{ GetBool(string) (bool, error) }, name string) bool {
	v, err := fs.GetBool(name)
	return err == nil && v
}

// extractDashMode rescues dash-prefixed mode operands (chmod -w FILE,
// chmod -rx FILE) before pflag sees them. An argument qualifies when
// every character after the dash belongs to the mode alphabet — which
// excludes every flag chmod defines (-R, --recursive, --help, ...).
func extractDashMode(args []string) (mode string, rest []string) {
	rest = make([]string, 0, len(args))
	sawOperand := false
	for i := 0; i < len(args); i++ {
		a := args[i]
		if a == "--" {
			rest = append(rest, args[i:]...)
			break
		}
		if strings.HasPrefix(a, "--") && len(a) > 2 {
			name, _, hasValue := strings.Cut(a[2:], "=")
			rest = append(rest, a)
			// A separately-spelled --reference value is data even when it
			// looks exactly like a symbolic mode (for example "-w").
			// Honor the framework's unambiguous long-option abbreviations.
			if !hasValue && longOptionPrefix(name, "reference") && i+1 < len(args) {
				i++
				rest = append(rest, args[i])
			}
			continue
		}
		if !sawOperand && mode == "" && len(a) > 1 && a[0] == '-' && a[1] != '-' && isModeBody(a[1:]) {
			mode = a
			sawOperand = true
			// Removing a dash-prefixed mode would otherwise expose later
			// file names to the option parser. Retain its operand boundary.
			rest = append(rest, "--")
			if i+1 < len(args) && args[i+1] == "--" {
				rest = append(rest, args[i+2:]...)
			} else {
				rest = append(rest, args[i+1:]...)
			}
			break
		}
		if a == "-" || !strings.HasPrefix(a, "-") {
			rest = append(rest, args[i:]...)
			break
		}
		rest = append(rest, a)
	}
	return mode, rest
}

func longOptionPrefix(name, full string) bool {
	return name != "" && strings.HasPrefix(full, name)
}

func isModeBody(s string) bool {
	for i := 0; i < len(s); i++ {
		if !strings.ContainsRune("ugoarwxXst+-=,", rune(s[i])) {
			return false
		}
	}
	return true
}

// derefFlags returns the effective symbolic-link policy after chmod's
// order-sensitive dereference options have been applied. They have no
// long form that pflag could order, and POSIX gives the same rule to
// every such group: the last one specified wins. Without -R a link
// operand names its referent, so the default is to follow; with -R the
// default is the physical walk, which neither follows a link nor
// changes one.
func derefFlags(args []string, recursive bool) derefMode {
	mode := derefAlways
	if recursive {
		mode = derefNever
	}
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--":
			return mode
		case a == "-" || !strings.HasPrefix(a, "-"):
			return mode
		case strings.HasPrefix(a, "--") && len(a) > 2:
			name, _, hasValue := strings.Cut(a[2:], "=")
			switch {
			case longOptionPrefix(name, "dereference"), name == "L":
				mode = derefAlways
			case longOptionPrefix(name, "no-dereference"), name == "P":
				mode = derefNever
			case name == "H":
				mode = derefCmdLine
			}
			// An option value is never another option. In particular, a
			// reference file named -L must not select logical traversal.
			if !hasValue && longOptionPrefix(name, "reference") && i+1 < len(args) {
				i++
			}
		case len(a) > 1 && a[0] == '-' && a[1] != '-':
			for _, c := range a[1:] {
				switch c {
				case 'H':
					mode = derefCmdLine
				case 'L':
					mode = derefAlways
				case 'P':
					mode = derefNever
				}
			}
		}
	}
	return mode
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
	octal bool
	val   uint32
	ops   []symOp
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
		return &modeChange{octal: true, val: uint32(v)}, nil
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
// um is the invoking process's file creation mask (0777 region), consulted
// only for clauses with no explicit who, as Issue 7 specifies.
func (mc *modeChange) apply(old uint32, isDir bool, um uint32) uint32 {
	if mc.octal {
		// POSIX Issue 7: an octal mode sets all listed mode bits
		// absolutely, including set-user-ID and set-group-ID on regular
		// files. No environment variable selects a different grammar.
		return mc.val
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
		// "Current (unmodified)" is the mode before this invocation, not
		// the in-progress result of an earlier clause.
		if so.condX && (isDir || old&0o111 != 0) {
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

// reason unwraps os wrapper errors so diagnostics report the filesystem cause.
func reason(err error) error {
	return tool.SysErr(err)
}
