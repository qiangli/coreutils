package exprcmd

import (
	"errors"
	"fmt"
	"math/big"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/qiangli/coreutils/pkg/bre"
	"github.com/qiangli/coreutils/pkg/collate"
	"github.com/qiangli/coreutils/pkg/ctype"
	"github.com/qiangli/coreutils/pkg/locale"
	"github.com/qiangli/coreutils/tool"
)

var cmd = &tool.Tool{
	Name:     "expr",
	Synopsis: "Evaluate expressions.",
	Usage:    "expr EXPRESSION",
}

func init() { cmd.Run = run; tool.Register(cmd) }

type value string

type ctypeProvider interface {
	bre.ByteCtype
	Close() error
}

type ctypeOpener func(string) (ctypeProvider, error)

type collateProvider interface {
	bre.ByteEquivalence
	bre.ByteEquivalenceValidity
	bre.ByteCollationWeights
	bre.ByteCollatingElements
	Compare(string, string) (int, error)
	Close() error
}

type collateOpener func(string) (collateProvider, error)

type exprLocale struct {
	characters characterCodec
	compare    func(value, value) (int, error)
	tables     *bre.LocaleByteTables
	close      func() error
}

type characterCodec int

const (
	characterBytes characterCodec = iota
	characterUTF8
	characterLatin1
)

// evalError marks a runtime evaluation failure — a well-formed expression that
// cannot be evaluated, such as division by zero. GNU expr reports these with
// EXPR_FAILURE (exit 3), distinct from EXPR_INVALID (exit 2) used for a
// syntactically invalid expression; POSIX likewise mandates ">2" for an error
// that occurs versus "2" for an invalid expression.
type evalError struct{ msg string }

func (e *evalError) Error() string { return e.msg }

func resolveExprLocale(rc *tool.RunContext, ctypeOpen ctypeOpener, collateOpen collateOpener) (*exprLocale, int) {
	lcCType := locale.Resolve(rc.Env, locale.CType)
	lcCollate := locale.Resolve(rc.Env, locale.Collate)
	characters, err := resolveCharacterCodec(lcCType)
	if err != nil {
		fmt.Fprintf(rc.Err, "expr: %v\n", err)
		return nil, 2
	}
	loc := &exprLocale{characters: characters, compare: compareC}
	var tables *bre.LocaleByteTables
	if needsCTypeProvider(lcCType) {
		provider, err := ctypeOpen(lcCType)
		if err != nil {
			fmt.Fprintf(rc.Err, "expr: LC_CTYPE %q: %v\n", lcCType, err)
			return nil, 2
		}
		var snapshotErr error
		tables, snapshotErr = bre.SnapshotLocaleByteCtypeTables(provider)
		closeErr := provider.Close()
		if snapshotErr != nil {
			fmt.Fprintf(rc.Err, "expr: LC_CTYPE %q: %v\n", lcCType, snapshotErr)
			return nil, 2
		}
		if closeErr != nil {
			fmt.Fprintf(rc.Err, "expr: LC_CTYPE %q: %v\n", lcCType, closeErr)
			return nil, 2
		}
	} else {
		tables, _ = bre.SnapshotLocaleByteCtypeTables(nil)
	}
	if lcCollate != "C" && lcCollate != "POSIX" {
		provider, err := collateOpen(lcCollate)
		if err != nil {
			fmt.Fprintf(rc.Err, "expr: LC_COLLATE %q: %v\n", lcCollate, err)
			return nil, 2
		}
		var snapshotErr error
		tables, snapshotErr = tables.WithCollation(provider)
		if snapshotErr != nil {
			_ = provider.Close()
			fmt.Fprintf(rc.Err, "expr: LC_COLLATE %q: %v\n", lcCollate, snapshotErr)
			return nil, 2
		}
		loc.compare = func(a, b value) (int, error) {
			ai, aok := integer(a)
			bi, bok := integer(b)
			if aok && bok {
				return ai.Cmp(bi), nil
			}
			return provider.Compare(string(a), string(b))
		}
		loc.close = func() error {
			if err := provider.Close(); err != nil {
				return fmt.Errorf("LC_COLLATE %q: %w", lcCollate, err)
			}
			return nil
		}
	}
	if needsCTypeProvider(lcCType) || lcCollate != "C" && lcCollate != "POSIX" {
		loc.tables = tables
	}
	return loc, -1
}

func needsCTypeProvider(name string) bool {
	if name == "C" || name == "POSIX" {
		return false
	}
	base, codeset := splitLocaleName(name)
	return !((base == "C" || base == "POSIX") && normalizeCodeset(codeset) == "UTF8")
}

func (l *exprLocale) closeLocale() error {
	if l != nil && l.close != nil {
		return l.close()
	}
	return nil
}

func resolveCharacterCodec(name string) (characterCodec, error) {
	base, codeset := splitLocaleName(name)
	switch {
	case (base == "C" || base == "POSIX") && codeset == "":
		return characterBytes, nil
	case (base == "C" || base == "POSIX") && normalizeCodeset(codeset) == "UTF8":
		return characterUTF8, nil
	case strings.EqualFold(base, "de_DE") && normalizeCodeset(codeset) == "ISO88591":
		return characterLatin1, nil
	default:
		return characterBytes, fmt.Errorf(
			"LC_CTYPE %q is unavailable; supported locales are C/POSIX, their UTF-8 aliases, and de_DE.ISO-8859-1",
			name,
		)
	}
}

func splitLocaleName(name string) (base, codeset string) {
	name, _, _ = strings.Cut(name, "@")
	base, codeset, _ = strings.Cut(name, ".")
	return base, codeset
}

func normalizeCodeset(name string) string {
	return strings.ToUpper(strings.NewReplacer("-", "", "_", "").Replace(name))
}

func (c characterCodec) count(s string) int {
	return len(c.split(s))
}

func (c characterCodec) split(s string) [][]byte {
	switch c {
	case characterUTF8:
		var out [][]byte
		for len(s) > 0 {
			r, size := utf8.DecodeRuneInString(s)
			if r == utf8.RuneError && size == 0 {
				break
			}
			out = append(out, []byte(s[:size]))
			s = s[size:]
		}
		return out
	default:
		out := make([][]byte, len(s))
		for i := range s {
			out[i] = []byte{s[i]}
		}
		return out
	}
}

func joinCharacterUnits(units [][]byte) string {
	var b strings.Builder
	for _, unit := range units {
		b.Write(unit)
	}
	return b.String()
}

func run(rc *tool.RunContext, args []string) int {
	return runWithLocales(rc, args, func(name string) (ctypeProvider, error) { return ctype.Open(name) }, func(name string) (collateProvider, error) { return collate.Open(name) })
}

func runWithLocales(rc *tool.RunContext, args []string, ctypeOpen ctypeOpener, collateOpen collateOpener) int {
	// The "--" delimiter and the GNU keyword extensions (length, substr,
	// index, and match) are deliberately not gated on POSIXLY_CORRECT:
	// XBD Utility Syntax Guideline 10 and the expr APPLICATION USAGE require
	// "--" to protect leading-minus operands, and Issue 7 leaves the keyword
	// results unspecified, so the extensions cannot conflict with the grammar.
	if len(args) == 1 && args[0] == "--help" {
		_, err := fmt.Fprintf(rc.Out, "Usage: %s\n%s\n\nOptions:\n      --help     display this help and exit\n      --version  output version information and exit\n", cmd.Usage, cmd.Synopsis)
		if err != nil && rc.SIGPIPEIgnored && tool.IsClosedPipeError(err) {
			fmt.Fprintln(rc.Err, "expr: stdout: Broken pipe")
			return 1
		}
		return 0
	}
	if len(args) == 1 && args[0] == "--version" {
		_, err := fmt.Fprintf(rc.Out, "%s (qiangli/coreutils) %s\n", cmd.Name, tool.Version)
		if err != nil && rc.SIGPIPEIgnored && tool.IsClosedPipeError(err) {
			fmt.Fprintln(rc.Err, "expr: stdout: Broken pipe")
			return 1
		}
		return 0
	}
	if len(args) > 0 && args[0] == "--" {
		args = args[1:]
	}
	if len(args) == 0 {
		return tool.UsageError(rc, cmd, "missing operand")
	}
	loc, code := resolveExprLocale(rc, ctypeOpen, collateOpen)
	if code >= 0 {
		return code
	}
	p := &parser{tokens: args, locale: loc}
	v, err := p.parseOr()
	if err == nil && p.more() {
		err = fmt.Errorf("syntax error")
	}
	if closeErr := loc.closeLocale(); err == nil && closeErr != nil {
		err = closeErr
	}
	if err != nil {
		fmt.Fprintf(rc.Err, "expr: %v\n", err)
		var ee *evalError
		if errors.As(err, &ee) {
			return 3
		}
		return 2
	}
	_, errOut := fmt.Fprintln(rc.Out, string(v))
	if errOut != nil && rc.SIGPIPEIgnored && tool.IsClosedPipeError(errOut) {
		fmt.Fprintln(rc.Err, "expr: stdout: Broken pipe")
		return 1
	}
	if truthy(v) {
		return 0
	}
	return 1
}

type parser struct {
	tokens []string
	pos    int
	locale *exprLocale
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
		cmp, err := p.locale.compare(left, right)
		if err != nil {
			return "", err
		}
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
				return "", &evalError{"division by zero"}
			}
			left = value(new(big.Int).Quo(a, b).String())
		case "%":
			if b.Sign() == 0 {
				return "", &evalError{"division by zero"}
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
		re, captures, err := p.compileBRE(string(pat))
		if err != nil {
			return "", fmt.Errorf("invalid regular expression")
		}
		matches, err := re.findAllStringSubmatchIndex(string(left), 1)
		if err != nil {
			return "", err
		}
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
			left = value(strconv.Itoa(p.locale.characters.count(string(left)[m[0]:m[1]])))
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
	if t == "length" || t == "index" || t == "substr" || t == "match" {
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
		if err != nil {
			return "", err
		}
		return value(strconv.Itoa(p.locale.characters.count(string(v)))), nil
	case "index":
		s, err := arg()
		if err != nil {
			return "", err
		}
		chars, err := arg()
		if err != nil {
			return "", err
		}
		haystack := p.locale.characters.split(string(s))
		needles := p.locale.characters.split(string(chars))
		for i, r := range haystack {
			for _, c := range needles {
				if string(r) == string(c) {
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
		rs := p.locale.characters.split(string(s))
		if pos.Sign() <= 0 || ln.Sign() <= 0 {
			return "", nil
		}
		// Compare the arbitrary-precision position before converting it to an
		// index. A length larger than the remaining input is valid: it selects
		// through the end of the string.
		available := big.NewInt(int64(len(rs)))
		if pos.Cmp(available) > 0 {
			return "", nil
		}
		start := pos.Int64() - 1
		remaining := int64(len(rs)) - start
		if ln.Cmp(big.NewInt(remaining)) >= 0 {
			return value(joinCharacterUnits(rs[int(start):])), nil
		}
		end := start + ln.Int64()
		return value(joinCharacterUnits(rs[int(start):int(end)])), nil
	case "match":
		s, err := arg()
		if err != nil {
			return "", err
		}
		pat, err := arg()
		if err != nil {
			return "", err
		}
		sub := &parser{tokens: []string{string(s), ":", string(pat)}, locale: p.locale}
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

func compareC(a, b value) (int, error) {
	ai, aok := integer(a)
	bi, bok := integer(b)
	if aok && bok {
		return ai.Cmp(bi), nil
	}
	if a < b {
		return -1, nil
	}
	if a > b {
		return 1, nil
	}
	return 0, nil
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

type exprRegexp struct {
	re       *bre.Regexp
	localeRe *bre.LocaleByteRegexp
}

func (r exprRegexp) findAllStringSubmatchIndex(src string, n int) ([][]int, error) {
	if r.localeRe != nil {
		return r.localeRe.FindAllStringSubmatchIndex(src, n)
	}
	return r.re.FindAllStringSubmatchIndexErr(src, n)
}

func (p *parser) compileBRE(pattern string) (exprRegexp, bool, error) {
	if p.locale.tables != nil {
		re, err := bre.CompileLocaleByteRegexpTables([]byte(pattern), p.locale.tables, bre.ByteRegexpOptions{Syntax: bre.ByteRegexpBRE})
		return exprRegexp{localeRe: re}, hasBRECapture(pattern), err
	}
	re, err := bre.Compile(pattern)
	if err != nil {
		return exprRegexp{}, false, err
	}
	re.Longest()
	return exprRegexp{re: re}, hasBRECapture(pattern), nil
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
