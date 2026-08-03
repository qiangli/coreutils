// Package findcmd implements find(1) per POSIX and the GNU findutils
// manual: walk each starting-point and evaluate an expression for
// every file.
//
// Options: -H, -L, -P (symlink handling for start points / everything;
// -P is the default).
// Supported tests: -name GLOB, -iname GLOB, -path GLOB, -type LETTERS
// (b c d f l p s, comma-separated per findutils), -atime/-ctime/-mtime
// [+-]N, -newer FILE, -size [+-]N[bcwkMGTP], -empty, -perm
// [-/]MODE (octal or chmod-style symbolic), -user/-group (name or
// numeric ID), -nouser, -nogroup, -links [+-]N.
// Actions: -print (default), -print0, -prune, -exec UTIL [ARG...] ;
// (also the POSIX batched form -exec UTIL [ARG...] {} +), and the
// interactive -ok UTIL [ARG...] ; which prompts on stderr and reads
// the reply from the invocation stdin.
// Operators: ( EXPR ), ! / -not, implicit and / -a / -and, -o / -or.
// Global options: -depth, -xdev, -maxdepth N, -mindepth N (positional
// anywhere, as GNU applies them; the GNU positional warning is not
// emitted). A "--" may close the leading -H/-L/-P options.
//
// Patterns (-name, -iname, -path) honor POSIX LC_CTYPE and LC_COLLATE
// category precedence. The C/POSIX locale is byte-oriented with ASCII
// classes; the provisioned de_DE ISO-8859-1 locale adds its alphabetic
// bytes and equivalence classes without depending on host locale archives.
// LC_MESSAGES likewise controls the affirmative response accepted by -ok.
//
// -exec/-ok spawn the named utility directly — that is find's
// upstream-documented purpose (the command-wrapper exception to the
// no-shell-out rule), argv is built verbatim with {} substitution and
// never concatenated through a shell. The one exception mirrors POSIX
// execvp exactly: if the utility exists and is executable but is not a
// recognized binary (ENOEXEC — e.g. a shebang-less script), it is retried
// once as `sh <file> [args...]`, just as GNU find does via execvp.
// -execdir, -okdir and -delete remain unsupported and fail with the
// standard contract error.
//
// Deviations from GNU worth knowing: traversal order is deterministic
// lexical (GNU uses directory order); parse/usage errors exit 2 per
// this repo's contract (GNU find exits 1); paths are printed with
// forward slashes on every platform; -iname folds ASCII case only,
// since no other byte has a case pair in the C locale.
package findcmd

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/qiangli/coreutils/pkg/ignore"
	"github.com/qiangli/coreutils/tool"
)

var cmd = &tool.Tool{
	Name:     "find",
	Synopsis: "Search for files in a directory hierarchy.",
	Usage:    "find [-H | -L | -P] [PATH...] [EXPRESSION]",
}

// Run is wired in init: a literal would create an initialization
// cycle (run's flag-error paths reference cmd).
func init() { cmd.Run = run; tool.Register(cmd) }

const helpText = `Usage: find [-H | -L | -P] [PATH...] [EXPRESSION]
Search for files in a directory hierarchy.

Default PATH is '.'; default expression is -print.

Options (before PATH):
  -P  never follow symlinks (default); -L  follow all symlinks;
  -H  follow symlinks given as PATH operands only
  --  end of options; PATH operands follow
Tests:
  -name GLOB, -iname GLOB   base name matches shell glob
  -path GLOB                whole path matches glob ('/' not special)
  -type [bcdflps][,...]     file type
  -atime/-ctime/-mtime [+-]N  accessed/changed/modified [more/less
                            than] N*24h ago
  -newer FILE               modified more recently than FILE
  -size [+-]N[bcwkMGTP]     size in 512-byte blocks (default), bytes, ...
  -empty                    empty regular file or directory
  -perm [-/]MODE            permission bits (octal or symbolic) match
                            exactly / all set (-) / any set (/)
  -user NAME, -group NAME   owned by user/group (name or numeric ID)
  -nouser, -nogroup         no known user/group owns the file
  -links [+-]N              link count
Actions:
  -print                    print path, newline-terminated (default)
  -print0                   print path, NUL-terminated
  -prune                    do not descend into matched directories
  -exec UTIL [ARG...] ;     run UTIL, {} replaced by the current path;
                            true when it exits 0
  -exec UTIL [ARG...] {} +  run UTIL with batches of paths appended
  -ok UTIL [ARG...] ;       like -exec ; but ask on stderr first,
                            reading the y/n reply from standard input
Operators (decreasing precedence):
  ( EXPR )   ! EXPR   -not EXPR   EXPR1 [-a] EXPR2   EXPR1 -o EXPR2
Global options:
  -depth                    process directory contents before the
                            directory itself
  -xdev                     do not descend into other file systems
  -maxdepth N, -mindepth N  depth limits (start point is depth 0)

-execdir, -okdir and -delete are not supported by pure-Go coreutils.
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

	// -H/-L/-P precede path operands; the last one specified applies.
	// A "--" ends the leading options (POSIX Utility Syntax Guideline
	// 10, and what GNU find does): it is consumed, and everything after
	// it is read as start points and expression exactly as before. It
	// does not force a following "-foo" to be a path — GNU keeps the
	// expression scan unchanged there, and so do we.
	follow := byte('P')
	i := 0
leading:
	for i < len(args) {
		switch args[i] {
		case "-H", "-L", "-P":
			follow = args[i][1]
			i++
		case "--":
			i++
			break leading
		default:
			break leading
		}
	}

	// Start points are everything before the first expression token.
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

	w := &walker{
		rc: rc, e: root,
		maxDepth: p.maxDepth, minDepth: p.minDepth,
		depthFirst: p.depthFirst, xdev: p.xdev,
		follow: follow,
		stdin:  bufio.NewReader(rc.In),
		owners: newOwnerCache(),
		locale: findLocaleFromEnv(rc.Env),
	}
	if agentic {
		w.matcher = ignore.New(rc.Dir)
	}
	for _, sp := range paths {
		w.walkRoot(sp)
	}
	// -exec ... {} + accumulates paths; run whatever is still pending.
	for _, e := range p.execs {
		e.flush(w)
	}
	// Transparency: announce what --agentic hid (stderr only).
	if n := w.matcher.Hidden(); n > 0 {
		fmt.Fprintf(rc.Err, "%s: --agentic skipped %d ignored path(s) (run without --agentic to include them)\n", cmd.Name, n)
	}
	// A failed write to standard output is an error like any other: one
	// diagnostic on stderr and a non-zero status, never a silent exit 0
	// (find > /dev/full must not look like success).
	if w.writeErr != nil {
		fmt.Fprintf(rc.Err, "%s: write error: %v\n", cmd.Name, w.writeErr)
		return 1
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
	rc         *tool.RunContext
	toks       []string
	i          int
	maxDepth   int // -1 = unset
	minDepth   int
	depthFirst bool
	xdev       bool
	hasAction  bool
	now        time.Time
	execs      []*execExpr // for the end-of-run {} + flush
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
	case "-depth":
		p.depthFirst = true
		return trueExpr{}, nil
	case "-xdev":
		if !haveDev {
			return nil, &notSupportedErr{"-xdev on " + runtime.GOOS + " (no device identity)"}
		}
		p.xdev = true
		return trueExpr{}, nil
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
	case "-mtime", "-atime", "-ctime":
		a, err := p.arg(t)
		if err != nil {
			return nil, err
		}
		cmp, n, err := parseSignedNum(a)
		if err != nil {
			return nil, fmt.Errorf("invalid argument '%s' to %s", a, t)
		}
		sel := t[1] // 'm', 'a' or 'c'
		if sel == 'a' && !haveAtime || sel == 'c' && !haveCtime {
			return nil, &notSupportedErr{t + " on " + runtime.GOOS}
		}
		return &timeExpr{cmp: cmp, n: n, now: p.now, sel: sel}, nil
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
	case "-links":
		a, err := p.arg(t)
		if err != nil {
			return nil, err
		}
		if !haveSysStat {
			return nil, &notSupportedErr{"-links on " + runtime.GOOS + " (no link count)"}
		}
		cmp, n, err := parseSignedNum(a)
		if err != nil {
			return nil, fmt.Errorf("invalid argument '%s' to -links", a)
		}
		return &linksExpr{cmp: cmp, n: n}, nil
	case "-perm":
		a, err := p.arg(t)
		if err != nil {
			return nil, err
		}
		return parsePerm(a)
	case "-user", "-group":
		a, err := p.arg(t)
		if err != nil {
			return nil, err
		}
		if !haveSysStat {
			return nil, &notSupportedErr{t + " on " + runtime.GOOS + " (no unix uid/gid)"}
		}
		id, err := lookupOwner(a, t == "-group")
		if err != nil {
			return nil, err
		}
		return &ownerExpr{id: id, group: t == "-group"}, nil
	case "-nouser", "-nogroup":
		if !haveSysStat {
			return nil, &notSupportedErr{t + " on " + runtime.GOOS + " (no unix uid/gid)"}
		}
		return &noOwnerExpr{group: t == "-nogroup"}, nil
	case "-exec", "-ok":
		tmpl, plus, err := p.execArgs(t)
		if err != nil {
			return nil, err
		}
		p.hasAction = true
		e := &execExpr{name: t, tmpl: tmpl, plus: plus, ok: t == "-ok"}
		p.execs = append(p.execs, e)
		return e, nil
	case "-execdir", "-okdir", "-delete":
		return nil, &notSupportedErr{t + " (would execute in-directory or delete files); use -exec/-ok"}
	case ",":
		return nil, &notSupportedErr{"the ',' operator"}
	default:
		if strings.HasPrefix(t, "-") {
			return nil, fmt.Errorf("unknown predicate '%s'", t)
		}
		return nil, fmt.Errorf("paths must precede expression: '%s'", t)
	}
}

// execArgs collects the utility name and arguments of -exec/-ok up to
// the terminating ';' (or, for -exec, a standalone '{}' immediately
// followed by '+', per POSIX). The terminator is consumed; the returned
// template includes the trailing '{}' in the + form.
//
// POSIX grammar for the batched form is strict: a '+' terminates the
// primary only when it immediately follows an argument that is exactly
// "{}", and that standalone "{}" is the single aggregation point — there
// must be a preceding utility and no other "{}" (standalone or embedded)
// may appear among the fixed arguments. A '+' anywhere else is an
// ordinary argument of the ';' form. Violations are rejected rather than
// silently passing literal braces to the child.
func (p *parser) execArgs(name string) (tmpl []string, plus bool, err error) {
	start := p.i
	for p.i < len(p.toks) {
		t := p.toks[p.i]
		if t == ";" {
			p.i++
			if len(tmpl) == 0 {
				return nil, false, fmt.Errorf("missing argument to '%s'", name)
			}
			return tmpl, false, nil
		}
		// Batched '{} +' terminator (only -exec has it; -ok does not).
		if name == "-exec" && t == "+" && p.i > start && p.toks[p.i-1] == "{}" {
			p.i++ // consume '+'
			// tmpl currently ends with the standalone "{}"; everything
			// before it is the utility plus its fixed arguments.
			fixed := tmpl[:len(tmpl)-1]
			if len(fixed) == 0 {
				return nil, false, fmt.Errorf("missing argument to '%s'", name)
			}
			for _, a := range fixed {
				if strings.Contains(a, "{}") {
					return nil, false, fmt.Errorf(
						"only one instance of '{}' is supported with -exec ... +, immediately before the terminating '+'")
				}
			}
			return tmpl, true, nil
		}
		tmpl = append(tmpl, t)
		p.i++
	}
	return nil, false, fmt.Errorf("missing argument to '%s' (no terminating ';' or '{} +')", name)
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

// parsePerm parses -perm's argument: an octal or chmod-style symbolic
// mode, optionally prefixed with '-' (all given bits set) or the GNU
// '/' (any given bit set); no prefix means the bits match exactly.
func parsePerm(a string) (expr, error) {
	s := a
	var how byte
	if s != "" && (s[0] == '-' || s[0] == '/') {
		how = s[0]
		s = s[1:]
	}
	bits, err := parsePermBits(s)
	if err != nil {
		return nil, fmt.Errorf("invalid mode '%s'", a)
	}
	return &permExpr{how: how, bits: bits}, nil
}

// parsePermBits resolves an octal or symbolic mode against an
// all-zeros starting mode with no umask, as POSIX specifies for find.
//
// The symbolic grammar is chmod's in full, because that is what POSIX
// says -perm's operand is: who-list, operator, and either a permission
// list or a permcopy ("-perm -g=u"). Clauses apply in order to the mode
// being built, so '-' clears and '=' replaces rather than being ignored.
// Two letters can only be resolved against a real file, which the parser
// does not have: 's' contributes set-user/set-group-ID for the classes
// named, and 'X' contributes the execute bit only when one is already
// set in the mode so far (a zero template is never a directory).
func parsePermBits(s string) (uint32, error) {
	if s == "" {
		return 0, errors.New("empty mode")
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
			return 0, errors.New("invalid octal mode")
		}
		return uint32(v), nil
	}

	var bits uint32
	for _, clause := range strings.Split(s, ",") {
		i := 0
		var who uint32 // bitmask: 4=u 2=g 1=o
	wholoop:
		for ; i < len(clause); i++ {
			switch clause[i] {
			case 'u':
				who |= 4
			case 'g':
				who |= 2
			case 'o':
				who |= 1
			case 'a':
				who |= 7
			default:
				break wholoop
			}
		}
		if who == 0 {
			who = 7 // no who: 'a', with no umask (unlike chmod)
		}
		if i >= len(clause) {
			return 0, errors.New("no operator")
		}
		for i < len(clause) {
			op := clause[i]
			if op != '+' && op != '-' && op != '=' {
				return 0, errors.New("bad operator")
			}
			i++
			perm, special, n, err := parsePermList(clause[i:], who, bits)
			if err != nil {
				return 0, err
			}
			i += n
			val := spreadPerm(who, perm) | special
			switch op {
			case '+':
				bits |= val
			case '-':
				bits &^= val
			case '=':
				// '=' replaces the named classes' bits, so an earlier
				// clause in the same mode string can be overridden.
				bits = bits&^permMask(who) | val
			}
		}
	}
	return bits, nil
}

// parsePermList reads one permission list (or permcopy) from the head of
// s, stopping at the next operator. bits is the mode built so far, which
// a permcopy reads from and 'X' consults. It returns the who-relative
// permission bits, the absolute special bits, and how much of s it used.
func parsePermList(s string, who, bits uint32) (perm, special uint32, n int, err error) {
	// permcopy: exactly one of u/g/o standing alone before the next
	// operator or the end of the clause ("g=u", "-perm -u+g").
	if len(s) > 0 && (s[0] == 'u' || s[0] == 'g' || s[0] == 'o') &&
		(len(s) == 1 || s[1] == '+' || s[1] == '-' || s[1] == '=') {
		switch s[0] {
		case 'u':
			perm = bits >> 6 & 7
		case 'g':
			perm = bits >> 3 & 7
		default:
			perm = bits & 7
		}
		return perm, 0, 1, nil
	}
	for ; n < len(s); n++ {
		switch s[n] {
		case 'r':
			perm |= 4
		case 'w':
			perm |= 2
		case 'x':
			perm |= 1
		case 'X':
			// chmod's conditional execute: a directory, or a mode that
			// already carries an execute bit. There is no file here and
			// the template starts at zero, so only the latter can hold.
			if bits&0o111 != 0 {
				perm |= 1
			}
		case 's':
			if who&4 != 0 {
				special |= 0o4000
			}
			if who&2 != 0 {
				special |= 0o2000
			}
		case 't':
			special |= 0o1000
		case '+', '-', '=':
			return perm, special, n, nil
		default:
			return 0, 0, 0, errors.New("bad permission letter")
		}
	}
	return perm, special, n, nil
}

// spreadPerm places a 3-bit permission value in each named class's slot.
func spreadPerm(who, perm uint32) uint32 {
	var v uint32
	if who&4 != 0 {
		v |= perm << 6
	}
	if who&2 != 0 {
		v |= perm << 3
	}
	if who&1 != 0 {
		v |= perm
	}
	return v
}

// permMask is every bit '=' clears for the named classes: their
// permission bits plus the set-ID bit each owns. The sticky bit belongs
// to no single class, so '=' leaves it alone.
func permMask(who uint32) uint32 {
	m := spreadPerm(who, 7)
	if who&4 != 0 {
		m |= 0o4000
	}
	if who&2 != 0 {
		m |= 0o2000
	}
	return m
}

// lookupOwner resolves -user/-group's argument: a name first, then a
// decimal ID per POSIX when no such name exists.
func lookupOwner(a string, group bool) (uint32, error) {
	var idStr string
	if group {
		if g, err := user.LookupGroup(a); err == nil {
			idStr = g.Gid
		}
	} else {
		if u, err := user.Lookup(a); err == nil {
			idStr = u.Uid
		}
	}
	if idStr == "" {
		idStr = a
	}
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		kind := "user"
		if group {
			kind = "group"
		}
		return 0, fmt.Errorf("'%s' is not the name of a known %s", a, kind)
	}
	return uint32(id), nil
}

// ---------------------------------------------------------------------------
// expression evaluation

type fctx struct {
	path   string // display path (operand-rooted, forward slashes)
	osPath string
	// info is the effective FileInfo per the -H/-L/-P follow mode
	// (for a dangling symlink under -L/-H it falls back to the link's
	// own lstat data, per POSIX).
	info   fs.FileInfo
	pruned bool
	w      *walker
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
	term := "\n"
	if p.nul {
		term = "\x00"
	}
	if _, err := io.WriteString(c.w.rc.Out, c.path+term); err != nil {
		c.w.noteWriteErr(err)
	}
	return true
}

type pruneExpr struct{}

func (pruneExpr) eval(c *fctx) bool {
	if c.info.IsDir() {
		c.pruned = true
	}
	return true
}

type nameExpr struct {
	pat  string
	fold bool
}

func (n *nameExpr) eval(c *fctx) bool {
	return fnmatchLocale(n.pat, baseName(c.path), n.fold, c.w.locale)
}

// baseName is the last component of the operand-rooted path, which is
// what -name matches against. It has to come from the path as find
// names it, not from the resolved filesystem path: for a start point
// GNU and BSD compare the operand's own final component (with trailing
// slashes stripped), so `find . -name .` matches while
// `find . -name <cwd-basename>` does not. Resolving first got both
// backwards. filepath.Base also copes with a "\" operand on Windows.
func baseName(p string) string {
	return filepath.Base(p)
}

type pathExpr struct{ pat string }

func (p *pathExpr) eval(c *fctx) bool {
	// GNU -path: matched against the path as printed; wildcards and
	// the match in general do not treat '/' specially.
	return fnmatchLocale(p.pat, c.path, false, c.w.locale)
}

type typeExpr struct{ letters string }

func (t *typeExpr) eval(c *fctx) bool {
	m := c.info.Mode()
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

// timeExpr is -mtime/-atime/-ctime: file time in whole 24h periods
// ago, remainder discarded per POSIX.
type timeExpr struct {
	cmp byte
	n   int64
	now time.Time
	sel byte // 'm', 'a', 'c'
}

func (m *timeExpr) eval(c *fctx) bool {
	var ft time.Time
	switch m.sel {
	case 'm':
		ft = c.info.ModTime()
	case 'a':
		t, err := fileAtime(c.info)
		if err != nil {
			c.w.reportErr(c.path, err)
			return false
		}
		ft = t
	case 'c':
		t, err := fileCtime(c.info)
		if err != nil {
			c.w.reportErr(c.path, err)
			return false
		}
		ft = t
	}
	days := int64(m.now.Sub(ft) / (24 * time.Hour))
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
	return c.info.ModTime().After(n.ref)
}

type sizeExpr struct {
	cmp  byte
	n    int64
	unit int64
}

func (s *sizeExpr) eval(c *fctx) bool {
	v := (c.info.Size() + s.unit - 1) / s.unit // GNU rounds up
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
	if c.info.IsDir() {
		ents, err := os.ReadDir(c.osPath)
		if err != nil {
			c.w.reportErr(c.path, err)
			return false
		}
		return len(ents) == 0
	}
	return c.info.Mode().IsRegular() && c.info.Size() == 0
}

type permExpr struct {
	how  byte // 0 exact, '-' all bits, '/' any bit
	bits uint32
}

func (p *permExpr) eval(c *fctx) bool {
	fb := fileModeBits(c.info.Mode())
	switch p.how {
	case '-':
		return fb&p.bits == p.bits
	case '/':
		if p.bits == 0 { // GNU: -perm /000 matches everything
			return true
		}
		return fb&p.bits != 0
	default:
		return fb == p.bits
	}
}

// fileModeBits maps a Go FileMode onto the POSIX 07777 bit layout.
func fileModeBits(m fs.FileMode) uint32 {
	b := uint32(m.Perm())
	if m&fs.ModeSetuid != 0 {
		b |= 0o4000
	}
	if m&fs.ModeSetgid != 0 {
		b |= 0o2000
	}
	if m&fs.ModeSticky != 0 {
		b |= 0o1000
	}
	return b
}

type linksExpr struct {
	cmp byte
	n   int64
}

func (l *linksExpr) eval(c *fctx) bool {
	nl, ok := fileNlink(c.info)
	if !ok {
		return false
	}
	switch l.cmp {
	case '+':
		return int64(nl) > l.n
	case '-':
		return int64(nl) < l.n
	default:
		return int64(nl) == l.n
	}
}

type ownerExpr struct {
	id    uint32
	group bool
}

func (o *ownerExpr) eval(c *fctx) bool {
	var id uint32
	var ok bool
	if o.group {
		id, ok = fileGID(c.info)
	} else {
		id, ok = fileUID(c.info)
	}
	return ok && id == o.id
}

// noOwnerExpr is -nouser/-nogroup: the file's numeric ID has no name.
type noOwnerExpr struct{ group bool }

func (n *noOwnerExpr) eval(c *fctx) bool {
	var id uint32
	var ok bool
	if n.group {
		id, ok = fileGID(c.info)
	} else {
		id, ok = fileUID(c.info)
	}
	if !ok {
		return false
	}
	return !c.w.owners.nameExists(id, n.group)
}

// ownerCache memoizes uid/gid → "a name resolves for this ID" lookups.
// It is scoped to one find invocation (one walker), never package-global:
// a transient name-service failure must not be cached as "no such name"
// and then leak into an unrelated later run. The mutex keeps it safe if a
// host ever evaluates the tree from more than one goroutine.
type ownerCache struct {
	mu   sync.Mutex
	uids map[uint32]bool
	gids map[uint32]bool
}

func newOwnerCache() *ownerCache {
	return &ownerCache{uids: map[uint32]bool{}, gids: map[uint32]bool{}}
}

// nameExists reports whether id resolves to a user (or group) name,
// memoizing the result for the lifetime of this cache.
func (oc *ownerCache) nameExists(id uint32, group bool) bool {
	oc.mu.Lock()
	defer oc.mu.Unlock()
	cache := oc.uids
	if group {
		cache = oc.gids
	}
	if known, seen := cache[id]; seen {
		return known
	}
	var err error
	if group {
		_, err = user.LookupGroupId(strconv.FormatUint(uint64(id), 10))
	} else {
		_, err = user.LookupId(strconv.FormatUint(uint64(id), 10))
	}
	known := err == nil
	cache[id] = known
	return known
}

// ---------------------------------------------------------------------------
// -exec / -ok

// Batch limits for the {} + form, in the ballpark of xargs defaults.
const (
	execBatchArgs  = 4096
	execBatchBytes = 128 * 1024
)

type execExpr struct {
	name string // "-exec" or "-ok", for diagnostics
	tmpl []string
	plus bool
	ok   bool

	batch      []string // pending paths ({} + form)
	batchBytes int
}

func (e *execExpr) eval(c *fctx) bool {
	if e.plus {
		// POSIX: the batched form always evaluates true; a child's
		// non-zero exit only makes find itself exit non-zero.
		e.batch = append(e.batch, c.path)
		e.batchBytes += len(c.path) + 1
		if len(e.batch) >= execBatchArgs || e.batchBytes >= execBatchBytes {
			e.flush(c.w)
		}
		return true
	}
	argv := make([]string, len(e.tmpl))
	for i, a := range e.tmpl {
		argv[i] = strings.ReplaceAll(a, "{}", c.path)
	}
	if e.ok && !c.w.confirm(argv[0], c.path) {
		return false
	}
	code, err := c.w.runArgv(argv, e.ok)
	if err != nil {
		c.w.reportErr(argv[0], err)
		return false
	}
	return code == 0
}

// flush runs the pending {} + batch, appending the collected paths in
// place of the trailing '{}'.
func (e *execExpr) flush(w *walker) {
	if !e.plus || len(e.batch) == 0 {
		return
	}
	argv := make([]string, 0, len(e.tmpl)-1+len(e.batch))
	argv = append(argv, e.tmpl[:len(e.tmpl)-1]...)
	argv = append(argv, e.batch...)
	e.batch, e.batchBytes = nil, 0
	code, err := w.runArgv(argv, false)
	if err != nil {
		w.reportErr(argv[0], err)
		return
	}
	if code != 0 {
		w.errored = true
	}
}

// confirm writes the -ok prompt to stderr and reads one reply line
// from the invocation's standard input; only a leading y/Y affirms.
func (w *walker) confirm(util, path string) bool {
	fmt.Fprintf(w.rc.Err, "< %s ... %s > ? ", util, path)
	line, err := w.stdin.ReadString('\n')
	if err != nil && line == "" {
		return false // EOF: not affirmative
	}
	line = strings.TrimSpace(line)
	if line == "" {
		return false
	}
	if w.locale.messagesGerman {
		return line[0] == 'j' || line[0] == 'J'
	}
	return line[0] == 'y' || line[0] == 'Y'
}

// runArgv spawns argv verbatim — no shell, no operand concatenation.
// The child runs in the invocation's working directory with its
// environment; -ok children get no stdin (GNU redirects it from the
// null device so the utility cannot eat the reply stream).
func (w *walker) runArgv(argv []string, isOK bool) (int, error) {
	path := lookCommand(w.rc, argv[0])
	if path == "" {
		return 0, errors.New("command not found")
	}
	code, err := w.spawn(path, argv[1:], isOK)
	// POSIX execvp semantics: a file that exists and is executable but is
	// not a recognized binary (ENOEXEC — e.g. a shebang-less shell script)
	// is retried through the shell as `sh <file> [args...]`. GNU find gets
	// this for free because it execs via execvp; Go's exec does a raw
	// execve, so reproduce the one documented fallback explicitly. The
	// shell path is compiled in (unix only); on Windows there is no ENOEXEC
	// retry and shellPath is "".
	if err != nil && isExecFormatError(err) {
		if sh := shellPath(); sh != "" {
			shArgv := append([]string{path}, argv[1:]...)
			return w.spawn(sh, shArgv, isOK)
		}
	}
	return code, err
}

// spawn runs an already-resolved executable path with the given arguments
// and maps its termination onto find's (exit-code, error) contract: a
// clean or non-zero exit yields (code, nil), a signal death yields
// (128+signal, nil), and a failure to start the process yields (0, err)
// with err carrying the raw exec error (so runArgv can detect ENOEXEC).
func (w *walker) spawn(path string, args []string, isOK bool) (int, error) {
	ctx := w.rc.Ctx
	if ctx == nil {
		ctx = context.Background()
	}
	c := exec.CommandContext(ctx, path, args...)
	c.Dir = w.rc.Dir
	c.Env = w.rc.Env
	if !isOK {
		c.Stdin = w.rc.In
	}
	c.Stdout = w.rc.Out
	c.Stderr = w.rc.Err
	err := c.Run()
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		if code := ee.ExitCode(); code >= 0 {
			return code, nil
		}
		// Negative ExitCode means the child was terminated by a signal;
		// report 128+signal per POSIX shell $? convention (portable
		// helper: real mapping on unix, always false on windows).
		if code, ok := signaledExitCode(ee.ProcessState); ok {
			return code, nil
		}
		return 128, nil
	}
	if err != nil {
		return 0, err
	}
	return 0, nil
}

// lookCommand resolves a utility name against the invocation PATH
// (rc.Env). A name containing a path separator resolves against the
// working directory (rc.Dir) and PATH is never consulted. Otherwise
// each PATH element is searched; a zero-length element means the working
// directory, per POSIX. An unset PATH falls back to the platform default
// search path, while an explicitly empty PATH ("PATH=") is one
// zero-length element — the working directory only — so the two are kept
// distinct.
func lookCommand(rc *tool.RunContext, name string) string {
	isWindows := runtime.GOOS == "windows"
	var exts []string
	if isWindows {
		pathExt := rc.Getenv("PATHEXT")
		if pathExt == "" {
			exts = []string{".com", ".exe", ".bat", ".cmd"}
		} else {
			for _, e := range filepath.SplitList(pathExt) {
				if e != "" {
					exts = append(exts, strings.ToLower(e))
				}
			}
		}
	}
	checkFile := func(path string) bool {
		fi, err := os.Stat(path)
		if err != nil || fi.IsDir() {
			return false
		}
		return isWindows || fi.Mode()&0o111 != 0
	}
	resolve := func(cand string) string {
		if !filepath.IsAbs(cand) {
			cand = rc.Path(cand)
		}
		if checkFile(cand) {
			return cand
		}
		for _, ext := range exts {
			if checkFile(cand + ext) {
				return cand + ext
			}
		}
		return ""
	}

	if strings.ContainsAny(name, `/\`) {
		return resolve(name)
	}

	pathValue, present := lookupEnv(rc.Env, "PATH")
	if !present {
		pathValue = defaultCommandPath()
	}
	for _, dir := range commandSearchPath(pathValue) {
		cand := name // zero-length element: search the working directory
		if dir != "" {
			cand = filepath.Join(dir, name)
		}
		if got := resolve(cand); got != "" {
			return got
		}
	}
	return ""
}

// commandSearchPath splits a PATH value into search prefixes. POSIX makes
// a zero-length prefix mean the working directory, and a wholly empty
// PATH is one zero-length prefix — not "nowhere to look" — so it searches
// the working directory only. filepath.SplitList("") yields no elements,
// hence the explicit case.
func commandSearchPath(value string) []string {
	dirs := filepath.SplitList(value)
	if len(dirs) == 0 {
		return []string{""}
	}
	return dirs
}

// lookupEnv reports a variable's value and whether it is present at all,
// so an unset PATH (default search path) is distinguishable from an
// explicitly empty one (working directory). Last assignment wins.
func lookupEnv(env []string, key string) (string, bool) {
	prefix := key + "="
	for i := len(env) - 1; i >= 0; i-- {
		if strings.HasPrefix(env[i], prefix) {
			return env[i][len(prefix):], true
		}
	}
	return "", false
}

// ---------------------------------------------------------------------------
// traversal

type walker struct {
	rc         *tool.RunContext
	e          expr
	maxDepth   int // -1 = unlimited
	minDepth   int
	depthFirst bool
	xdev       bool
	follow     byte // 'P', 'H' or 'L'
	errored    bool
	writeErr   error // first failed write to standard output
	stdin      *bufio.Reader
	matcher    *ignore.Matcher // --agentic path filter (nil = off, skips nothing)
	owners     *ownerCache     // per-invocation -nouser/-nogroup lookup cache
	locale     findLocale      // POSIX category settings for matching and -ok
}

func (w *walker) reportErr(display string, err error) {
	w.errored = true
	fmt.Fprintf(w.rc.Err, "%s: '%s': %s\n", cmd.Name, display, pathErrMsg(err))
}

// noteWriteErr records the first standard-output write failure. The walk
// continues (a later path may still be wanted by -exec or -ok, and GNU
// reports the failure once, at exit) but the run can no longer succeed.
func (w *walker) noteWriteErr(err error) {
	if w.writeErr == nil {
		w.writeErr = err
	}
}

func (w *walker) walkRoot(operand string) {
	root := w.rc.Path(operand)
	lst, err := os.Lstat(root)
	if err != nil {
		w.reportErr(operand, err)
		return
	}
	info := lst
	if w.follow != 'P' && lst.Mode()&fs.ModeSymlink != 0 {
		info = w.followLink(operand, root, lst)
	}
	var dev uint64
	if w.xdev {
		dev, _ = fileDev(info)
	}
	w.visit(operand, root, info, 0, nil, dev)
}

// followLink stats through a symlink. A dangling link silently falls
// back to the link's own data per POSIX (-type then matches 'l');
// any other failure (ELOOP, permission) is diagnosed.
func (w *walker) followLink(disp, osPath string, lst fs.FileInfo) fs.FileInfo {
	st, err := os.Stat(osPath)
	if err == nil {
		return st
	}
	if !errors.Is(err, fs.ErrNotExist) {
		w.reportErr(disp, err)
	}
	return lst
}

// visit evaluates one file and recurses into directories. ancestors
// carries the FileInfo of every directory on the current path when
// symlinks are followed, for loop detection.
func (w *walker) visit(disp, osPath string, info fs.FileInfo, depth int, ancestors []fs.FileInfo, dev uint64) {
	descend := info.IsDir() && (w.maxDepth < 0 || depth < w.maxDepth)
	if descend && w.xdev {
		if d, ok := fileDev(info); ok && d != dev {
			descend = false // evaluate the mount point itself, never enter it
		}
	}
	if descend && w.follow == 'L' {
		// Loop detection: following symlinks can revisit an ancestor.
		for _, a := range ancestors {
			if os.SameFile(a, info) {
				w.reportErr(disp, errors.New("file system loop detected"))
				return
			}
		}
	}

	if !w.depthFirst && depth >= w.minDepth {
		c := &fctx{path: disp, osPath: osPath, info: info, w: w}
		w.e.eval(c)
		if c.pruned {
			descend = false
		}
	}

	if descend {
		ents, err := os.ReadDir(osPath) // sorted: deterministic lexical order
		if err != nil {
			w.reportErr(disp, err)
		}
		var childAnc []fs.FileInfo
		if w.follow == 'L' {
			childAnc = append(ancestors, info)
		}
		for _, ent := range ents {
			childOS := filepath.Join(osPath, ent.Name())
			childDisp := disp + "/" + ent.Name()
			if strings.HasSuffix(disp, "/") {
				childDisp = disp + ent.Name()
			}
			// --agentic: prune .gitignore'd / noise paths (never the start point).
			if w.matcher.Skip(childOS, ent.IsDir()) {
				continue
			}
			clst, err := ent.Info()
			if err != nil {
				w.reportErr(childDisp, err)
				continue
			}
			cinfo := clst
			if w.follow == 'L' && clst.Mode()&fs.ModeSymlink != 0 {
				cinfo = w.followLink(childDisp, childOS, clst)
			}
			w.visit(childDisp, childOS, cinfo, depth+1, childAnc, dev)
		}
	}

	if w.depthFirst && depth >= w.minDepth {
		c := &fctx{path: disp, osPath: osPath, info: info, w: w}
		w.e.eval(c)
	}
}

// pathErrMsg strips Go's "lstat <path>: " prefix so diagnostics read
// like GNU's "find: '<name>': <reason>".
func pathErrMsg(err error) string {
	return tool.SysErrString(err)
}

// ---------------------------------------------------------------------------
// fnmatch: POSIX pattern matching (XCU 2.13) for -name/-iname/-path.
// Unlike path.Match, '*' and '?' also match '/' (GNU -path rule),
// backslash escapes the next character, and [!...] negation is accepted.
//
// Matching is byte-oriented because that is what the C/POSIX locale
// means: a character is a byte, so '?' matches one byte of a multi-byte
// sequence, ranges compare byte values, and [[:alpha:]] and friends are
// the C locale's ASCII sets. The result is therefore the same whatever
// LC_ALL, LC_CTYPE, LC_COLLATE or LANG say — the determinism the agent
// contract requires, and what GNU find does in the POSIX locale.

func fnmatch(pattern, name string, fold bool) bool {
	return fnmatchLocale(pattern, name, fold, findLocale{})
}

func fnmatchLocale(pattern, name string, fold bool, loc findLocale) bool {
	for len(pattern) > 0 {
		switch pattern[0] {
		case '*':
			for len(pattern) > 0 && pattern[0] == '*' {
				pattern = pattern[1:]
			}
			if len(pattern) == 0 {
				return true
			}
			for i := 0; i <= len(name); i++ {
				if fnmatchLocale(pattern, name[i:], fold, loc) {
					return true
				}
			}
			return false
		case '?':
			if len(name) == 0 {
				return false
			}
			pattern, name = pattern[1:], name[1:]
		case '[':
			if len(name) == 0 {
				return false
			}
			matched, rest, valid := matchClassLocale(pattern, name[0], fold, loc)
			if !valid {
				// unmatched '[' is a literal
				if !eqByte('[', name[0], fold) {
					return false
				}
				pattern, name = pattern[1:], name[1:]
				continue
			}
			if !matched {
				return false
			}
			pattern, name = rest, name[1:]
		case '\\':
			if len(pattern) >= 2 {
				pattern = pattern[1:]
			}
			if len(name) == 0 || !eqByte(pattern[0], name[0], fold) {
				return false
			}
			pattern, name = pattern[1:], name[1:]
		default:
			if len(name) == 0 || !eqByte(pattern[0], name[0], fold) {
				return false
			}
			pattern, name = pattern[1:], name[1:]
		}
	}
	return len(name) == 0
}

// eqByte compares one byte, folding case for -iname. Folding is ASCII
// only: in the C locale no other byte has a case pair.
func eqByte(a, b byte, fold bool) bool {
	if a == b {
		return true
	}
	return fold && lowerASCII(a) == lowerASCII(b)
}

func lowerASCII(c byte) byte {
	if c >= 'A' && c <= 'Z' {
		return c + 'a' - 'A'
	}
	return c
}

func flipASCII(c byte) byte {
	switch {
	case c >= 'A' && c <= 'Z':
		return c + 'a' - 'A'
	case c >= 'a' && c <= 'z':
		return c - 'a' + 'A'
	}
	return c
}

// classFns are the C locale's character classes: ASCII only, so a byte
// of a UTF-8 sequence is never a letter, a digit or printable.
var classFns = map[string]func(byte) bool{
	"alpha":  isAlphaC,
	"digit":  isDigitC,
	"alnum":  func(c byte) bool { return isAlphaC(c) || isDigitC(c) },
	"upper":  func(c byte) bool { return c >= 'A' && c <= 'Z' },
	"lower":  func(c byte) bool { return c >= 'a' && c <= 'z' },
	"space":  func(c byte) bool { return c == ' ' || (c >= '\t' && c <= '\r') },
	"blank":  func(c byte) bool { return c == ' ' || c == '\t' },
	"punct":  func(c byte) bool { return c > ' ' && c < 0x7f && !isAlphaC(c) && !isDigitC(c) },
	"cntrl":  func(c byte) bool { return c < ' ' || c == 0x7f },
	"graph":  func(c byte) bool { return c > ' ' && c < 0x7f },
	"print":  func(c byte) bool { return c >= ' ' && c < 0x7f },
	"xdigit": func(c byte) bool { return isDigitC(c) || (lowerASCII(c) >= 'a' && lowerASCII(c) <= 'f') },
}

func isAlphaC(c byte) bool {
	return c >= 'A' && c <= 'Z' || c >= 'a' && c <= 'z'
}

func isDigitC(c byte) bool { return c >= '0' && c <= '9' }

// matchClass parses one bracket expression at the head of p and tests
// the byte c against it. valid=false means the '[' had no closing ']'
// and must be treated as a literal.
func matchClass(p string, c byte, fold bool) (matched bool, rest string, valid bool) {
	return matchClassLocale(p, c, fold, findLocale{})
}

func matchClassLocale(p string, c byte, fold bool, loc findLocale) (matched bool, rest string, valid bool) {
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
		// POSIX named bracket elements: [:class:], [=equivalence=], and
		// [.collating-symbol.]. In the C locale an equivalence class and a
		// collating symbol each denote their single byte; the surrounding
		// syntax is one element of this outer bracket expression.
		if p[i] == '[' && i+1 < len(p) && (p[i+1] == ':' || p[i+1] == '=' || p[i+1] == '.') {
			kind := p[i+1]
			j := i + 2
			for j+1 < len(p) && !(p[j] == kind && p[j+1] == ']') {
				j++
			}
			if j+1 >= len(p) {
				return false, "", false
			}
			element := p[i+2 : j]
			switch kind {
			case ':':
				if fn, ok := classFns[element]; ok {
					// -iname: either case of the byte satisfying the class
					// is a match, so [[:upper:]] finds a lowercase name.
					if fn(c) || (fold && fn(flipASCII(c))) {
						matched = true
					}
					if loc.ctypeGerman && germanClassMatch(element, c, fold) {
						matched = true
					}
				}
			case '=', '.':
				if len(element) == 1 && (eqByte(element[0], c, fold) ||
					(kind == '=' && loc.collateGerman && germanEquivalent(element[0], c))) {
					matched = true
				}
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
		b := c
		if fold {
			b, lo, hi = lowerASCII(c), lowerASCII(lo), lowerASCII(hi)
		}
		if lo <= b && b <= hi {
			matched = true
		}
	}
	return false, "", false
}

type findLocale struct {
	ctypeGerman    bool
	collateGerman  bool
	messagesGerman bool
}

func findLocaleFromEnv(env []string) findLocale {
	return findLocale{
		ctypeGerman:    isGermanLocale(localeCategory(env, "LC_CTYPE")),
		collateGerman:  isGermanLocale(localeCategory(env, "LC_COLLATE")),
		messagesGerman: isGermanLocale(localeCategory(env, "LC_MESSAGES")),
	}
}

func localeCategory(env []string, category string) string {
	for _, key := range []string{"LC_ALL", category, "LANG"} {
		if value, ok := lookupEnv(env, key); ok && value != "" {
			return value
		}
	}
	return "POSIX"
}

func isGermanLocale(name string) bool {
	return strings.HasPrefix(strings.ToLower(name), "de_de")
}

func germanClassMatch(class string, c byte, fold bool) bool {
	isUpper := c >= 0xc0 && c <= 0xd6 || c >= 0xd8 && c <= 0xde
	isLower := c >= 0xdf && c <= 0xf6 || c >= 0xf8
	switch class {
	case "alpha":
		return isUpper || isLower
	case "alnum":
		return isUpper || isLower
	case "upper":
		return isUpper || fold && isLower
	case "lower":
		return isLower || fold && isUpper
	case "graph", "print":
		return c >= 0xa0
	}
	return false
}

func germanEquivalent(pattern, candidate byte) bool {
	base := lowerASCII(pattern)
	switch candidate {
	case 0xc4, 0xe4: // Ä, ä
		return base == 'a'
	case 0xd6, 0xf6: // Ö, ö
		return base == 'o'
	case 0xdc, 0xfc: // Ü, ü
		return base == 'u'
	case 0xdf: // ß
		return base == 's'
	}
	return false
}
