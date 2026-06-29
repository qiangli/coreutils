// Package findcmd implements find(1) per the GNU findutils manual:
// walk each starting-point and evaluate an expression for every file.
//
// Supported tests: -name GLOB, -iname GLOB, -path GLOB, -type LETTERS
// (b c d f l p s, comma-separated per findutils), -mtime [+-]N,
// -newer FILE, -size [+-]N[bcwkMGTP], -empty.
// Actions: -print (default), -print0, -prune.
// Operators: ( EXPR ), ! / -not, implicit and / -a / -and, -o / -or.
// Global options: -maxdepth N, -mindepth N (positional anywhere, as
// GNU applies them; the GNU positional warning is not emitted).
//
// -exec, -execdir, -ok, -okdir and -delete are deliberately not
// supported (-exec needs process execution, -delete could silently
// destroy data); they fail with the standard contract error.
//
// Deviations from GNU worth knowing: traversal order is deterministic
// lexical (GNU uses directory order); parse/usage errors exit 2 per
// this repo's contract (GNU find exits 1); paths are printed with
// forward slashes on every platform; -iname folds case per Unicode
// simple folding inside bracket ranges rather than C-locale collation.
package findcmd

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/qiangli/coreutils/pkg/ignore"
	"github.com/qiangli/coreutils/tool"
)

var cmd = &tool.Tool{
	Name:     "find",
	Synopsis: "Search for files in a directory hierarchy.",
	Usage:    "find [PATH...] [EXPRESSION]",
}

// Run is wired in init: a literal would create an initialization
// cycle (run's flag-error paths reference cmd).
func init() { cmd.Run = run; tool.Register(cmd) }

const helpText = `Usage: find [PATH...] [EXPRESSION]
Search for files in a directory hierarchy.

Default PATH is '.'; default expression is -print.

Tests:
  -name GLOB, -iname GLOB   base name matches shell glob
  -path GLOB                whole path matches glob ('/' not special)
  -type [bcdflps][,...]     file type
  -mtime [+-]N              data modified [more/less than] N*24h ago
  -newer FILE               modified more recently than FILE
  -size [+-]N[bcwkMGTP]     size in 512-byte blocks (default), bytes, ...
  -empty                    empty regular file or directory
Actions:
  -print                    print path, newline-terminated (default)
  -print0                   print path, NUL-terminated
  -prune                    do not descend into matched directories
Operators (decreasing precedence):
  ( EXPR )   ! EXPR   -not EXPR   EXPR1 [-a] EXPR2   EXPR1 -o EXPR2
Global options:
  -maxdepth N, -mindepth N  depth limits (start point is depth 0)

-exec, -execdir, -ok, -okdir and -delete are not supported by
pure-Go coreutils.
`

type notSupportedErr struct{ what string }

func (e *notSupportedErr) Error() string { return e.what }

func run(rc *tool.RunContext, args []string) int {
	agentic := false
	kept := args[:0:0]
	for _, a := range args {
		if a == "--help" {
			io.WriteString(rc.Out, helpText)
			return 0
		}
		if a == "--version" {
			fmt.Fprintf(rc.Out, "find (qiangli/coreutils) %s\n", tool.Version)
			return 0
		}
		// --agentic (opt-in): strip it before the path/expression split so the
		// hand-rolled parser never sees it as an expression token. Default off,
		// so without it find behaves exactly as before.
		if a == "--agentic" {
			agentic = true
			continue
		}
		kept = append(kept, a)
	}
	args = kept

	// Start points are everything before the first expression token.
	i := 0
	var paths []string
	for i < len(args) && !isExprToken(args[i]) {
		paths = append(paths, args[i])
		i++
	}
	if len(paths) == 0 {
		paths = []string{"."}
	}

	p := &parser{rc: rc, toks: args[i:], maxDepth: -1, now: time.Now()}
	root, err := p.parse()
	if err != nil {
		var ns *notSupportedErr
		if errors.As(err, &ns) {
			return tool.NotSupported(rc, cmd, ns.what)
		}
		return tool.UsageError(rc, cmd, "%v", err)
	}
	if !p.hasAction {
		root = &andExpr{root, &printExpr{}}
	}

	w := &walker{rc: rc, e: root, maxDepth: p.maxDepth, minDepth: p.minDepth}
	if agentic {
		w.matcher = ignore.New(rc.Dir)
	}
	for _, sp := range paths {
		w.walkRoot(sp)
	}
	// Transparency: announce what --agentic hid (stderr only).
	if n := w.matcher.Hidden(); n > 0 {
		fmt.Fprintf(rc.Err, "%s: --agentic skipped %d ignored path(s) (run without --agentic to include them)\n", cmd.Name, n)
	}
	if w.errored {
		return 1
	}
	return 0
}

func isExprToken(a string) bool {
	return strings.HasPrefix(a, "-") || a == "(" || a == ")" || a == "!" || a == ","
}

// ---------------------------------------------------------------------------
// expression parsing

type parser struct {
	rc        *tool.RunContext
	toks      []string
	i         int
	maxDepth  int // -1 = unset
	minDepth  int
	hasAction bool
	now       time.Time
}

func (p *parser) parse() (expr, error) {
	if len(p.toks) == 0 {
		return trueExpr{}, nil
	}
	e, err := p.parseOr()
	if err != nil {
		return nil, err
	}
	if p.i < len(p.toks) {
		return nil, fmt.Errorf("invalid expression; unexpected token '%s'", p.toks[p.i])
	}
	return e, nil
}

func (p *parser) parseOr() (expr, error) {
	l, err := p.parseAnd()
	if err != nil {
		return nil, err
	}
	for p.i < len(p.toks) && (p.toks[p.i] == "-o" || p.toks[p.i] == "-or") {
		p.i++
		r, err := p.parseAnd()
		if err != nil {
			return nil, err
		}
		l = &orExpr{l, r}
	}
	return l, nil
}

func (p *parser) parseAnd() (expr, error) {
	l, err := p.parseNot()
	if err != nil {
		return nil, err
	}
	for p.i < len(p.toks) {
		t := p.toks[p.i]
		if t == "-o" || t == "-or" || t == ")" {
			break
		}
		if t == "-a" || t == "-and" {
			p.i++
		}
		r, err := p.parseNot()
		if err != nil {
			return nil, err
		}
		l = &andExpr{l, r}
	}
	return l, nil
}

func (p *parser) parseNot() (expr, error) {
	if p.i < len(p.toks) && (p.toks[p.i] == "!" || p.toks[p.i] == "-not") {
		p.i++
		e, err := p.parseNot()
		if err != nil {
			return nil, err
		}
		return &notExpr{e}, nil
	}
	return p.primary()
}

func (p *parser) arg(name string) (string, error) {
	if p.i >= len(p.toks) {
		return "", fmt.Errorf("missing argument to '%s'", name)
	}
	a := p.toks[p.i]
	p.i++
	return a, nil
}

func (p *parser) primary() (expr, error) {
	if p.i >= len(p.toks) {
		return nil, errors.New("expected an expression")
	}
	t := p.toks[p.i]
	p.i++
	switch t {
	case "(":
		e, err := p.parseOr()
		if err != nil {
			return nil, err
		}
		if p.i >= len(p.toks) || p.toks[p.i] != ")" {
			return nil, errors.New("invalid expression; expected ')'")
		}
		p.i++
		return e, nil
	case "-print":
		p.hasAction = true
		return &printExpr{}, nil
	case "-print0":
		p.hasAction = true
		return &printExpr{nul: true}, nil
	case "-prune":
		return pruneExpr{}, nil
	case "-empty":
		return emptyExpr{}, nil
	case "-name", "-iname":
		a, err := p.arg(t)
		if err != nil {
			return nil, err
		}
		return &nameExpr{pat: a, fold: t == "-iname"}, nil
	case "-path":
		a, err := p.arg(t)
		if err != nil {
			return nil, err
		}
		return &pathExpr{pat: a}, nil
	case "-type":
		a, err := p.arg(t)
		if err != nil {
			return nil, err
		}
		return parseType(a)
	case "-maxdepth", "-mindepth":
		a, err := p.arg(t)
		if err != nil {
			return nil, err
		}
		n, err := strconv.Atoi(a)
		if err != nil || n < 0 {
			return nil, fmt.Errorf("expected a non-negative integer argument to %s, not '%s'", t, a)
		}
		if t == "-maxdepth" {
			p.maxDepth = n
		} else {
			p.minDepth = n
		}
		return trueExpr{}, nil
	case "-mtime":
		a, err := p.arg(t)
		if err != nil {
			return nil, err
		}
		cmp, n, err := parseSignedNum(a)
		if err != nil {
			return nil, fmt.Errorf("invalid argument '%s' to -mtime", a)
		}
		return &mtimeExpr{cmp: cmp, n: n, now: p.now}, nil
	case "-newer":
		a, err := p.arg(t)
		if err != nil {
			return nil, err
		}
		st, err := os.Stat(p.rc.Path(a))
		if err != nil {
			return nil, fmt.Errorf("'%s': %s", a, pathErrMsg(err))
		}
		return &newerExpr{ref: st.ModTime()}, nil
	case "-size":
		a, err := p.arg(t)
		if err != nil {
			return nil, err
		}
		return parseSize(a)
	case "-exec", "-execdir", "-ok", "-okdir", "-delete":
		return nil, &notSupportedErr{t + " (would execute commands or delete files)"}
	case ",":
		return nil, &notSupportedErr{"the ',' operator"}
	default:
		if strings.HasPrefix(t, "-") {
			return nil, fmt.Errorf("unknown predicate '%s'", t)
		}
		return nil, fmt.Errorf("paths must precede expression: '%s'", t)
	}
}

// parseSignedNum splits GNU's [+-]N numeric argument shape.
// cmp is '+', '-', or 0 for exact.
func parseSignedNum(s string) (cmp byte, n int64, err error) {
	if s != "" && (s[0] == '+' || s[0] == '-') {
		cmp = s[0]
		s = s[1:]
	}
	n, err = strconv.ParseInt(s, 10, 64)
	if err != nil || n < 0 {
		return 0, 0, errors.New("not a number")
	}
	return cmp, n, nil
}

func parseType(a string) (expr, error) {
	te := &typeExpr{}
	for _, part := range strings.Split(a, ",") {
		if len(part) != 1 || !strings.ContainsAny(part, "bcdflps") {
			return nil, fmt.Errorf("unknown argument to -type: %s", part)
		}
		te.letters += part
	}
	return te, nil
}

var sizeUnits = map[byte]int64{
	'b': 512, 'c': 1, 'w': 2,
	'k': 1 << 10, 'M': 1 << 20, 'G': 1 << 30, 'T': 1 << 40, 'P': 1 << 50,
}

func parseSize(a string) (expr, error) {
	s := a
	var cmp byte
	if s != "" && (s[0] == '+' || s[0] == '-') {
		cmp = s[0]
		s = s[1:]
	}
	unit := int64(512) // default: 512-byte blocks
	if s != "" {
		if u, ok := sizeUnits[s[len(s)-1]]; ok {
			unit = u
			s = s[:len(s)-1]
		}
	}
	n, err := strconv.ParseInt(s, 10, 64)
	if err != nil || n < 0 {
		return nil, fmt.Errorf("invalid argument '%s' to -size", a)
	}
	return &sizeExpr{cmp: cmp, n: n, unit: unit}, nil
}

// ---------------------------------------------------------------------------
// expression evaluation

type fctx struct {
	path     string // display path (operand-rooted, forward slashes)
	osPath   string
	d        fs.DirEntry
	info     fs.FileInfo
	statDone bool
	pruned   bool
	w        *walker
}

// stat lazily lstats the current file (WalkDir's DirEntry.Info).
func (c *fctx) stat() fs.FileInfo {
	if c.statDone {
		return c.info
	}
	c.statDone = true
	info, err := c.d.Info()
	if err != nil {
		c.w.reportErr(c.path, err)
		return nil
	}
	c.info = info
	return c.info
}

type expr interface{ eval(c *fctx) bool }

type trueExpr struct{}

func (trueExpr) eval(*fctx) bool { return true }

type notExpr struct{ e expr }

func (n *notExpr) eval(c *fctx) bool { return !n.e.eval(c) }

type andExpr struct{ l, r expr }

func (a *andExpr) eval(c *fctx) bool { return a.l.eval(c) && a.r.eval(c) }

type orExpr struct{ l, r expr }

func (o *orExpr) eval(c *fctx) bool { return o.l.eval(c) || o.r.eval(c) }

type printExpr struct{ nul bool }

func (p *printExpr) eval(c *fctx) bool {
	if p.nul {
		fmt.Fprintf(c.w.rc.Out, "%s\x00", c.path)
	} else {
		fmt.Fprintf(c.w.rc.Out, "%s\n", c.path)
	}
	return true
}

type pruneExpr struct{}

func (pruneExpr) eval(c *fctx) bool {
	if c.d.IsDir() {
		c.pruned = true
	}
	return true
}

type nameExpr struct {
	pat  string
	fold bool
}

func (n *nameExpr) eval(c *fctx) bool {
	return fnmatch(n.pat, filepath.Base(c.osPath), n.fold)
}

type pathExpr struct{ pat string }

func (p *pathExpr) eval(c *fctx) bool {
	// GNU -path: matched against the path as printed; wildcards and
	// the match in general do not treat '/' specially.
	return fnmatch(p.pat, c.path, false)
}

type typeExpr struct{ letters string }

func (t *typeExpr) eval(c *fctx) bool {
	m := c.d.Type()
	for i := 0; i < len(t.letters); i++ {
		ok := false
		switch t.letters[i] {
		case 'f':
			ok = m.IsRegular()
		case 'd':
			ok = m.IsDir()
		case 'l':
			ok = m&fs.ModeSymlink != 0
		case 's':
			ok = m&fs.ModeSocket != 0
		case 'p':
			ok = m&fs.ModeNamedPipe != 0
		case 'c':
			ok = m&fs.ModeCharDevice != 0
		case 'b':
			ok = m&fs.ModeDevice != 0 && m&fs.ModeCharDevice == 0
		}
		if ok {
			return true
		}
	}
	return false
}

type mtimeExpr struct {
	cmp byte
	n   int64
	now time.Time
}

func (m *mtimeExpr) eval(c *fctx) bool {
	info := c.stat()
	if info == nil {
		return false
	}
	days := int64(m.now.Sub(info.ModTime()) / (24 * time.Hour))
	switch m.cmp {
	case '+':
		return days > m.n
	case '-':
		return days < m.n
	default:
		return days == m.n
	}
}

type newerExpr struct{ ref time.Time }

func (n *newerExpr) eval(c *fctx) bool {
	info := c.stat()
	return info != nil && info.ModTime().After(n.ref)
}

type sizeExpr struct {
	cmp  byte
	n    int64
	unit int64
}

func (s *sizeExpr) eval(c *fctx) bool {
	info := c.stat()
	if info == nil {
		return false
	}
	v := (info.Size() + s.unit - 1) / s.unit // GNU rounds up
	switch s.cmp {
	case '+':
		return v > s.n
	case '-':
		return v < s.n
	default:
		return v == s.n
	}
}

type emptyExpr struct{}

func (emptyExpr) eval(c *fctx) bool {
	if c.d.IsDir() {
		ents, err := os.ReadDir(c.osPath)
		if err != nil {
			c.w.reportErr(c.path, err)
			return false
		}
		return len(ents) == 0
	}
	info := c.stat()
	return info != nil && info.Mode().IsRegular() && info.Size() == 0
}

// ---------------------------------------------------------------------------
// traversal

type walker struct {
	rc       *tool.RunContext
	e        expr
	maxDepth int // -1 = unlimited
	minDepth int
	errored  bool
	matcher  *ignore.Matcher // --agentic path filter (nil = off, skips nothing)
}

func (w *walker) reportErr(display string, err error) {
	w.errored = true
	fmt.Fprintf(w.rc.Err, "%s: '%s': %s\n", cmd.Name, display, pathErrMsg(err))
}

func (w *walker) walkRoot(operand string) {
	root := w.rc.Path(operand)
	filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		disp := displayPath(operand, root, p)
		if err != nil {
			w.reportErr(disp, err)
			return nil
		}
		// --agentic: prune .gitignore'd / noise paths (never the start point).
		if p != root && w.matcher.Skip(p, d.IsDir()) {
			if d.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		depth := 0
		if rel, rerr := filepath.Rel(root, p); rerr == nil && rel != "." {
			depth = strings.Count(rel, string(filepath.Separator)) + 1
		}
		var skip error
		if d.IsDir() && w.maxDepth >= 0 && depth >= w.maxDepth {
			skip = fs.SkipDir
		}
		if depth < w.minDepth {
			return skip
		}
		c := &fctx{path: disp, osPath: p, d: d, w: w}
		w.e.eval(c)
		if c.pruned {
			return fs.SkipDir
		}
		return skip
	})
}

// displayPath joins the operand as typed with the walk position,
// using forward slashes (matches GNU output shape on unix).
func displayPath(operand, root, p string) string {
	rel, err := filepath.Rel(root, p)
	if err != nil || rel == "." {
		return operand
	}
	rel = filepath.ToSlash(rel)
	if strings.HasSuffix(operand, "/") {
		return operand + rel
	}
	return operand + "/" + rel
}

// pathErrMsg strips Go's "lstat <path>: " prefix so diagnostics read
// like GNU's "find: '<name>': <reason>".
func pathErrMsg(err error) string {
	var pe *fs.PathError
	if errors.As(err, &pe) {
		return pe.Err.Error()
	}
	return err.Error()
}

// ---------------------------------------------------------------------------
// fnmatch: POSIX shell glob for -name/-iname/-path. Unlike
// path.Match, '*' and '?' also match '/' (GNU -path rule), backslash
// escapes the next character, and [!...] negation is accepted.

func fnmatch(pattern, name string, fold bool) bool {
	return fnmatchRunes([]rune(pattern), []rune(name), fold)
}

func fnmatchRunes(p, s []rune, fold bool) bool {
	for len(p) > 0 {
		switch p[0] {
		case '*':
			for len(p) > 0 && p[0] == '*' {
				p = p[1:]
			}
			if len(p) == 0 {
				return true
			}
			for i := 0; i <= len(s); i++ {
				if fnmatchRunes(p, s[i:], fold) {
					return true
				}
			}
			return false
		case '?':
			if len(s) == 0 {
				return false
			}
			p, s = p[1:], s[1:]
		case '[':
			if len(s) == 0 {
				return false
			}
			matched, rest, valid := matchClass(p, s[0], fold)
			if !valid {
				// unmatched '[' is a literal
				if !eqRune('[', s[0], fold) {
					return false
				}
				p, s = p[1:], s[1:]
				continue
			}
			if !matched {
				return false
			}
			p, s = rest, s[1:]
		case '\\':
			if len(p) >= 2 {
				p = p[1:]
			}
			if len(s) == 0 || !eqRune(p[0], s[0], fold) {
				return false
			}
			p, s = p[1:], s[1:]
		default:
			if len(s) == 0 || !eqRune(p[0], s[0], fold) {
				return false
			}
			p, s = p[1:], s[1:]
		}
	}
	return len(s) == 0
}

func eqRune(a, b rune, fold bool) bool {
	if a == b {
		return true
	}
	return fold && unicode.ToLower(a) == unicode.ToLower(b)
}

var classFns = map[string]func(rune) bool{
	"alpha":  unicode.IsLetter,
	"digit":  unicode.IsDigit,
	"alnum":  func(r rune) bool { return unicode.IsLetter(r) || unicode.IsDigit(r) },
	"upper":  unicode.IsUpper,
	"lower":  unicode.IsLower,
	"space":  unicode.IsSpace,
	"blank":  func(r rune) bool { return r == ' ' || r == '\t' },
	"punct":  unicode.IsPunct,
	"cntrl":  unicode.IsControl,
	"graph":  unicode.IsGraphic,
	"print":  unicode.IsPrint,
	"xdigit": func(r rune) bool { return strings.ContainsRune("0123456789abcdefABCDEF", r) },
}

// matchClass parses one bracket expression at the head of p and tests
// r against it. valid=false means the '[' had no closing ']' and must
// be treated as a literal.
func matchClass(p []rune, r rune, fold bool) (matched bool, rest []rune, valid bool) {
	i := 1
	neg := false
	if i < len(p) && (p[i] == '!' || p[i] == '^') {
		neg = true
		i++
	}
	first := true
	for i < len(p) {
		if p[i] == ']' && !first {
			return matched != neg, p[i+1:], true
		}
		first = false
		// [:class:]
		if p[i] == '[' && i+1 < len(p) && p[i+1] == ':' {
			j := i + 2
			for j+1 < len(p) && !(p[j] == ':' && p[j+1] == ']') {
				j++
			}
			if j+1 >= len(p) {
				return false, nil, false
			}
			if fn, ok := classFns[string(p[i+2:j])]; ok && fn(r) {
				matched = true
			}
			i = j + 2
			continue
		}
		lo := p[i]
		if lo == '\\' && i+1 < len(p) {
			i++
			lo = p[i]
		}
		i++
		hi := lo
		if i+1 < len(p) && p[i] == '-' && p[i+1] != ']' {
			i++
			hi = p[i]
			if hi == '\\' && i+1 < len(p) {
				i++
				hi = p[i]
			}
			i++
		}
		c := r
		if fold {
			c, lo, hi = unicode.ToLower(r), unicode.ToLower(lo), unicode.ToLower(hi)
		}
		if lo <= c && c <= hi {
			matched = true
		}
	}
	return false, nil, false
}
