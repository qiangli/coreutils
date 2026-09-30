package m4cmd

// evalExpr evaluates the integer expression of the eval built-in:
// decimal, octal (leading 0) and hexadecimal (leading 0x) constants,
// parentheses, and the C operators POSIX lists — unary + - ~ !, then
// * / %, + -, << >>, < <= > >=, == !=, &, ^, |, && and || in decreasing
// precedence, the last two short-circuiting. Arithmetic is 32-bit signed
// and wraps. A non-empty errmsg means the expression has no value.
func evalExpr(s string) (v int32, errmsg string) {
	if len(s) == 0 {
		return 0, ""
	}
	e := &evaluator{s: s}
	v = e.binary(0, true)
	if e.err == "" {
		if e.space(); e.pos < len(e.s) {
			e.err = "bad expression"
		}
	}
	return v, e.err
}

type evaluator struct {
	s   string
	pos int
	err string
}

func (e *evaluator) fail(msg string) {
	if e.err == "" {
		e.err = msg
	}
}

func (e *evaluator) space() {
	for e.pos < len(e.s) && isSpace(e.s[e.pos]) {
		e.pos++
	}
}

// binaryOps lists the binary operators by precedence level, loosest
// first. Within a level a longer spelling precedes its prefix.
var binaryOps = [][]string{
	{"||"},
	{"&&"},
	{"|"},
	{"^"},
	{"&"},
	{"==", "!="},
	{"<=", ">=", "<", ">"},
	{"<<", ">>"},
	{"+", "-"},
	{"*", "/", "%"},
}

// operator consumes and returns an operator of the given level if one is
// next. A match must not be the prefix of a longer operator that belongs
// to another level ("<" of "<<" or "<=", "&" of "&&", "|" of "||").
func (e *evaluator) operator(level int) string {
	e.space()
	rest := e.s[e.pos:]
	for _, op := range binaryOps[level] {
		if len(rest) < len(op) || rest[:len(op)] != op {
			continue
		}
		if len(op) == 1 && len(rest) > 1 {
			switch two := rest[:2]; two {
			case "||", "&&", "<<", ">>", "<=", ">=", "==", "!=":
				return ""
			}
		}
		e.pos += len(op)
		return op
	}
	return ""
}

// binary parses one precedence level. live is false inside the
// unevaluated operand of a short-circuited && or ||: the operand is
// still parsed, but a division by zero there is not an error.
func (e *evaluator) binary(level int, live bool) int32 {
	if level == len(binaryOps) {
		return e.unary(live)
	}
	l := e.binary(level+1, live)
	for e.err == "" {
		op := e.operator(level)
		if op == "" {
			break
		}
		switch op {
		case "||":
			r := e.binary(level+1, live && l == 0)
			l = truth(l != 0 || r != 0)
			continue
		case "&&":
			r := e.binary(level+1, live && l != 0)
			l = truth(l != 0 && r != 0)
			continue
		}
		r := e.binary(level+1, live)
		if e.err != "" {
			break
		}
		switch op {
		case "|":
			l |= r
		case "^":
			l ^= r
		case "&":
			l &= r
		case "==":
			l = truth(l == r)
		case "!=":
			l = truth(l != r)
		case "<":
			l = truth(l < r)
		case "<=":
			l = truth(l <= r)
		case ">":
			l = truth(l > r)
		case ">=":
			l = truth(l >= r)
		case "<<":
			l = int32(uint32(l) << (uint32(r) & 31))
		case ">>":
			l >>= uint32(r) & 31
		case "+":
			l += r
		case "-":
			l -= r
		case "*":
			l *= r
		case "/", "%":
			switch {
			case r != 0 && op == "/":
				l /= r
			case r != 0:
				l %= r
			case live && op == "/":
				e.fail("divide by zero")
			case live:
				e.fail("modulo by zero")
			}
		}
	}
	return l
}

func truth(b bool) int32 {
	if b {
		return 1
	}
	return 0
}

func (e *evaluator) unary(live bool) int32 {
	e.space()
	if e.pos == len(e.s) {
		e.fail("bad expression")
		return 0
	}
	switch c := e.s[e.pos]; c {
	case '+':
		e.pos++
		return e.unary(live)
	case '-':
		e.pos++
		return -e.unary(live)
	case '~':
		e.pos++
		return ^e.unary(live)
	case '!':
		e.pos++
		return truth(e.unary(live) == 0)
	case '(':
		e.pos++
		v := e.binary(0, live)
		if e.space(); e.pos == len(e.s) || e.s[e.pos] != ')' {
			e.fail("bad expression")
			return 0
		}
		e.pos++
		return v
	}
	return e.constant()
}

func (e *evaluator) constant() int32 {
	base := uint32(10)
	start := e.pos
	if e.s[e.pos] == '0' {
		base = 8
		if e.pos+1 < len(e.s) && (e.s[e.pos+1] == 'x' || e.s[e.pos+1] == 'X') {
			base = 16
			e.pos += 2
			start = e.pos
		}
	}
	var v uint32
	for ; e.pos < len(e.s); e.pos++ {
		c := e.s[e.pos]
		var d uint32
		switch {
		case c >= '0' && c <= '9':
			d = uint32(c - '0')
		case c >= 'a' && c <= 'z':
			d = uint32(c-'a') + 10
		case c >= 'A' && c <= 'Z':
			d = uint32(c-'A') + 10
		default:
			d = 99
		}
		if d == 99 {
			break
		}
		if d >= base {
			e.fail("bad expression")
			return 0
		}
		v = v*base + d
	}
	if e.pos == start {
		e.fail("bad expression")
		return 0
	}
	return int32(v)
}
