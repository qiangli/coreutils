package terminfo

import (
	"fmt"
	"strconv"
	"strings"
)

// This file implements the `%`-directive language a parameterized terminfo
// capability is written in — a small stack machine, not a printf dialect,
// though it borrows printf's conversion syntax for output.
//
// The one genuinely ambiguous piece of the grammar is `%+`: it is both the
// addition operator and, read as printf, the "always show a sign" flag. The
// grammar resolves it positionally — a conversion specification may only begin
// with `:`, `#`, a space, a digit or `.`, so a `-` or `+` immediately after
// `%` is always an operator. formatIntroducer below is that rule, and it is
// the reason `%p1%p2%+%c` and `%:+d` both work.

// value is one stack cell: terminfo values are either integers or strings.
type value struct {
	num int
	str string
	// isStr distinguishes an empty string from the integer 0; the arithmetic
	// operators need to know which they were handed.
	isStr bool
}

func numValue(n int) value    { return value{num: n} }
func strValue(s string) value { return value{str: s, isStr: true} }

func (v value) asInt() int {
	if !v.isStr {
		return v.num
	}
	n, err := strconv.Atoi(v.str)
	if err != nil {
		return 0
	}
	return n
}

func (v value) asString() string {
	if v.isStr {
		return v.str
	}
	return strconv.Itoa(v.num)
}

type tparmState struct {
	out    strings.Builder
	stack  []value
	params [9]value
	dyn    [26]int // %Pa..%Pz
	stat   [26]int // %PA..%PZ
}

func (st *tparmState) push(v value) { st.stack = append(st.stack, v) }

// pop returns the top of stack, or a zero integer when the stack is empty.
// An underflow means the capability string is malformed; the reference
// behaviour is to keep going with a zero rather than abort, and a hard error
// here would break terminals whose entries have long-tolerated sloppiness.
func (st *tparmState) pop() value {
	if len(st.stack) == 0 {
		return numValue(0)
	}
	v := st.stack[len(st.stack)-1]
	st.stack = st.stack[:len(st.stack)-1]
	return v
}

func (st *tparmState) popInt() int { return st.pop().asInt() }

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

// formatIntroducer reports whether c can start a printf-style conversion
// specification. See the note at the top of the file.
func formatIntroducer(c byte) bool {
	switch {
	case c == ':' || c == '#' || c == ' ' || c == '.':
		return true
	case c >= '0' && c <= '9':
		return true
	}
	return false
}

func isConversion(c byte) bool {
	switch c {
	case 'd', 'o', 'x', 'X', 's', 'c':
		return true
	}
	return false
}

// tparm instantiates a parameterized capability with params.
//
// It returns an error only for input the grammar cannot describe at all (an
// unterminated `%'`, `%{`, or an unrecognised directive). Silent tolerance is
// what makes a mistyped capability look like it worked.
func tparm(s string, params []value) (string, error) {
	st := &tparmState{}
	copy(st.params[:], params)

	for i := 0; i < len(s); {
		c := s[i]
		if c != '%' {
			st.out.WriteByte(c)
			i++
			continue
		}
		i++
		if i >= len(s) {
			return "", fmt.Errorf("trailing %% at end of capability string")
		}
		next, err := st.directive(s, i)
		if err != nil {
			return "", err
		}
		i = next
	}
	return st.out.String(), nil
}

// directive executes the directive whose body starts at i (just past the '%')
// and returns the index of the next unconsumed byte.
func (st *tparmState) directive(s string, i int) (int, error) {
	c := s[i]

	if isConversion(c) {
		return st.conversion(s, i, i, c)
	}
	if formatIntroducer(c) {
		return st.formatted(s, i)
	}

	i++
	switch c {
	case '%':
		st.out.WriteByte('%')

	case 'p':
		if i >= len(s) || s[i] < '1' || s[i] > '9' {
			return 0, fmt.Errorf("%%p must be followed by a digit 1-9")
		}
		st.push(st.params[s[i]-'1'])
		i++

	case 'P':
		if i >= len(s) {
			return 0, fmt.Errorf("%%P must be followed by a variable name")
		}
		v := st.popInt()
		switch n := s[i]; {
		case n >= 'a' && n <= 'z':
			st.dyn[n-'a'] = v
		case n >= 'A' && n <= 'Z':
			st.stat[n-'A'] = v
		default:
			return 0, fmt.Errorf("%%P%c: variable name must be a letter", n)
		}
		i++

	case 'g':
		if i >= len(s) {
			return 0, fmt.Errorf("%%g must be followed by a variable name")
		}
		switch n := s[i]; {
		case n >= 'a' && n <= 'z':
			st.push(numValue(st.dyn[n-'a']))
		case n >= 'A' && n <= 'Z':
			st.push(numValue(st.stat[n-'A']))
		default:
			return 0, fmt.Errorf("%%g%c: variable name must be a letter", n)
		}
		i++

	case '\'':
		// %'c' pushes a character constant. The quoted character may itself be
		// a quote in principle; the format has no escape for it, so a single
		// byte is taken literally and the closing quote must follow.
		if i+1 >= len(s) || s[i+1] != '\'' {
			return 0, fmt.Errorf("unterminated %%'c' character constant")
		}
		st.push(numValue(int(s[i])))
		i += 2

	case '{':
		end := strings.IndexByte(s[i:], '}')
		if end < 0 {
			return 0, fmt.Errorf("unterminated %%{...} integer constant")
		}
		lit := s[i : i+end]
		n, err := strconv.Atoi(lit)
		if err != nil {
			return 0, fmt.Errorf("%%{%s}: not an integer", lit)
		}
		st.push(numValue(n))
		i += end + 1

	case 'l':
		st.push(numValue(len(st.pop().asString())))

	case '+', '-', '*', '/', 'm', '&', '|', '^', '=', '>', '<', 'A', 'O':
		b := st.popInt()
		a := st.popInt()
		st.push(numValue(binaryOp(c, a, b)))

	case '!':
		st.push(numValue(boolInt(st.popInt() == 0)))

	case '~':
		st.push(numValue(^st.popInt()))

	case 'i':
		// Increments the first two parameters. Only integers move: a string
		// parameter has no successor, and coercing it would corrupt it.
		for k := 0; k < 2; k++ {
			if !st.params[k].isStr {
				st.params[k].num++
			}
		}

	case '?', ';':
		// Structural markers only. The work happens at %t and %e.

	case 't':
		if st.popInt() == 0 {
			return skipBranch(s, i, true)
		}

	case 'e':
		// Reached only by falling out of a taken then-branch, so the whole
		// remaining chain of else-if arms is skipped.
		return skipBranch(s, i, false)

	default:
		return 0, fmt.Errorf("unsupported terminfo directive %%%c", c)
	}
	return i, nil
}

// binaryOp applies the two-operand operators. Division and modulo by zero
// yield zero rather than trapping: a capability string is data read from a
// file, and a crash is never the right response to bad data.
func binaryOp(op byte, a, b int) int {
	switch op {
	case '+':
		return a + b
	case '-':
		return a - b
	case '*':
		return a * b
	case '/':
		if b == 0 {
			return 0
		}
		return a / b
	case 'm':
		if b == 0 {
			return 0
		}
		return a % b
	case '&':
		return a & b
	case '|':
		return a | b
	case '^':
		return a ^ b
	case '=':
		return boolInt(a == b)
	case '>':
		return boolInt(a > b)
	case '<':
		return boolInt(a < b)
	case 'A':
		return boolInt(a != 0 && b != 0)
	case 'O':
		return boolInt(a != 0 || b != 0)
	}
	return 0
}

// formatted handles a conversion specification with flags, width or precision.
func (st *tparmState) formatted(s string, start int) (int, error) {
	i := start
	if s[i] == ':' {
		// The ':' exists solely to let a '-' or '+' be read as a flag instead
		// of as the subtraction/addition operator. It is not part of the spec.
		i++
	}
	for i < len(s) && strings.IndexByte("-+# 0", s[i]) >= 0 {
		i++
	}
	for i < len(s) && s[i] >= '0' && s[i] <= '9' {
		i++
	}
	if i < len(s) && s[i] == '.' {
		i++
		for i < len(s) && s[i] >= '0' && s[i] <= '9' {
			i++
		}
	}
	if i >= len(s) || !isConversion(s[i]) {
		return 0, fmt.Errorf("malformed conversion specification %%%s", s[start:min(len(s), start+8)])
	}
	specStart := start
	if s[specStart] == ':' {
		specStart++
	}
	return st.conversion(s, specStart, i, s[i])
}

// conversion emits one value. specStart..convPos is the flags/width/precision
// run (possibly empty) and conv is the conversion character.
func (st *tparmState) conversion(s string, specStart, convPos int, conv byte) (int, error) {
	spec := "%" + s[specStart:convPos] + string(conv)
	switch conv {
	case 'c':
		// A raw byte, not a rune: values above 127 name a single 8-bit
		// character in the terminal's own encoding, and UTF-8 encoding them
		// would emit two bytes the terminal never asked for. Width and flags
		// are therefore not applied to %c.
		st.out.WriteByte(byte(st.popInt()))
	case 's':
		fmt.Fprintf(&st.out, spec, st.pop().asString())
	default: // d, o, x, X
		fmt.Fprintf(&st.out, spec, st.popInt())
	}
	return convPos + 1, nil
}

// skipBranch advances past the branch starting at i.
//
// When stopAtElse is true (a false %t) it stops after the matching %e or %;,
// so the else-arm runs. When false (falling out of a taken then-branch at %e)
// it stops only after the matching %;, discarding every remaining else-if arm.
// Only %? opens a new nesting level — %t and %e do not — which is what makes
// the flat `%?c1%tA%ec2%tB%eC%;` else-if chain work.
func skipBranch(s string, i int, stopAtElse bool) (int, error) {
	depth := 0
	for i < len(s) {
		if s[i] != '%' {
			i++
			continue
		}
		i++
		if i >= len(s) {
			break
		}
		c := s[i]
		i++
		switch c {
		case '?':
			depth++
		case ';':
			if depth == 0 {
				return i, nil
			}
			depth--
		case 'e':
			if depth == 0 && stopAtElse {
				return i, nil
			}
		case '\'':
			// Skip the quoted character and its closing quote so a literal
			// '%' or ';' inside it cannot be mistaken for a directive.
			i += 2
		case '{':
			if end := strings.IndexByte(s[i:], '}'); end >= 0 {
				i += end + 1
			}
		case 'p', 'P', 'g':
			i++ // one-character operand
		}
	}
	return 0, fmt.Errorf("unterminated %%? conditional")
}

// --- capability instantiation ----------------------------------------------

// Instantiate applies the operands to a capability string and strips padding.
//
// With NO operands the string is emitted verbatim rather than run through the
// parameter engine. That is the reference behaviour and it is deliberate:
// `tput cup` with no arguments prints the uninstantiated `\E[%i%p1%d;%p2%dH`,
// which is how a script asks for the raw template. Running the engine on it
// instead would silently substitute zeros and print `\E[1;1H` — a valid-looking
// escape sequence that is not what was asked for.
func Instantiate(s string, parms []string) (string, error) {
	if len(parms) == 0 {
		return StripPadding(s), nil
	}
	out, err := tparm(s, parseParams(parms))
	if err != nil {
		return "", err
	}
	return StripPadding(out), nil
}

// parseParams converts command-line operands to stack values. An operand that
// reads as a decimal integer is passed as a number, because that is what the
// arithmetic directives in a capability like `cup` expect; everything else is
// passed as a string, which is what capabilities like `pfkey` expect.
func parseParams(parms []string) []value {
	if len(parms) > 9 {
		parms = parms[:9]
	}
	out := make([]value, len(parms))
	for i, p := range parms {
		if n, err := strconv.Atoi(p); err == nil {
			out[i] = numValue(n)
			continue
		}
		out[i] = strValue(p)
	}
	return out
}

// StripPadding removes `$<...>` delay specifications.
//
// A delay is an instruction to the OUTPUT DRIVER — wait this many
// milliseconds, or send this many pad characters at the current line speed —
// and it is meaningful only when writing to a real terminal at a known baud
// rate. tput's output is overwhelmingly captured into a shell variable, where
// emitting the literal text "$<5>" would corrupt it and where sleeping would
// just make the script slower. Modern terminals declare no delays at all, so
// this is invisible for them; for the ones that do, dropping the delay is the
// behaviour a script sees from the reference implementation writing to a pipe.
//
// Text that merely looks like a delay is left alone: the closing '>' and a
// well-formed body are both required.
func StripPadding(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); {
		if s[i] != '$' || i+1 >= len(s) || s[i+1] != '<' {
			b.WriteByte(s[i])
			i++
			continue
		}
		end := strings.IndexByte(s[i+2:], '>')
		if end < 0 || !isDelayBody(s[i+2:i+2+end]) {
			b.WriteByte(s[i])
			i++
			continue
		}
		i += 2 + end + 1
	}
	return b.String()
}

// isDelayBody reports whether body is a delay: a decimal number with at most
// one fractional digit, optionally followed by '*' (per affected line) and/or
// '/' (mandatory even with xon/xoff), in either order.
func isDelayBody(body string) bool {
	i := 0
	digits := 0
	for i < len(body) && body[i] >= '0' && body[i] <= '9' {
		i++
		digits++
	}
	if digits == 0 {
		return false
	}
	if i < len(body) && body[i] == '.' {
		i++
		for i < len(body) && body[i] >= '0' && body[i] <= '9' {
			i++
		}
	}
	for i < len(body) && (body[i] == '*' || body[i] == '/') {
		i++
	}
	return i == len(body)
}
