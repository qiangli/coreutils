// Package grepcmd implements grep(1) per the GNU grep manual: print
// lines of each FILE (or standard input) that match PATTERNS.
//
// Pattern dialects:
//
//   - Default (BRE) is translated construct-by-construct — see bre.go.
//     Patterns without back-references use RE2; patterns with \1..\9 use
//     pkg/bre's bounded backtracking matcher.
//   - -E (ERE): Go regexp syntax is a near-superset of POSIX ERE, so
//     patterns pass through after rejecting back-references and \< \>.
//     Corner deviations from GNU: a malformed interval such as "a{1,"
//     is a compile error here where GNU falls back to a literal; and
//     escape handling inside bracket expressions follows RE2, not
//     POSIX (where backslash is literal inside [...]).
//   - -F: fixed strings.
//
// Binary files (NUL byte within the first 32 KiB) print one
// "Binary file NAME matches" line instead of the matching lines, per
// GNU behavior. -c/-l/-L/-q are unaffected by binary detection.
//
// --include/--exclude/--exclude-dir match the base name of each file
// or directory with shell-glob wildcards (GNU fnmatch globs whose
// wildcards may also match '/' degenerate to the same thing on base
// names; globs containing '/' are not supported and never match).
package grepcmd

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/qiangli/coreutils/pkg/bre"
	"github.com/qiangli/coreutils/pkg/ignore"
	"github.com/qiangli/coreutils/tool"
)

var cmd = &tool.Tool{
	Name:     "grep",
	Synopsis: "Search for PATTERNS in each FILE or standard input.",
	Usage:    "grep [OPTION]... PATTERNS [FILE]...",
}

// Run is wired in init: a literal would create an initialization
// cycle (run's flag-error paths reference cmd).
func init() { cmd.Run = run; tool.Register(cmd) }

type grepper struct {
	rc *tool.RunContext
	re grepMatcher

	invert     bool
	word       bool
	lineRegexp bool
	count      bool
	filesWith  bool
	filesWout  bool
	quiet      bool
	lineNum    bool
	showName   bool
	maxCount   int // -1 = unlimited

	include    []string
	exclude    []string
	excludeDir []string

	matcher *ignore.Matcher // --agentic path filter (nil = off, skips nothing)

	useLit bool   // literal fast path: bytes.Index instead of RE2 (literal.go)
	lit    []byte // the single plain-literal pattern
	buf    []byte // fast path: reused read buffer
	ob     []byte // fast path: batched output buffer

	anyMatch   bool
	anyErr     bool
	listedWout bool
}

func run(rc *tool.RunContext, args []string) int {
	fs := tool.NewFlags(cmd.Name)
	extended := fs.BoolP("extended-regexp", "E", false, "PATTERNS are extended regular expressions")
	fixed := fs.BoolP("fixed-strings", "F", false, "PATTERNS are strings")
	basic := fs.BoolP("basic-regexp", "G", false, "PATTERNS are basic regular expressions (default)")
	patterns := fs.StringArrayP("regexp", "e", nil, "use PATTERNS for matching")
	ignoreCase := fs.BoolP("ignore-case", "i", false, "ignore case distinctions in patterns and data")
	invert := fs.BoolP("invert-match", "v", false, "select non-matching lines")
	word := fs.BoolP("word-regexp", "w", false, "match only whole words")
	lineRe := fs.BoolP("line-regexp", "x", false, "match only whole lines")
	count := fs.BoolP("count", "c", false, "print only a count of selected lines per FILE")
	filesWith := fs.BoolP("files-with-matches", "l", false, "print only names of FILEs with selected lines")
	filesWout := fs.BoolP("files-without-match", "L", false, "print only names of FILEs with no selected lines")
	maxCount := fs.IntP("max-count", "m", -1, "stop after NUM selected lines")
	quiet := fs.BoolP("quiet", "q", false, "suppress all normal output")
	silent := fs.Bool("silent", false, "same as --quiet")
	lineNum := fs.BoolP("line-number", "n", false, "print line number with output lines")
	noFilename := fs.BoolP("no-filename", "h", false, "suppress the file name prefix on output")
	withFilename := fs.BoolP("with-filename", "H", false, "print file name with output lines")
	recurse := fs.BoolP("recursive", "r", false, "read all files under each directory, recursively")
	deref := fs.BoolP("dereference-recursive", "R", false, "likewise, but follow all symlinks")
	include := fs.StringArray("include", nil, "search only files whose base name matches GLOB")
	exclude := fs.StringArray("exclude", nil, "skip files whose base name matches GLOB")
	excludeDir := fs.StringArray("exclude-dir", nil, "skip directories whose base name matches GLOB")
	agentic := fs.Bool("agentic", false, "opt-in: skip .gitignore'd and noise paths (node_modules, .git, vendor, …) during recursive search")

	operands, code := tool.Parse(rc, cmd, fs, args)
	if code >= 0 {
		return code
	}

	matchers := 0
	for _, b := range []bool{*extended, *fixed, *basic} {
		if b {
			matchers++
		}
	}
	if matchers > 1 {
		fmt.Fprintf(rc.Err, "%s: conflicting matchers specified\n", cmd.Name)
		return 2
	}

	pats := *patterns
	files := operands
	if len(pats) == 0 {
		if len(operands) == 0 {
			return tool.UsageError(rc, cmd, "missing pattern; usage: %s [OPTION]... PATTERNS [FILE]...", cmd.Name)
		}
		pats = operands[:1]
		files = operands[1:]
	}
	// A pattern argument containing newlines is a list of patterns.
	var split []string
	for _, p := range pats {
		split = append(split, strings.Split(p, "\n")...)
	}

	// -w is the only mode that reads a match's extent rather than just its
	// existence, so it is the only one that needs POSIX leftmost-longest.
	re, err := compilePattern(split, *fixed, *extended, *lineRe, *ignoreCase, *word)
	if err != nil {
		fmt.Fprintf(rc.Err, "%s: %v\n", cmd.Name, err)
		return 2
	}

	recursive := *recurse || *deref
	if len(files) == 0 {
		if recursive {
			files = []string{"."}
		} else {
			files = []string{"-"}
		}
	}

	g := &grepper{
		rc:         rc,
		re:         re,
		invert:     *invert,
		word:       *word && !*lineRe, // -x makes -w a no-op (GNU)
		lineRegexp: *lineRe,
		count:      *count,
		filesWith:  *filesWith,
		filesWout:  *filesWout,
		quiet:      *quiet || *silent,
		lineNum:    *lineNum,
		maxCount:   *maxCount,
		include:    *include,
		exclude:    *exclude,
		excludeDir: *excludeDir,
	}
	// Literal fast path: a single metachar-free pattern is plain
	// substring work — searchStreamLit skips RE2 and per-line string
	// allocation. Anything it can't serve byte-identically (-i, -w,
	// multiple patterns, real regex) keeps the RE2 path unchanged.
	if lit, ok := literalPattern(split, *fixed, *ignoreCase, g.word); ok {
		g.lit, g.useLit = lit, true
	}
	// --agentic (opt-in): a nil matcher when off skips nothing, so default
	// behavior is byte-identical; on, it prunes .gitignore'd + noise paths.
	if *agentic {
		g.matcher = ignore.New(rc.Dir)
	}
	// GNU default: file names are shown when searching more than one
	// file or recursing; -h suppresses, -H forces.
	switch {
	case *noFilename:
		g.showName = false
	case *withFilename:
		g.showName = true
	default:
		g.showName = len(files) > 1 || recursive
	}

	for _, f := range files {
		if g.quiet && g.anyMatch {
			break
		}
		if f == "-" {
			g.searchStream(rc.In, "(standard input)")
			continue
		}
		full := rc.Path(f)
		st, err := os.Stat(full)
		if err != nil {
			g.report(f, err)
			continue
		}
		if st.IsDir() {
			if !recursive {
				fmt.Fprintf(rc.Err, "%s: %s: Is a directory\n", cmd.Name, f)
				g.anyErr = true
				continue
			}
			if matchAnyGlob(g.excludeDir, filepath.Base(full)) {
				continue
			}
			if *deref {
				g.walkFollow(full, f, map[string]bool{})
			} else {
				g.walk(full, f)
			}
			continue
		}
		if !g.fileAllowed(filepath.Base(f)) {
			continue
		}
		g.grepPath(full, f)
	}

	// Transparency: announce what --agentic hid, so a short/empty result is never
	// silently misleading. stderr only (stdout stays pure matches).
	if n := g.matcher.Hidden(); n > 0 && !g.quiet {
		fmt.Fprintf(rc.Err, "%s: --agentic skipped %d ignored path(s) (run without --agentic to include them)\n", cmd.Name, n)
	}

	switch {
	case g.quiet && g.anyMatch:
		return 0
	case g.anyErr:
		return 2
	case g.filesWout:
		if g.listedWout {
			return 0
		}
		return 1
	case g.anyMatch:
		return 0
	default:
		return 1
	}
}

type grepMatcher interface {
	MatchString(string) bool
	FindStringIndex(string) []int
}

type multiMatcher []*bre.Regexp

func (m multiMatcher) MatchString(s string) bool {
	for _, re := range m {
		if re.MatchString(s) {
			return true
		}
	}
	return false
}

func (m multiMatcher) FindStringIndex(s string) []int {
	var best []int
	for _, re := range m {
		loc := re.FindStringIndex(s)
		if loc == nil {
			continue
		}
		if best == nil || loc[0] < best[0] || (loc[0] == best[0] && loc[1] > best[1]) {
			best = loc
		}
	}
	return best
}

// compilePattern builds one matcher implementing the selected pattern list and
// dialect. BREs without back-references or word-edge anchors still take the
// single combined RE2 path.
//
// longest selects POSIX leftmost-longest matching. It is off by default: a
// leftmost-first and a leftmost-longest engine always agree on whether a line
// matches, so grep's usual boolean question is unaffected and RE2 keeps its
// faster lanes. Callers that read a match's extent (-w) pass true, without
// which `grep -w 'a\|ab'` would report the "a" alternative, fail the
// word-boundary test, and wrongly reject the line "ab".
func compilePattern(pats []string, fixed, extended, lineRe, ignoreCase, longest bool) (grepMatcher, error) {
	if !fixed && !extended {
		needBRE := false
		for _, p := range pats {
			if breNeedsPackageMatcher(p) {
				needBRE = true
				break
			}
		}
		if needBRE {
			out := make(multiMatcher, 0, len(pats))
			for _, p := range pats {
				if lineRe {
					p = "^" + p + "$"
				}
				flags := ""
				if ignoreCase {
					flags = "(?i)"
				}
				re, err := bre.CompileWithFlags(p, flags)
				if err != nil {
					return nil, err
				}
				if longest {
					re.Longest()
				}
				out = append(out, re)
			}
			return out, nil
		}
	}
	parts := make([]string, 0, len(pats))
	for _, p := range pats {
		switch {
		case fixed:
			parts = append(parts, regexp.QuoteMeta(p))
		case extended:
			t, err := bre.ToGoERE(p)
			if err != nil {
				return nil, err
			}
			parts = append(parts, t)
		default:
			t, err := bre.ToGo(p)
			if err != nil {
				return nil, err
			}
			parts = append(parts, t)
		}
	}
	for i, p := range parts {
		parts[i] = "(?:" + p + ")"
	}
	expr := strings.Join(parts, "|")
	if lineRe {
		expr = "^(?:" + expr + ")$"
	}
	if ignoreCase {
		expr = "(?i)" + expr
	}
	re, err := regexp.Compile(expr)
	if err != nil {
		return nil, err
	}
	if longest {
		re.Longest()
	}
	return re, nil
}

func breNeedsPackageMatcher(p string) bool {
	for i := 0; i+1 < len(p); i++ {
		if p[i] == '\\' {
			n := p[i+1]
			if (n >= '1' && n <= '9') || n == '<' || n == '>' {
				return true
			}
			i++
		}
	}
	return false
}

// walk handles -r: lexical filepath.WalkDir, symlinks not followed
// (GNU -r follows symlinks only on the command line).
func (g *grepper) walk(root, display string) {
	filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		disp := joinDisplay(display, root, p)
		if err != nil {
			g.report(disp, err)
			return nil
		}
		if d.IsDir() {
			if p != root && matchAnyGlob(g.excludeDir, d.Name()) {
				return fs.SkipDir
			}
			if p != root && g.matcher.Skip(p, true) {
				return fs.SkipDir
			}
			return nil
		}
		if !d.Type().IsRegular() {
			return nil
		}
		if g.matcher.Skip(p, false) {
			return nil
		}
		if !g.fileAllowed(d.Name()) {
			return nil
		}
		g.grepPath(p, disp)
		if g.quiet && g.anyMatch {
			return fs.SkipAll
		}
		return nil
	})
}

// walkFollow handles -R: like walk but follows symlinks, with loop
// protection via a resolved-directory set.
func (g *grepper) walkFollow(dir, display string, seen map[string]bool) {
	if resolved, err := filepath.EvalSymlinks(dir); err == nil {
		if seen[resolved] {
			return
		}
		seen[resolved] = true
	}
	ents, err := os.ReadDir(dir) // sorted by name
	if err != nil {
		g.report(display, err)
		return
	}
	for _, e := range ents {
		if g.quiet && g.anyMatch {
			return
		}
		p := filepath.Join(dir, e.Name())
		disp := display + "/" + e.Name()
		st, err := os.Stat(p) // follows symlinks
		if err != nil {
			g.report(disp, err)
			continue
		}
		switch {
		case st.IsDir():
			if matchAnyGlob(g.excludeDir, e.Name()) || g.matcher.Skip(p, true) {
				continue
			}
			g.walkFollow(p, disp, seen)
		case st.Mode().IsRegular():
			if !g.matcher.Skip(p, false) && g.fileAllowed(e.Name()) {
				g.grepPath(p, disp)
			}
		}
	}
}

func (g *grepper) fileAllowed(base string) bool {
	if matchAnyGlob(g.exclude, base) {
		return false
	}
	if len(g.include) == 0 {
		return true
	}
	return matchAnyGlob(g.include, base)
}

func matchAnyGlob(globs []string, base string) bool {
	for _, gl := range globs {
		if ok, err := path.Match(gl, base); err == nil && ok {
			return true
		}
	}
	return false
}

func (g *grepper) grepPath(full, display string) {
	f, err := os.Open(full)
	if err != nil {
		g.report(display, err)
		return
	}
	defer f.Close()
	g.searchStream(f, display)
}

// scanLinesKeepCR is bufio.ScanLines minus the \r stripping: GNU grep
// treats a carriage return as ordinary line data.
func scanLinesKeepCR(data []byte, atEOF bool) (int, []byte, error) {
	if atEOF && len(data) == 0 {
		return 0, nil, nil
	}
	if i := bytes.IndexByte(data, '\n'); i >= 0 {
		return i + 1, data[:i], nil
	}
	if atEOF {
		return len(data), data, nil
	}
	return 0, nil, nil
}

func (g *grepper) searchStream(r io.Reader, name string) {
	if g.useLit {
		g.searchStreamLit(r, name)
		return
	}
	if r == nil {
		r = strings.NewReader("")
	}
	br := bufio.NewReaderSize(r, 32*1024)
	peek, _ := br.Peek(32 * 1024)
	binary := bytes.IndexByte(peek, 0) >= 0

	selected := 0
	if g.maxCount != 0 { // -m 0 selects nothing and reads nothing
		sc := bufio.NewScanner(br)
		sc.Buffer(make([]byte, 64*1024), 64*1024*1024)
		sc.Split(scanLinesKeepCR)
		lineNo := 0
		for sc.Scan() {
			lineNo++
			if g.matchLine(sc.Text()) == g.invert {
				continue
			}
			selected++
			g.anyMatch = true
			if g.quiet {
				return
			}
			if g.filesWith {
				fmt.Fprintln(g.rc.Out, name)
				return
			}
			if !g.count && !g.filesWout {
				if binary {
					break // one summary line after the loop
				}
				g.printLine(name, lineNo, sc.Text())
			}
			if g.maxCount > 0 && selected >= g.maxCount {
				break
			}
		}
		if err := sc.Err(); err != nil {
			g.report(name, err)
			return
		}
	}

	if binary && selected > 0 && !g.count && !g.filesWith && !g.filesWout {
		fmt.Fprintf(g.rc.Out, "Binary file %s matches\n", name)
	}
	if g.count {
		if g.showName {
			fmt.Fprintf(g.rc.Out, "%s:%d\n", name, selected)
		} else {
			fmt.Fprintln(g.rc.Out, selected)
		}
	}
	if g.filesWout && selected == 0 {
		fmt.Fprintln(g.rc.Out, name)
		g.listedWout = true
	}
}

func (g *grepper) matchLine(line string) bool {
	if !g.word {
		return g.re.MatchString(line)
	}
	// -w: a line is selected if some match has non-word-constituent
	// context on both sides (GNU: word constituents are letters,
	// digits, and underscore; a side also passes when the match's own
	// edge character is a non-word constituent).
	for i := 0; i <= len(line); {
		loc := g.re.FindStringIndex(line[i:])
		if loc == nil {
			return false
		}
		s, e := i+loc[0], i+loc[1]
		if wordBoundaryOK(line, s, e) {
			return true
		}
		i = s + 1
	}
	return false
}

func wordBoundaryOK(line string, s, e int) bool {
	startOK := s == 0 || !isWordByte(line[s-1]) || (s < e && !isWordByte(line[s]))
	endOK := e == len(line) || !isWordByte(line[e]) || (e > s && !isWordByte(line[e-1]))
	return startOK && endOK
}

func isWordByte(b byte) bool {
	return b == '_' || (b >= '0' && b <= '9') || (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z')
}

func (g *grepper) printLine(name string, n int, line string) {
	var b strings.Builder
	if g.showName {
		b.WriteString(name)
		b.WriteByte(':')
	}
	if g.lineNum {
		b.WriteString(strconv.Itoa(n))
		b.WriteByte(':')
	}
	b.WriteString(line)
	b.WriteByte('\n')
	io.WriteString(g.rc.Out, b.String())
}

func (g *grepper) report(name string, err error) {
	g.anyErr = true
	fmt.Fprintf(g.rc.Err, "%s: %s: %s\n", cmd.Name, name, pathErrMsg(err))
}

// pathErrMsg strips Go's "open <path>: " prefix so diagnostics read
// like GNU's "grep: <name>: <reason>".
func pathErrMsg(err error) string {
	return tool.SysErrString(err)
}

// joinDisplay maps an OS walk path back onto the operand as the user
// typed it, joined with forward slashes (deterministic output shape).
func joinDisplay(operand, root, p string) string {
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
