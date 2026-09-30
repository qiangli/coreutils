package m4cmd

import (
	"bufio"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/qiangli/coreutils/tool"
)

// maxNesting bounds nested argument collection (a macro call inside the
// arguments of another). Rescanning an expansion does not nest, so
// ordinary recursive macros are unaffected; this only turns unbounded
// argument recursion into a diagnostic instead of stack exhaustion.
const maxNesting = 4096

// macro is one definition: a built-in (fn set) or defining text.
type macro struct {
	text string
	fn   builtinFunc
	name string // the built-in's own name, for diagnostics
	// blind built-ins are recognized only when followed by '('; without
	// one the name is ordinary text.
	blind bool
}

// argument is one collected macro argument. fn is set when the argument
// is the definition of a built-in handed over by defn, so that define
// and pushdef can rename built-ins.
type argument struct {
	text string
	fn   *macro
}

type tokenKind int

const (
	tokEOF  tokenKind = iota
	tokText           // quoted string (quotes stripped) or comment: never rescanned
	tokName
	tokChar
)

// fatal aborts processing; process recovers it and exits 1.
type fatal struct{}

// sourceLoc is the identity carried by one byte of pushed-back input. Macro
// expansions inherit the call site; included bytes carry their own file and
// line so diagnostics and -s output follow the included source and then return
// to the caller accurately.
type sourceLoc struct {
	file string
	line int
	seq  int
}

type processor struct {
	rc     *tool.RunContext
	out    *bufio.Writer
	macros map[string][]*macro

	lquote, rquote string
	bcomm, ecomm   string

	// Input is the pushback stack (stored reversed, so pushing an
	// expansion is an append) in front of the current file's bytes.
	pushed    []byte
	pushedLoc []sourceLoc
	src       []byte
	pos       int
	files     []string

	file    string
	line    int
	fileSeq int

	srcFile   string
	srcLine   int
	srcPendNL bool // a source newline was read; its line advances with the next source byte
	srcSeq    int
	nextSeq   int

	sync    bool
	bol     bool
	outLine int
	outSeq  int

	nesting int
	ticks   int
	failed  bool
	stdin   bool // standard input has been consumed

	divnum     int
	diversions [10]strings.Builder
	wraps      []string
	traceAll   bool
	trace      map[string]bool
	sysval     int
	exitSet    bool
	exitCode   int
}

func newProcessor(rc *tool.RunContext, files []string, sync bool) *processor {
	p := &processor{
		rc:     rc,
		out:    bufio.NewWriter(rc.Out),
		macros: map[string][]*macro{},
		trace:  map[string]bool{},
		lquote: "`", rquote: "'",
		bcomm: "#", ecomm: "\n",
		files: files,
		sync:  sync,
		bol:   true,
	}
	for name, b := range builtins {
		p.macros[name] = []*macro{b}
	}
	return p
}

func (p *processor) process() (code int) {
	defer func() {
		if r := recover(); r != nil {
			if _, ok := r.(fatal); !ok {
				panic(r)
			}
			code = 1
			if p.exitSet {
				code = p.exitCode
			}
		}
		if err := p.out.Flush(); err != nil && code == 0 {
			fmt.Fprintf(p.rc.Err, "m4: write error: %v\n", err)
			code = 1
		}
	}()
	for {
		kind, s := p.token()
		switch kind {
		case tokEOF:
			if len(p.wraps) > 0 {
				n := len(p.wraps) - 1
				w := p.wraps[n]
				p.wraps = p.wraps[:n]
				// Wrapped input is evaluated after the source is exhausted;
				// GNU-compatible synclines identify that synthetic input as line 0.
				p.line = 0
				p.pushback(w)
				continue
			}
			p.flushDiversions()
			if p.failed {
				return 1
			}
			return 0
		case tokName:
			if handled, _ := p.expand(s); handled {
				continue
			}
			p.emit(s)
		default:
			p.emit(s)
		}
	}
}

// warnf reports a diagnostic located at the current input position.
func (p *processor) warnf(format string, a ...any) {
	p.out.Flush()
	fmt.Fprintf(p.rc.Err, "m4:%s:%d: %s\n", p.file, p.line, fmt.Sprintf(format, a...))
}

func (p *processor) fatalf(format string, a ...any) {
	p.warnf(format, a...)
	panic(fatal{})
}

// ---- input ----

// nextFile makes the next readable operand the current input.
func (p *processor) nextFile() bool {
	for len(p.files) > 0 {
		name := p.files[0]
		p.files = p.files[1:]
		var data []byte
		var err error
		display := name
		if name == "-" {
			display = "stdin"
			if !p.stdin && p.rc.In != nil {
				data, err = io.ReadAll(p.rc.In)
			}
			p.stdin = true
		} else {
			var f io.ReadCloser
			if f, err = p.rc.FS.Open(p.rc.Path(name)); err == nil {
				data, err = io.ReadAll(f)
				if cerr := f.Close(); err == nil {
					err = cerr
				}
			}
		}
		if err != nil {
			p.out.Flush()
			fmt.Fprintf(p.rc.Err, "m4: %s: %v\n", name, err)
			p.failed = true
			continue
		}
		p.src, p.pos = data, 0
		p.nextSeq++
		p.srcFile, p.srcLine, p.srcPendNL, p.srcSeq = display, 1, false, p.nextSeq
		p.file, p.line, p.fileSeq = p.srcFile, p.srcLine, p.srcSeq
		return true
	}
	return false
}

// fill reports whether any input remains, opening the next file when
// the current one is exhausted.
func (p *processor) fill() bool {
	for len(p.pushed) == 0 && p.pos >= len(p.src) {
		if !p.nextFile() {
			return false
		}
	}
	return true
}

func (p *processor) next() (byte, bool) {
	if !p.fill() {
		return 0, false
	}
	if n := len(p.pushed); n > 0 {
		c := p.pushed[n-1]
		loc := p.pushedLoc[n-1]
		p.pushed = p.pushed[:n-1]
		p.pushedLoc = p.pushedLoc[:n-1]
		p.file, p.line, p.fileSeq = loc.file, loc.line, loc.seq
		return c, true
	}
	if p.srcPendNL {
		p.srcLine++
		p.srcPendNL = false
	}
	c := p.src[p.pos]
	p.pos++
	p.file, p.line, p.fileSeq = p.srcFile, p.srcLine, p.srcSeq
	if c == '\n' {
		p.srcPendNL = true
	}
	return c, true
}

// peekAt looks i bytes ahead without consuming and without crossing
// into the next file: a token never spans two operands.
func (p *processor) peekAt(i int) (byte, bool) {
	n := len(p.pushed)
	if i < n {
		return p.pushed[n-1-i], true
	}
	if j := p.pos + i - n; j < len(p.src) {
		return p.src[j], true
	}
	return 0, false
}

func (p *processor) hasPrefix(s string) bool {
	for i := 0; i < len(s); i++ {
		if c, ok := p.peekAt(i); !ok || c != s[i] {
			return false
		}
	}
	return true
}

func (p *processor) skip(n int) {
	for ; n > 0; n-- {
		p.next()
	}
}

// pushback makes s the next input to be scanned.
func (p *processor) pushback(s string) {
	loc := sourceLoc{file: p.file, line: p.line, seq: p.fileSeq}
	for i := len(s) - 1; i >= 0; i-- {
		p.pushed = append(p.pushed, s[i])
		p.pushedLoc = append(p.pushedLoc, loc)
	}
}

// pushIncluded puts a named file ahead of all remaining input. Unlike an
// ordinary macro expansion, every byte carries the included file's advancing
// line identity. The already-pushed caller bytes retain their original
// identity, which restores diagnostics and synclines after the include ends.
func (p *processor) pushIncluded(name string, data []byte) {
	p.nextSeq++
	seq := p.nextSeq
	locs := make([]sourceLoc, len(data))
	line := 1
	for i, c := range data {
		locs[i] = sourceLoc{file: name, line: line, seq: seq}
		if c == '\n' {
			line++
		}
	}
	for i := len(data) - 1; i >= 0; i-- {
		p.pushed = append(p.pushed, data[i])
		p.pushedLoc = append(p.pushedLoc, locs[i])
	}
}

// ---- scanner ----

func isNameStart(c byte) bool {
	return c == '_' || (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z')
}

func isNameChar(c byte) bool { return isNameStart(c) || (c >= '0' && c <= '9') }

func isSpace(c byte) bool {
	return c == ' ' || c == '\t' || c == '\n' || c == '\r' || c == '\v' || c == '\f'
}

// token reads the next token. A comment is matched before a name and a
// name before a quoted string, so a delimiter that starts with a letter
// loses to the name it is part of while a comment delimiter does not.
func (p *processor) token() (tokenKind, string) {
	if p.ticks++; p.ticks&0xfff == 0 && p.rc.Ctx != nil && p.rc.Ctx.Err() != nil {
		p.fatalf("interrupted")
	}
	if !p.fill() {
		return tokEOF, ""
	}
	if p.bcomm != "" && p.hasPrefix(p.bcomm) {
		p.skip(len(p.bcomm))
		b := []byte(p.bcomm)
		for {
			if p.hasPrefix(p.ecomm) {
				p.skip(len(p.ecomm))
				return tokText, string(append(b, p.ecomm...))
			}
			if _, ok := p.peekAt(0); !ok {
				return tokText, string(b)
			}
			c, _ := p.next()
			b = append(b, c)
		}
	}
	c, _ := p.peekAt(0)
	if isNameStart(c) {
		var b []byte
		for {
			c, ok := p.peekAt(0)
			if !ok || !isNameChar(c) {
				return tokName, string(b)
			}
			p.next()
			b = append(b, c)
		}
	}
	if p.lquote != "" && p.hasPrefix(p.lquote) {
		p.skip(len(p.lquote))
		return tokText, p.quoted()
	}
	p.next()
	return tokChar, string(c)
}

// quoted reads a quoted string whose opening delimiter has been
// consumed, returning it with that outer pair of quotes removed.
func (p *processor) quoted() string {
	var b []byte
	depth := 1
	for {
		if p.hasPrefix(p.rquote) {
			p.skip(len(p.rquote))
			if depth--; depth == 0 {
				return string(b)
			}
			b = append(b, p.rquote...)
			continue
		}
		if p.hasPrefix(p.lquote) {
			p.skip(len(p.lquote))
			depth++
			b = append(b, p.lquote...)
			continue
		}
		c, ok := p.next()
		if !ok {
			p.fatalf("ERROR: end of file in string")
		}
		b = append(b, c)
	}
}

// ---- expansion ----

func (p *processor) lookup(name string) *macro {
	if st := p.macros[name]; len(st) > 0 {
		return st[len(st)-1]
	}
	return nil
}

// define installs m as the current definition of name; push preserves
// the previous one (pushdef) instead of replacing it.
func (p *processor) define(name string, m *macro, push bool) {
	st := p.macros[name]
	if push || len(st) == 0 {
		p.macros[name] = append(st, m)
		return
	}
	st[len(st)-1] = m
}

// expand handles the name just scanned. It reports false when the name
// is not a macro call and must be treated as text. Otherwise the call's
// arguments are collected and its expansion is pushed back for
// rescanning; fn is non-nil when the expansion is a built-in's
// definition (defn) rather than text.
func (p *processor) expand(name string) (handled bool, fn *macro) {
	m := p.lookup(name)
	if m == nil {
		return false, nil
	}
	c, ok := p.peekAt(0)
	called := ok && c == '('
	if !called && m.blind {
		return false, nil
	}
	var args []argument
	if called {
		p.next()
		args = p.collect()
	}
	if m.fn != nil {
		if p.traceAll || p.trace[name] || p.trace[m.name] {
			fmt.Fprintf(p.rc.Err, "m4trace:%s:%d: -%s(%s)\n", p.file, p.line, name, joinArgs(args, "`", "'"))
		}
		text, fn := m.fn(p, m.name, args)
		p.pushback(text)
		return true, fn
	}
	p.pushback(p.substitute(name, m.text, args))
	return true, nil
}

// collect reads the arguments of a call whose '(' has been consumed.
// Unquoted leading white space of each argument is dropped; commas and
// the closing parenthesis count only outside nested parentheses, quotes
// and comments; macro calls are expanded as they are met and their
// expansions rescanned as part of the argument list.
func (p *processor) collect() []argument {
	if p.nesting++; p.nesting > maxNesting {
		p.fatalf("ERROR: macro calls nested too deeply (limit %d)", maxNesting)
	}
	defer func() { p.nesting-- }()
	var args []argument
	for {
		for {
			if !p.fill() {
				p.fatalf("ERROR: end of file in argument list")
			}
			if c, ok := p.peekAt(0); !ok || !isSpace(c) {
				break
			}
			p.next()
		}
		var b []byte
		var argFn *macro
		parens := 0
	arg:
		for {
			kind, s := p.token()
			switch kind {
			case tokEOF:
				p.fatalf("ERROR: end of file in argument list")
			case tokText:
				b = append(b, s...)
			case tokName:
				handled, fn := p.expand(s)
				if !handled {
					b = append(b, s...)
				} else if fn != nil && len(b) == 0 {
					argFn = fn
				}
			case tokChar:
				switch {
				case s == "(":
					parens++
				case s == ")" && parens > 0:
					parens--
				case s == ")":
					return append(args, argument{text: string(b), fn: argFn})
				case s == "," && parens == 0:
					args = append(args, argument{text: string(b), fn: argFn})
					break arg
				}
				b = append(b, s...)
			}
		}
	}
}

// substitute builds the expansion of a user macro: $0 is the name, $1-$9
// the arguments, $# their count, $* all of them comma-separated and $@
// the same with each one quoted.
func (p *processor) substitute(name, text string, args []argument) string {
	if !strings.Contains(text, "$") {
		return text
	}
	var b strings.Builder
	for i := 0; i < len(text); i++ {
		c := text[i]
		if c != '$' || i+1 == len(text) {
			b.WriteByte(c)
			continue
		}
		switch d := text[i+1]; {
		case d == '0':
			b.WriteString(name)
		case d >= '1' && d <= '9':
			if n := int(d - '0'); n <= len(args) {
				b.WriteString(args[n-1].text)
			}
		case d == '#':
			b.WriteString(strconv.Itoa(len(args)))
		case d == '*':
			b.WriteString(joinArgs(args, "", ""))
		case d == '@':
			b.WriteString(joinArgs(args, p.lquote, p.rquote))
		default:
			b.WriteByte(c)
			continue
		}
		i++
	}
	return b.String()
}

func joinArgs(args []argument, lq, rq string) string {
	var b strings.Builder
	for i, a := range args {
		if i > 0 {
			b.WriteByte(',')
		}
		b.WriteString(lq)
		b.WriteString(a.text)
		b.WriteString(rq)
	}
	return b.String()
}

// ---- output ----

// emit writes text to standard output. With -s, a #line directive is
// written at the start of any output line whose position no longer
// matches the input line (or file) being read.
func (p *processor) emit(s string) {
	if p.divnum < 0 {
		return
	}
	if p.divnum > 0 {
		if p.divnum < len(p.diversions) {
			p.diversions[p.divnum].WriteString(s)
		}
		return
	}
	if !p.sync {
		p.out.WriteString(s)
		return
	}
	for i := 0; i < len(s); i++ {
		if p.bol {
			p.bol = false
			if p.outSeq != p.fileSeq || p.outLine != p.line {
				fmt.Fprintf(p.out, "#line %d", p.line)
				if p.outSeq != p.fileSeq {
					fmt.Fprintf(p.out, " \"%s\"", p.file)
				}
				p.out.WriteByte('\n')
				p.outSeq, p.outLine = p.fileSeq, p.line
			}
		}
		p.out.WriteByte(s[i])
		if s[i] == '\n' {
			p.outLine++
			p.bol = true
		}
	}
}

func (p *processor) flushDiversions() {
	old := p.divnum
	p.divnum = 0
	for i := 1; i < len(p.diversions); i++ {
		if p.diversions[i].Len() == 0 {
			continue
		}
		s := p.diversions[i].String()
		p.diversions[i].Reset()
		p.emit(s)
	}
	p.divnum = old
}

func (p *processor) readNamedFile(name string) ([]byte, error) {
	f, err := p.rc.FS.Open(p.rc.Path(name))
	if err != nil {
		return nil, err
	}
	defer f.Close()
	b, err := io.ReadAll(f)
	if err != nil {
		return nil, err
	}
	return b, nil
}
