package exprcmd

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"

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
	if len(args) == 1 && args[0] == "--help" {
		fmt.Fprintln(rc.Out, cmd.Usage)
		return 0
	}
	if len(args) == 1 && args[0] == "--version" {
		fmt.Fprintln(rc.Out, "expr (coreutils-go)")
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
		if truthy(left) {
			return left, nil
		}
		left = right
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
			left = right
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
		a, b, err := ints(left, right)
		if err != nil {
			return "", err
		}
		if op == "+" {
			left = value(strconv.FormatInt(a+b, 10))
		} else {
			left = value(strconv.FormatInt(a-b, 10))
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
		a, b, err := ints(left, right)
		if err != nil {
			return "", err
		}
		switch op {
		case "*":
			left = value(strconv.FormatInt(a*b, 10))
		case "/":
			if b == 0 {
				return "", fmt.Errorf("division by zero")
			}
			left = value(strconv.FormatInt(a/b, 10))
		case "%":
			if b == 0 {
				return "", fmt.Errorf("division by zero")
			}
			left = value(strconv.FormatInt(a%b, 10))
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
		m := re.FindStringSubmatchIndex(string(left))
		if len(m) == 0 {
			if captures {
				left = ""
			} else {
				left = "0"
			}
		} else if captures {
			if len(m) > 3 && m[2] >= 0 {
				left = value(string(left)[m[2]:m[3]])
			} else {
				left = ""
			}
		} else {
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
		pos, ln, err := ints(posv, lenv)
		if err != nil {
			return "", err
		}
		rs := []rune(s)
		start := int(pos) - 1
		if start < 0 || start >= len(rs) || ln <= 0 {
			return "", nil
		}
		end := start + int(ln)
		if end > len(rs) {
			end = len(rs)
		}
		return value(string(rs[start:end])), nil
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

func ints(a, b value) (int64, int64, error) {
	ai, err := strconv.ParseInt(string(a), 10, 64)
	if err != nil {
		return 0, 0, fmt.Errorf("non-integer argument")
	}
	bi, err := strconv.ParseInt(string(b), 10, 64)
	if err != nil {
		return 0, 0, fmt.Errorf("non-integer argument")
	}
	return ai, bi, nil
}

func compare(a, b value) int {
	ai, ea := strconv.ParseInt(string(a), 10, 64)
	bi, eb := strconv.ParseInt(string(b), 10, 64)
	if ea == nil && eb == nil {
		if ai < bi {
			return -1
		}
		if ai > bi {
			return 1
		}
		return 0
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
	if s == "-" {
		return true
	}
	if strings.HasPrefix(s, "-") {
		return strings.Trim(s[1:], "0") != ""
	}
	return strings.Trim(s, "0") != ""
}

func compileBRE(pattern string) (*regexp.Regexp, bool, error) {
	converted := convertBRE(pattern)
	captures := strings.Contains(pattern, `\(`)
	if !strings.HasPrefix(converted, "^") {
		converted = "^" + converted
	}
	re, err := regexp.Compile(converted)
	return re, captures, err
}

func convertBRE(pattern string) string {
	var b strings.Builder
	for i := 0; i < len(pattern); i++ {
		if pattern[i] == '\\' && i+1 < len(pattern) {
			i++
			switch pattern[i] {
			case '(', ')', '{', '}', '|':
				b.WriteByte(pattern[i])
			default:
				b.WriteByte('\\')
				b.WriteByte(pattern[i])
			}
			continue
		}
		switch pattern[i] {
		case '+', '?', '|', '(', ')', '{', '}':
			b.WriteByte('\\')
			b.WriteByte(pattern[i])
		default:
			b.WriteByte(pattern[i])
		}
	}
	return b.String()
}
