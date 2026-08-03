// Package testcmd implements test(1) and its `[` spelling: evaluate a
// conditional EXPRESSION and exit 0 (true), 1 (false), or 2 (a syntax
// or usage error). Behavior follows POSIX (XCU `test`) and the GNU
// coreutils manual; nothing here is derived from a shell's builtin or
// from GNU source.
//
// The two names share one implementation, exactly as upstream does.
// `[` requires a final `]` operand — a missing one is a syntax error
// naming the bracket.
//
// Options are the one place these two names differ, and the difference
// is upstream's: `test` accepts NO options at all. The GNU manual states
// it outright — test treats `--help` and `--version` like any other
// non-empty STRING — so `test --help` exits 0 and prints nothing. Only
// `[` recognizes them, only as its single argument, and only before the
// closing bracket is considered: `[ --help` prints the help text, while
// `[ --help ]` is a string test that exits 0 silently. Matching is
// literal, so `[ --he` is a one-operand string test rather than an
// abbreviation. That is also why this tool bypasses tool.Parse: an
// expression's operands are not flags, and the framework's long-option
// abbreviation would rewrite them.
//
// Operand counts 0 through 4 are the POSIX-specified forms and are
// evaluated exactly as specified (0 operands is false; 1 operand is
// true when non-empty; 2/3/4 dispatch on `!`, `(`…`)`, and the unary
// and binary primaries). Longer expressions are unspecified by POSIX;
// they are evaluated by the same recursive-descent grammar GNU
// documents — `-a` binds tighter than `-o`, `!` negates a term, and
// `(` … `)` group. Neither operator short-circuits, so a syntax error
// in any branch is always reported.
//
// Deliberate, documented deviations from GNU:
//
//   - `-o` is the OR operator only, which is the only meaning the GNU
//     manual gives it. GNU's binary additionally accepts an
//     undocumented unary `-o` (a shell-option test) and answers false;
//     here `test -o NAME` is a syntax error (exit 2) naming the
//     operand, per the contract's "loud, never silently approximate".
//   - `-l STRING` (an undocumented string-length modifier in front of a
//     numeric comparison) is not implemented; it is a syntax error.
//   - `==` is accepted as a synonym for `=`, a documented superset: it
//     is what every shell's `[` accepts, and `=` keeps its exact
//     upstream meaning either way.
//   - Integer operands are arbitrary precision. GNU is limited to
//     intmax_t and fails on wider values; comparing them exactly is a
//     superset, never a different answer for values GNU accepts.
//   - `-t` resolves file descriptors 0, 1 and 2 through the
//     RunContext streams, because an embedded tool has no process of
//     its own (see the tool package). For larger descriptors it probes
//     the host descriptor table. That is exact at a standalone process
//     boundary and lets an embedding shell expose its inherited/open
//     descriptors without copying them into RunContext.
//   - `-O`/`-G` (ownership) require POSIX uid/gid and fail loudly with
//     exit 2 on platforms that have none, rather than answering false.
//   - Diagnostics name the operand that is actually wrong. Where GNU
//     reports a two-operand expression like `test x y` as "missing
//     argument after 'y'", this says "'x': unary operator expected".
//     The exit status is 2 either way; only the wording differs, and
//     it is a deterministic function of the operands.
//
// String comparison (`=`, `!=`, `<`, `>`) is byte-wise, which is the
// LC_ALL=C collating order the agent contract mandates.
package testcmd

import (
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"strings"

	"github.com/qiangli/coreutils/tool"
)

var cmd = &tool.Tool{
	Name:     "test",
	Synopsis: "Evaluate a conditional expression.",
	Usage:    "test EXPRESSION\n  or:  test",
}

var bracketCmd = &tool.Tool{
	Name:     "[",
	Synopsis: "Evaluate a conditional expression (test, spelled with a closing ']').",
	Usage:    "[ EXPRESSION ]\n  or:  [ ]",
}

// Run is wired in init: a literal would create an initialization cycle
// (run's diagnostics reference the tools).
func init() {
	cmd.Run = func(rc *tool.RunContext, args []string) int { return run(rc, cmd, args) }
	bracketCmd.Run = func(rc *tool.RunContext, args []string) int { return run(rc, bracketCmd, args) }
	tool.Register(cmd)
	tool.Register(bracketCmd)
}

// Exit statuses, per the GNU manual: the expression's truth value, or 2
// for a syntax/usage error.
const (
	statusTrue    = 0
	statusFalse   = 1
	statusSyntax  = 2
	statusPending = -1 // sentinel: no error yet
)

func run(rc *tool.RunContext, t *tool.Tool, args []string) int {
	if t == bracketCmd {
		// The long options are recognized only under the `[` spelling,
		// only when the whole argument list is exactly one operand, and
		// only BEFORE the closing bracket is considered. That ordering is
		// what makes `[ --help ]` a string test (true, no output) while
		// `[ --help` prints the help text.
		//
		// Matched literally, never by prefix: `[ --he` is a one-operand
		// string test, not an abbreviation of --help. This is why the
		// tool bypasses tool.Parse, whose GNU long-option abbreviation
		// would expand it.
		if len(args) == 1 {
			switch args[0] {
			case "--help":
				return help(rc, t)
			case "--version":
				return version(rc, t)
			}
		}
		// The closing bracket is part of the invocation, not of the
		// expression.
		if len(args) == 0 || args[len(args)-1] != "]" {
			fmt.Fprintf(rc.Err, "%s: missing %s\n", t.Name, quote("]"))
			return statusSyntax
		}
		args = args[:len(args)-1]
	}
	// `test` itself accepts no options whatsoever — the GNU manual is
	// explicit that it treats --help and --version like any other
	// non-empty STRING, so `test --help` is simply true and prints
	// nothing. `[ --help` is the spelling that documents both.

	p := &parser{rc: rc, name: t.Name, args: args}
	value, code := p.evaluate()
	if code != statusPending {
		return code
	}
	if value {
		return statusTrue
	}
	return statusFalse
}

func help(rc *tool.RunContext, t *tool.Tool) int {
	fmt.Fprintf(rc.Out, "Usage: %s\n%s\n", t.Usage, t.Synopsis)
	fmt.Fprint(rc.Out, `
Exit status is 0 if EXPRESSION is true, 1 if false, 2 if EXPRESSION is
malformed. An omitted EXPRESSION is false.

Expressions:
  ( EXPRESSION )           EXPRESSION is true
  ! EXPRESSION             EXPRESSION is false
  EXPRESSION1 -a EXPRESSION2   both EXPRESSION1 and EXPRESSION2 are true
  EXPRESSION1 -o EXPRESSION2   either EXPRESSION1 or EXPRESSION2 is true

  -n STRING                the length of STRING is non-zero
  STRING                   equivalent to -n STRING
  -z STRING                the length of STRING is zero
  STRING1 = STRING2        the strings are equal (== is a synonym)
  STRING1 != STRING2       the strings are not equal
  STRING1 < STRING2        STRING1 sorts before STRING2 (byte order)
  STRING1 > STRING2        STRING1 sorts after STRING2 (byte order)

  INTEGER1 -eq INTEGER2    the integers are equal
  INTEGER1 -ne INTEGER2    the integers are not equal
  INTEGER1 -gt INTEGER2    INTEGER1 is greater than INTEGER2
  INTEGER1 -ge INTEGER2    INTEGER1 is greater than or equal to INTEGER2
  INTEGER1 -lt INTEGER2    INTEGER1 is less than INTEGER2
  INTEGER1 -le INTEGER2    INTEGER1 is less than or equal to INTEGER2

  FILE1 -ef FILE2          same device and inode numbers
  FILE1 -nt FILE2          FILE1 is newer than FILE2, or FILE2 is missing
  FILE1 -ot FILE2          FILE1 is older than FILE2, or FILE1 is missing

  -e FILE                  FILE exists
  -a FILE                  same as -e (deprecated)
  -b FILE                  FILE exists and is a block special file
  -c FILE                  FILE exists and is a character special file
  -d FILE                  FILE exists and is a directory
  -f FILE                  FILE exists and is a regular file
  -g FILE                  FILE exists and is set-group-ID
  -G FILE                  FILE exists and is owned by the effective group ID
  -h FILE                  FILE exists and is a symbolic link (same as -L)
  -k FILE                  FILE exists and has its sticky bit set
  -L FILE                  FILE exists and is a symbolic link (same as -h)
  -N FILE                  FILE exists and has been modified since it was last read
  -O FILE                  FILE exists and is owned by the effective user ID
  -p FILE                  FILE exists and is a named pipe
  -r FILE                  FILE exists and read permission is granted
  -s FILE                  FILE exists and has a size greater than zero
  -S FILE                  FILE exists and is a socket
  -t FD                    file descriptor FD is opened on a terminal
  -u FILE                  FILE exists and its set-user-ID bit is set
  -w FILE                  FILE exists and write permission is granted
  -x FILE                  FILE exists and execute (or search) permission is granted

Options:
      --help     display this help and exit
      --version  output version information and exit

NOTE: only [ honors --help and --version, and only as its single
argument: '[ --help' prints this text, while '[ --help ]' is a string
test. 'test' has no options at all -- it treats --help and --version as
ordinary non-empty strings, so 'test --help' is simply true.
`)
	return statusTrue
}

func version(rc *tool.RunContext, t *tool.Tool) int {
	fmt.Fprintf(rc.Out, "%s (qiangli/coreutils) %s\n", t.Name, tool.Version)
	return statusTrue
}

// quote renders an operand for a diagnostic. Single quotes are the
// LC_ALL=C rendering GNU uses; the operand is never escaped further, so
// the diagnostic is a deterministic function of the operand.
func quote(s string) string { return "'" + s + "'" }

// syntaxError unwinds the recursive-descent evaluation. The grammar is
// mutually recursive over boolean-returning functions, and every error
// is terminal (exit 2), so threading an error return through each of
// them would only obscure the evaluation rules.
type syntaxError struct{ msg string }

type parser struct {
	rc   *tool.RunContext
	name string // the invoked name: "test" or "[", used in diagnostics
	args []string
	pos  int
}

func (p *parser) fail(format string, a ...any) {
	panic(syntaxError{msg: fmt.Sprintf(format, a...)})
}

// beyond reports an expression that ended while a primary still wanted
// an operand.
func (p *parser) beyond() {
	last := ""
	if len(p.args) > 0 {
		last = p.args[len(p.args)-1]
	}
	p.fail("missing argument after %s", quote(last))
}

func (p *parser) remaining() int { return len(p.args) - p.pos }

// at returns the operand i positions ahead of the cursor. Callers check
// remaining() first.
func (p *parser) at(i int) string { return p.args[p.pos+i] }

// next returns the operand at the cursor and advances past it.
func (p *parser) next() string {
	if p.pos >= len(p.args) {
		p.beyond()
	}
	s := p.args[p.pos]
	p.pos++
	return s
}

// evaluate runs the POSIX operand-count dispatch and returns the truth
// value, or (false, 2) after reporting a syntax error.
func (p *parser) evaluate() (result bool, code int) {
	code = statusPending
	defer func() {
		r := recover()
		if r == nil {
			return
		}
		se, ok := r.(syntaxError)
		if !ok {
			panic(r)
		}
		fmt.Fprintf(p.rc.Err, "%s: %s\n", p.name, se.msg)
		result, code = false, statusSyntax
	}()

	var value bool
	switch len(p.args) {
	case 0:
		// POSIX: no expression at all is false.
		return false, statusPending
	case 1:
		value = p.oneArgument()
	case 2:
		value = p.twoArguments()
	case 3:
		value = p.threeArguments()
	case 4:
		switch {
		case p.at(0) == "!":
			p.pos++
			value = !p.threeArguments()
		case p.at(0) == "(" && p.at(3) == ")":
			p.pos++
			value = p.twoArguments()
			p.pos++ // the ")"
		default:
			// Unspecified by POSIX at four operands; the documented
			// grammar takes over.
			value = p.expression()
		}
	default:
		value = p.expression()
	}
	if p.pos != len(p.args) {
		p.fail("extra argument %s", quote(p.args[p.pos]))
	}
	return value, statusPending
}

// oneArgument: a lone operand is true when it is not the empty string.
func (p *parser) oneArgument() bool { return p.next() != "" }

// twoArguments: `! STRING`, or a unary primary and its operand.
func (p *parser) twoArguments() bool {
	switch a := p.at(0); {
	case a == "!":
		p.pos++
		return !p.oneArgument()
	case isUnaryOp(a):
		return p.unaryOperator()
	default:
		p.fail("%s: unary operator expected", quote(a))
		return false // unreachable: fail panics
	}
}

// threeArguments: a binary primary, a negated two-operand expression,
// a parenthesized single operand, or an -a/-o join.
func (p *parser) threeArguments() bool {
	switch {
	case isBinaryOp(p.at(1)):
		return p.binaryOperator()
	case p.at(0) == "!":
		p.pos++
		return !p.twoArguments()
	case p.at(0) == "(" && p.at(2) == ")":
		p.pos++
		value := p.oneArgument()
		p.pos++ // the ")"
		return value
	case p.at(1) == "-a" || p.at(1) == "-o":
		return p.expression()
	default:
		p.fail("%s: binary operator expected", quote(p.at(1)))
		return false // unreachable: fail panics
	}
}

// expression is the grammar for the operand counts POSIX leaves
// unspecified: or → and → term.
func (p *parser) expression() bool {
	if p.pos >= len(p.args) {
		p.beyond()
	}
	return p.or()
}

func (p *parser) or() bool {
	value := false
	for {
		// Deliberately not short-circuiting: every branch is evaluated
		// so that a malformed one is still reported (exit 2 beats a
		// true answer produced by skipping over nonsense).
		if p.and() {
			value = true
		}
		if p.pos < len(p.args) && p.args[p.pos] == "-o" {
			p.pos++
			continue
		}
		return value
	}
}

func (p *parser) and() bool {
	value := true
	for {
		if !p.term() {
			value = false
		}
		if p.pos < len(p.args) && p.args[p.pos] == "-a" {
			p.pos++
			continue
		}
		return value
	}
}

func (p *parser) term() bool {
	if p.pos >= len(p.args) {
		p.beyond()
	}
	if p.args[p.pos] == "!" {
		// A run of negations folds into its parity.
		negate := false
		for p.pos < len(p.args) && p.args[p.pos] == "!" {
			p.pos++
			negate = !negate
		}
		return negate != p.term()
	}
	if p.args[p.pos] == "(" {
		p.pos++
		value := p.expression()
		if p.pos >= len(p.args) || p.args[p.pos] != ")" {
			p.fail("%s expected", quote(")"))
		}
		p.pos++
		return value
	}
	if p.remaining() >= 3 && isBinaryOp(p.at(1)) {
		return p.binaryOperator()
	}
	if a := p.at(0); len(a) == 2 && a[0] == '-' {
		if isUnaryOp(a) {
			return p.unaryOperator()
		}
		p.fail("%s: unary operator expected", quote(a))
	}
	return p.next() != ""
}

// unaryOps are the single-letter primaries documented in the GNU manual.
// `-o` is absent on purpose (see the package comment).
const unaryOps = "abcdefghknprstuwxzGLNOS"

func isUnaryOp(s string) bool {
	return len(s) == 2 && s[0] == '-' && strings.IndexByte(unaryOps, s[1]) >= 0
}

func isBinaryOp(s string) bool {
	switch s {
	case "=", "==", "!=", "<", ">",
		"-eq", "-ne", "-lt", "-le", "-gt", "-ge",
		"-ef", "-nt", "-ot":
		return true
	}
	return false
}

func (p *parser) unaryOperator() bool {
	op := p.next()[1]
	operand := p.next()
	switch op {
	case 'n':
		return operand != ""
	case 'z':
		return operand == ""
	case 't':
		return p.terminalTest(operand)
	default:
		return p.fileTest(op, operand)
	}
}

func (p *parser) binaryOperator() bool {
	left := p.next()
	op := p.next()
	right := p.next()
	switch op {
	case "=", "==":
		return left == right
	case "!=":
		return left != right
	case "<":
		return left < right
	case ">":
		return left > right
	case "-eq", "-ne", "-lt", "-le", "-gt", "-ge":
		c := p.integer(left).Cmp(p.integer(right))
		switch op {
		case "-eq":
			return c == 0
		case "-ne":
			return c != 0
		case "-lt":
			return c < 0
		case "-le":
			return c <= 0
		case "-gt":
			return c > 0
		default: // -ge
			return c >= 0
		}
	case "-ef":
		li, lerr := statOperand(p.rc, left)
		ri, rerr := statOperand(p.rc, right)
		return lerr == nil && rerr == nil && os.SameFile(li, ri)
	case "-nt", "-ot":
		li, lerr := statOperand(p.rc, left)
		ri, rerr := statOperand(p.rc, right)
		if op == "-nt" {
			// True when FILE1 is newer, or when FILE1 exists and FILE2
			// does not.
			return lerr == nil && (rerr != nil || li.ModTime().After(ri.ModTime()))
		}
		return rerr == nil && (lerr != nil || li.ModTime().Before(ri.ModTime()))
	}
	// isBinaryOp gates every caller, so this is a programming error.
	p.fail("%s: binary operator expected", quote(op))
	return false
}

// integer parses a numeric operand: optional surrounding blanks, an
// optional sign, then decimal digits.
func (p *parser) integer(s string) *big.Int {
	body := strings.Trim(s, " \t")
	digits := body
	if strings.HasPrefix(digits, "+") || strings.HasPrefix(digits, "-") {
		digits = digits[1:]
	}
	if digits == "" {
		p.fail("invalid integer %s", quote(s))
	}
	for i := 0; i < len(digits); i++ {
		if digits[i] < '0' || digits[i] > '9' {
			p.fail("invalid integer %s", quote(s))
		}
	}
	n, ok := new(big.Int).SetString(body, 10)
	if !ok {
		p.fail("invalid integer %s", quote(s))
	}
	return n
}

// terminalTest implements -t FD. Standard streams come from RunContext;
// larger descriptors are inherited process descriptors and are probed
// without taking ownership of (or closing) them.
func (p *parser) terminalTest(operand string) bool {
	fd := p.integer(operand)
	if !fd.IsInt64() || fd.Sign() < 0 {
		return false
	}
	n := fd.Int64()
	var stream any
	switch n {
	case 0:
		stream = p.rc.In
	case 1:
		stream = p.rc.Out
	case 2:
		stream = p.rc.Err
	default:
		return isTerminalDescriptor(n)
	}
	f, ok := stream.(*os.File)
	if !ok || f == nil {
		return false
	}
	return isTerminal(f)
}

// fileTest applies a single-letter file primary to operand, resolved
// against the invocation working directory.
func (p *parser) fileTest(op byte, operand string) bool {
	switch op {
	case 'h', 'L':
		fi, err := lstatOperand(p.rc, operand)
		return err == nil && fi.Mode()&os.ModeSymlink != 0
	case 'r', 'w', 'x':
		ok, err := accessOK(p.rc, operand, op)
		if err != nil {
			p.fail("-%c: %v", op, err)
		}
		return ok
	case 'O', 'G':
		owned, err := ownedByEffective(p.rc, operand, op == 'G')
		if err != nil {
			p.fail("-%c: %v", op, err)
		}
		return owned
	case 'N':
		atime, mtime, err := fileTimes(p.rc, operand)
		if err != nil {
			if os.IsNotExist(err) {
				return false
			}
			p.fail("-N: %v", err)
		}
		return mtime.After(atime)
	}

	// The remaining primaries are pure stat questions; every one of them
	// is false when the file does not exist.
	fi, err := statOperand(p.rc, operand)
	if err != nil {
		return false
	}
	mode := fi.Mode()
	switch op {
	case 'a', 'e':
		return true
	case 'f':
		return mode.IsRegular()
	case 'd':
		return fi.IsDir()
	case 's':
		return fi.Size() > 0
	case 'b':
		return mode&os.ModeDevice != 0 && mode&os.ModeCharDevice == 0
	case 'c':
		return mode&os.ModeCharDevice != 0
	case 'p':
		return mode&os.ModeNamedPipe != 0
	case 'S':
		return mode&os.ModeSocket != 0
	case 'u':
		return mode&os.ModeSetuid != 0
	case 'g':
		return mode&os.ModeSetgid != 0
	case 'k':
		return mode&os.ModeSticky != 0
	}
	// unaryOps gates every caller, so this is a programming error.
	p.fail("-%c: unary operator expected", op)
	return false
}

// openOperandDir opens rc.Dir for a directory-relative retry. It exists
// because rc.Path joins a relative operand onto rc.Dir into one
// materialized absolute string: a working directory that is itself long
// but individually valid (every component was created and entered one
// step at a time, exactly as a shell's cd does) can, once a further
// operand is appended, produce a string that exceeds the platform's
// path-length limit even though the file is perfectly reachable — a
// plain relative lookup from the already-open directory never needs
// that string at all. Only a relative operand can benefit: an absolute
// operand names an exact location with nothing to resolve against
// rc.Dir.
func openOperandDir(rc *tool.RunContext, operand string) (*os.File, bool) {
	if !canRetryAgainstDir(rc, operand) {
		return nil, false
	}
	f, err := os.Open(rc.Dir)
	if err != nil {
		return nil, false
	}
	return f, true
}

// canRetryAgainstDir reports whether operand is a relative name that a
// directory-handle retry could help with — see openOperandDir.
func canRetryAgainstDir(rc *tool.RunContext, operand string) bool {
	return rc.Dir != "" && !filepath.IsAbs(operand)
}

// statOperand resolves operand the way rc.Path + os.Stat always have,
// then — only on failure, and only for a relative operand — retries via
// os.Root, which resolves each name against an already-open directory
// handle instead of a materialized absolute string. See
// openOperandDir for why the plain join can fail where GNU's test would
// not.
func statOperand(rc *tool.RunContext, operand string) (os.FileInfo, error) {
	fi, err := os.Stat(rc.Path(operand))
	if err == nil || !canRetryAgainstDir(rc, operand) {
		return fi, err
	}
	if root, rerr := os.OpenRoot(rc.Dir); rerr == nil {
		defer root.Close()
		if fi2, err2 := root.Stat(operand); err2 == nil {
			return fi2, nil
		}
	}
	return fi, err
}

// lstatOperand is statOperand for the symlink-preserving primaries (-h/-L).
func lstatOperand(rc *tool.RunContext, operand string) (os.FileInfo, error) {
	fi, err := os.Lstat(rc.Path(operand))
	if err == nil || !canRetryAgainstDir(rc, operand) {
		return fi, err
	}
	if root, rerr := os.OpenRoot(rc.Dir); rerr == nil {
		defer root.Close()
		if fi2, err2 := root.Lstat(operand); err2 == nil {
			return fi2, nil
		}
	}
	return fi, err
}
