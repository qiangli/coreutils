package exprcmd

import (
	"fmt"
	"math/big"
	"strconv"
	"unicode/utf8"

	"github.com/qiangli/coreutils/pkg/bre"
	"github.com/qiangli/coreutils/tool"
)

var cmd = &tool.Tool{
	Name:     "expr",
	Synopsis: "Evaluate expressions.",
	Usage:    "expr EXPRESSION",
}

func init() { cmd.Run = run; tool.Register(cmd) }

type value string

func run(rc *tool.RunContext, args []string) int {
	if len(args) == 1 && (args[0] == "--help" || args[0] == "-h") {
		fmt.Fprintf(rc.Out, "Usage: %s\n%s\n\nOptions:\n      --help     display this help and exit\n      --version  output version information and exit\n", cmd.Usage, cmd.Synopsis)
		return 0
	}
	if len(args) == 1 && (args[0] == "--version" || args[0] == "-V") {
		fmt.Fprintf(rc.Out, "%s (qiangli/coreutils) %s\n", cmd.Name, tool.Version)
		return 0
	}
	if len(args) > 0 && args[0] == "--" {
		args = args[1:]
	}
	if len(args) == 0 {
		return tool.UsageError(rc, cmd, "missing operand")
	}
	p := &parser{tokens: args}
	v, err := p.parseOr()
	if err == nil && p.more() {
		err = fmt.Errorf("syntax error")
	}
	if err != nil {
		fmt.Fprintf(rc.Err, "expr: %v\n", err)
		return 2
	}
	fmt.Fprintln(rc.Out, string(v))
	if truthy(v) {
		return 0
	}
	return 1
}

type parser struct {
	tokens []string
	pos    int
}

func (p *parser) more() bool { return p.pos < len(p.tokens) }
func (p *parser) peek() string {
	if p.more() {
		return p.tokens[p.pos]
	}
	return ""
}
func (p *parser) next() string {
	s := p.peek()
	p.pos++
	return s
}

func (p *parser) parseOr() (value, error) {
	left, err := p.parseAnd()
	if err != nil {
		return "", err
	}
	for p.peek() == "|" {
		p.next()
		right, err := p.parseAnd()
		if err != nil {
			return "", err
		}
		if !truthy(left) {
			if right == "" {
				left = "0"
			} else {
				left = right
			}
		}
	}
	return left, nil
}

func (p *parser) parseAnd() (value, error) {
	left, err := p.parseCompare()
	if err != nil {
		return "", err
	}
	for p.peek() == "&" {
		p.next()
		right, err := p.parseCompare()
		if err != nil {
			return "", err
		}
		if truthy(left) && truthy(right) {
			// POSIX &: return expr1 when both operands are true.
		} else {
			left = "0"
		}
	}
	return left, nil
}

func (p *parser) parseCompare() (value, error) {
	left, err := p.parseAdd()
	if err != nil {
		return "", err
	}
	for {
		op := p.peek()
		if op != "=" && op != "==" && op != "!=" && op != "<" && op != "<=" && op != ">" && op != ">=" {
			return left, nil
		}
		p.next()
		right, err := p.parseAdd()
		if err != nil {
			return "", err
		}
		cmp := compare(left, right)
		ok := map[string]bool{"=": cmp == 0, "==": cmp == 0, "!=": cmp != 0, "<": cmp < 0, "<=": cmp <= 0, ">": cmp > 0, ">=": cmp >= 0}[op]
		if ok {
			left = "1"
		} else {
			left = "0"
		}
	}
}

func (p *parser) parseAdd() (value, error) {
	left, err := p.parseMul()
	if err != nil {
		return "", err
	}
	for p.peek() == "+" || p.peek() == "-" {
		op := p.next()
		right, err := p.parseMul()
		if err != nil {
			return "", err
		}
		a, b, err := integers(left, right)
		if err != nil {
			return "", err
		}
		if op == "+" {
			left = value(new(big.Int).Add(a, b).String())
		} else {
			left = value(new(big.Int).Sub(a, b).String())
		}
	}
	return left, nil
}

func (p *parser) parseMul() (value, error) {
	left, err := p.parseMatch()
	if err != nil {
		return "", err
	}
	for p.peek() == "*" || p.peek() == "/" || p.peek() == "%" {
		op := p.next()
		right, err := p.parseMatch()
		if err != nil {
			return "", err
		}
		a, b, err := integers(left, right)
		if err != nil {
			return "", err
		}
		switch op {
		case "*":
			left = value(new(big.Int).Mul(a, b).String())
		case "/":
			if b.Sign() == 0 {
				return "", fmt.Errorf("division by zero")
			}
			left = value(new(big.Int).Quo(a, b).String())
		case "%":
			if b.Sign() == 0 {
				return "", fmt.Errorf("division by zero")
			}
			left = value(new(big.Int).Rem(a, b).String())
		}
	}
	return left, nil
}

func (p *parser) parseMatch() (value, error) {
	left, err := p.parsePrimary()
	if err != nil {
		return "", err
	}
	for p.peek() == ":" {
		p.next()
		pat, err := p.parsePrimary()
		if err != nil {
			return "", err
		}
		re, captures, err := compileBRE(string(pat))
		if err != nil {
			return "", fmt.Errorf("invalid regular expression")
		}
		matches := re.FindAllStringSubmatchIndex(string(left), 1)
		if len(matches) == 0 || matches[0][0] != 0 {
			if captures {
				left = ""
			} else {
				left = "0"
			}
		} else if captures {
			m := matches[0]
			if len(m) > 3 && m[2] >= 0 {
				left = value(string(left)[m[2]:m[3]])
			} else {
				left = ""
			}
		} else {
			m := matches[0]
			left = value(strconv.Itoa(utf8.RuneCountInString(string(left)[m[0]:m[1]])))
		}
	}
	return left, nil
}

func (p *parser) parsePrimary() (value, error) {
	if !p.more() {
		return "", fmt.Errorf("missing operand")
	}
	t := p.next()
	if t == "+" && p.more() {
		// GNU's leading-+ guard forces an operator-looking token to be an
		// ordinary string operand: `expr + length` prints "length".
		return value(p.next()), nil
	}
	if t == "(" {
		v, err := p.parseOr()
		if err != nil {
			return "", err
		}
		if p.next() != ")" {
			return "", fmt.Errorf("unmatched opening parenthesis")
		}
		return v, nil
	}
	if t == ")" {
		return "", fmt.Errorf("unmatched closing parenthesis")
	}
	if t == "length" || t == "quote" || t == "index" || t == "substr" || t == "match" {
		return p.parseFunction(t)
	}
	return value(t), nil
}

func (p *parser) parseFunction(name string) (value, error) {
	arg := func() (value, error) {
		if !p.more() {
			return "", fmt.Errorf("missing argument after %s", name)
		}
		return p.parsePrimary()
	}
	switch name {
	case "length":
		v, err := arg()
		return value(strconv.Itoa(len([]rune(v)))), err
	case "quote":
		return arg()
	case "index":
		s, err := arg()
		if err != nil {
			return "", err
		}
		chars, err := arg()
		if err != nil {
			return "", err
		}
		for i, r := range []rune(s) {
			for _, c := range []rune(chars) {
				if r == c {
					return value(strconv.Itoa(i + 1)), nil
				}
			}
		}
		return "0", nil
	case "substr":
		s, err := arg()
		if err != nil {
			return "", err
		}
		posv, err := arg()
		if err != nil {
			return "", err
		}
		lenv, err := arg()
		if err != nil {
			return "", err
		}
		pos, ln, err := integers(posv, lenv)
		if err != nil {
			return "", err
		}
		rs := []rune(s)
		if !pos.IsInt64() || !ln.IsInt64() || pos.Sign() <= 0 || ln.Sign() <= 0 {
			return "", nil
		}
		start := pos.Int64() - 1
		if start >= int64(len(rs)) {
			return "", nil
		}
		end := int64(len(rs))
		if ln.Int64() < end-start {
			end = start + ln.Int64()
		}
		return value(string(rs[int(start):int(end)])), nil
	case "match":
		s, err := arg()
		if err != nil {
			return "", err
		}
		pat, err := arg()
		if err != nil {
			return "", err
		}
		sub := &parser{tokens: []string{string(s), ":", string(pat)}}
		return sub.parseMatch()
	default:
		return "", fmt.Errorf("unknown function")
	}
}

func integers(a, b value) (*big.Int, *big.Int, error) {
	ai, ok := integer(a)
	if !ok {
		return nil, nil, fmt.Errorf("non-integer argument")
	}
	bi, ok := integer(b)
	if !ok {
		return nil, nil, fmt.Errorf("non-integer argument")
	}
	return ai, bi, nil
}

func integer(v value) (*big.Int, bool) {
	s := string(v)
	if s == "" {
		return nil, false
	}
	i := 0
	if s[0] == '-' {
		i = 1
	}
	if i == len(s) {
		return nil, false
	}
	for ; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return nil, false
		}
	}
	n, ok := new(big.Int).SetString(s, 10)
	return n, ok
}

func compare(a, b value) int {
	ai, aok := integer(a)
	bi, bok := integer(b)
	if aok && bok {
		return ai.Cmp(bi)
	}
	if a < b {
		return -1
	}
	if a > b {
		return 1
	}
	return 0
}

func truthy(v value) bool {
	s := string(v)
	if s == "" {
		return false
	}
	if n, ok := integer(v); ok {
		return n.Sign() != 0
	}
	return true
}

func compileBRE(pattern string) (*bre.Regexp, bool, error) {
	re, err := bre.Compile(pattern)
	if err != nil {
		return nil, false, err
	}
	re.Longest()
	return re, hasBRECapture(pattern), nil
}

func hasBRECapture(pattern string) bool {
	inBracket := false
	for i := 0; i < len(pattern); i++ {
		switch pattern[i] {
		case '[':
			inBracket = true
		case ']':
			inBracket = false
		case '\\':
			if i+1 < len(pattern) {
				i++
				if !inBracket && pattern[i] == '(' {
					return true
				}
			}
		}
	}
	return false
}
