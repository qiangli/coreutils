// Package grepcmd implements grep(1) per the GNU grep manual: print
// lines of each FILE (or standard input) that match PATTERNS.
//
// Pattern dialects:
//
//   - Default (BRE) is translated construct-by-construct — see bre.go.
//     Patterns without back-references use RE2; patterns with \1..\9 use
//     pkg/bre's bounded backtracking matcher.
//   - -E (ERE): patterns without back-references use RE2 after translation of
//     POSIX bracket expressions and GNU word-edge anchors; patterns with
//     \1..\9 use pkg/bre's bounded backtracking matcher.
//   - -F: fixed strings.
//
// Binary files (NUL byte within the first 32 KiB) print one
// "Binary file NAME matches" line instead of the matching lines, per
// GNU behavior. -c/-l/-L/-q are unaffected by binary detection.
//
// --include/--exclude rules are applied in command-line order; the last
// matching rule wins. Recursive entries match by base name, while explicit
// command-line files match any slash-delimited name suffix, per GNU.
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
	rc  *tool.RunContext
	re  grepMatcher
	out io.Writer

	invert       bool
	word         bool
	lineRegexp   bool
	count        bool
	filesWith    bool
	filesWout    bool
	quiet        bool
	silent       bool
	lineNum      bool
	showName     bool
	maxCount     int // -1 = unlimited
	onlyMatching bool
	before       int
	after        int

	fileRules  []fileRule
	excludeDir []string

	matcher *ignore.Matcher // --agentic path filter (nil = off, skips nothing)

	useLit bool   // literal fast path: bytes.Index instead of RE2 (literal.go)
	lit    []byte // the single plain-literal pattern
	buf    []byte // fast path: reused read buffer
	ob     []byte // fast path: batched output buffer

	anyMatch   bool
	anyErr     bool
	outputErr  bool
	listedWout bool
	wroteGroup bool
}

type fileRule struct {
	glob    string
	include bool
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
	onlyMatching := fs.BoolP("only-matching", "o", false, "show only the part of a line matching PATTERN")
	quiet := fs.BoolP("quiet", "q", false, "suppress all normal output")
	silent := fs.Bool("silent", false, "same as --quiet")
	suppressErrors := fs.BoolP("no-messages", "s", false, "suppress error messages")
	patternFiles := fs.StringArrayP("file", "f", nil, "obtain patterns from FILE")
	lineNum := fs.BoolP("line-number", "n", false, "print line number with output lines")
	noFilename := fs.BoolP("no-filename", "h", false, "suppress the file name prefix on output")
	withFilename := fs.BoolP("with-filename", "H", false, "print file name with output lines")
	beforeContext := fs.IntP("before-context", "B", 0, "print NUM lines of leading context")
	afterContext := fs.IntP("after-context", "A", 0, "print NUM lines of trailing context")
	contextLines := fs.IntP("context", "C", 0, "print NUM lines of output context")
	recurse := fs.BoolP("recursive", "r", false, "read all files under each directory, recursively")
	deref := fs.BoolP("dereference-recursive", "R", false, "likewise, but follow all symlinks")
	include := fs.StringArray("include", nil, "search only files whose base name matches GLOB")
	exclude := fs.StringArray("exclude", nil, "skip files whose base name matches GLOB")
	excludeDir := fs.StringArray("exclude-dir", nil, "skip directories whose base name matches GLOB")
	agentic := fs.Bool("agentic", false, "opt-in: skip .gitignore'd and noise paths (node_modules, .git, vendor, …) during recursive search")
	// GNU grep permutes options after operands by default. POSIXLY_CORRECT
	// switches to POSIX utility syntax, where option recognition ends at the
	// first operand; that makes real input files named -i and -- unambiguous.
	if _, set := localeEnv(rc.Env, "POSIXLY_CORRECT"); set {
		fs.SetInterspersed(false)
	}

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
	if *beforeContext < 0 || *afterContext < 0 || *contextLines < 0 {
		return tool.UsageError(rc, cmd, "context length must be nonnegative")
	}
	before, after := contextLengths(args, *beforeContext, *afterContext, *contextLines)
	if *onlyMatching && (before > 0 || after > 0) {
		fmt.Fprintln(rc.Err, "grep: warning: --only-matching is specified, but context options are ignored")
		before, after = 0, 0
	}

	pats := append([]string(nil), *patterns...)
	files := operands
	var err error
	if len(pats) == 0 && len(*patternFiles) == 0 {
		if len(operands) == 0 {
			return tool.UsageError(rc, cmd, "missing pattern; usage: %s [OPTION]... PATTERNS [FILE]...", cmd.Name)
		}
		pats = operands[:1]
		files = operands[1:]
	}
	for _, name := range *patternFiles {
		var r io.Reader
		var f *os.File
		if name == "-" && !operandExists(rc, name) {
			r = rc.In
		} else {
			f, err = os.Open(operandPath(rc, name))
			if err != nil {
				if !*suppressErrors {
					fmt.Fprintf(rc.Err, "%s: %s: %s\n", cmd.Name, name, pathErrMsg(err))
				}
				return 2
			}
			r = f
		}
		filePats, readErr := readPatternFile(r)
		if f != nil {
			f.Close()
		}
		if readErr != nil {
			if !*suppressErrors {
				fmt.Fprintf(rc.Err, "%s: %s: %s\n", cmd.Name, name, pathErrMsg(readErr))
			}
			return 2
		}
		pats = append(pats, filePats...)
	}
	// A pattern argument containing newlines is a list of patterns.
	var split []string
	for _, p := range pats {
		split = append(split, strings.Split(p, "\n")...)
	}

	// -w and -o read a match's extent rather than just its existence, so they
	// need POSIX leftmost-longest matching. In particular, -o must print "ab"
	// rather than "a" when equally-leftmost alternatives are "a" and "ab".
	locale := grepLocaleFromEnv(rc.Env)
	if !*fixed {
		for i := range split {
			split[i] = locale.rewritePattern(split[i])
		}
	}
	re, err := compilePattern(split, *fixed, *extended, *lineRe, *ignoreCase, *word || *onlyMatching)
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
		rc:           rc,
		re:           re,
		invert:       *invert,
		word:         *word && !*lineRe, // -x makes -w a no-op (GNU)
		lineRegexp:   *lineRe,
		count:        *count,
		filesWith:    *filesWith,
		filesWout:    *filesWout,
		quiet:        *quiet || *silent,
		silent:       *suppressErrors,
		lineNum:      *lineNum,
		maxCount:     *maxCount,
		before:       before,
		after:        after,
		fileRules:    orderedFileRules(args, *include, *exclude),
		excludeDir:   *excludeDir,
		onlyMatching: *onlyMatching,
	}
	if locale.ctypeGerman && !*fixed {
		re = localeMatcher{inner: re}
		g.re = re
	}
	g.out = grepOutput{g: g}
	// Literal fast path: a single metachar-free pattern is plain
	// substring work — searchStreamLit skips RE2 and per-line string
	// allocation. Anything it can't serve byte-identically (-i, -w,
	// multiple patterns, real regex) keeps the RE2 path unchanged.
	if lit, ok := literalPattern(split, *fixed, *ignoreCase, g.word, g.onlyMatching); ok && before == 0 && after == 0 {
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
		full := operandPath(rc, f)
		st, err := os.Stat(full)
		if err != nil {
			g.report(f, err)
			continue
		}
		if st.IsDir() {
			if !recursive {
				if !g.silent {
					fmt.Fprintf(rc.Err, "%s: %s: Is a directory\n", cmd.Name, f)
				}
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
		if !g.fileAllowed(f, false) {
			continue
		}
		g.grepPath(full, f)
	}

	// Transparency: announce what --agentic hid, so a short/empty result is never
	// silently misleading. stderr only (stdout stays pure matches).
	if n := g.matcher.Hidden(); n > 0 && !g.quiet && !g.silent {
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

// operandPath preserves a standalone invocation's lexical pathname. Besides
// avoiding needless allocation, this leaves `..` and repeated separators for
// the kernel to resolve, as POSIX pathname-resolution assertions require. An
// embedded invocation still resolves against its virtual RunContext directory.
func operandPath(rc *tool.RunContext, operand string) string {
	if rc.DirIsProcessCwd && !filepath.IsAbs(operand) {
		return operand
	}
	return rc.Path(operand)
}

func operandExists(rc *tool.RunContext, operand string) bool {
	_, err := os.Stat(operandPath(rc, operand))
	return err == nil
}

type grepOutput struct{ g *grepper }

func (w grepOutput) Write(p []byte) (int, error) {
	n, err := w.g.rc.Out.Write(p)
	if err != nil && !w.g.outputErr {
		w.g.outputErr = true
		w.g.anyErr = true
		fmt.Fprintf(w.g.rc.Err, "%s: write error: %s\n", cmd.Name, pathErrMsg(err))
	}
	return n, err
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
// faster lanes. Callers that read a match's extent (-w and -o) pass true,
// without which `grep -w 'a\|ab'` would report the "a" alternative, fail
// the word-boundary test, and wrongly reject the line "ab"; -o would likewise
// print the shorter alternative.
func compilePattern(pats []string, fixed, extended, lineRe, ignoreCase, longest bool) (grepMatcher, error) {
	if len(pats) == 0 {
		return noMatcher{}, nil
	}
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
	if !fixed && extended {
		needBRE := false
		for _, p := range pats {
			if bre.ERERequiresBacktracking(p) {
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
				re, err := bre.CompileEREWithFlags(p, flags)
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

type noMatcher struct{}

func (noMatcher) MatchString(string) bool      { return false }
func (noMatcher) FindStringIndex(string) []int { return nil }

// contextLengths replays context options in command-line order. GNU lets -C
// set both values and a later -A or -B replace just one side, so the final
// pflag values alone are not enough to recover the requested behavior.
func contextLengths(args []string, parsedBefore, parsedAfter, parsedContext int) (before, after int) {
	seen := false
	set := func(kind byte, value int) {
		seen = true
		switch kind {
		case 'A':
			after = value
		case 'B':
			before = value
		case 'C':
			before, after = value, value
		}
	}
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--" {
			break
		}
		if strings.HasPrefix(arg, "--") {
			name, value, hasValue := strings.Cut(strings.TrimPrefix(arg, "--"), "=")
			kind := byte(0)
			switch {
			case strings.HasPrefix("after-context", name):
				kind = 'A'
			case strings.HasPrefix("before-context", name):
				kind = 'B'
			case strings.HasPrefix("context", name):
				kind = 'C'
			}
			if kind != 0 {
				if !hasValue && i+1 < len(args) {
					i++
					value = args[i]
				}
				if n, err := strconv.Atoi(value); err == nil {
					set(kind, n)
				}
				continue
			}
			// Do not mistake a value belonging to another option for a
			// context flag.
			if !hasValue {
				switch name {
				case "regexp", "file", "max-count", "include", "exclude", "exclude-dir":
					i++
				}
			}
			continue
		}
		if len(arg) < 2 || arg[0] != '-' {
			continue
		}
		cluster := arg[1:]
		for j := 0; j < len(cluster); j++ {
			ch := cluster[j]
			switch ch {
			case 'A', 'B', 'C':
				value := cluster[j+1:]
				if value == "" && i+1 < len(args) {
					i++
					value = args[i]
				}
				if n, err := strconv.Atoi(value); err == nil {
					set(ch, n)
				}
				j = len(cluster)
			case 'e', 'f', 'm':
				if j+1 == len(cluster) {
					i++
				}
				j = len(cluster)
			}
		}
	}
	if !seen {
		// This fallback also covers any future spelling accepted by the flag
		// layer that the replay above does not recognize.
		if parsedContext != 0 {
			return parsedContext, parsedContext
		}
		return parsedBefore, parsedAfter
	}
	return before, after
}

func readPatternFile(r io.Reader) ([]string, error) {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 64*1024), 64*1024*1024)
	sc.Split(scanLinesKeepCR)
	var pats []string
	for sc.Scan() {
		pats = append(pats, sc.Text())
	}
	return pats, sc.Err()
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
		if !g.fileAllowed(d.Name(), true) {
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
			if !g.matcher.Skip(p, false) && g.fileAllowed(e.Name(), true) {
				g.grepPath(p, disp)
			}
		}
	}
}

func (g *grepper) fileAllowed(name string, recursive bool) bool {
	if len(g.fileRules) == 0 {
		return true
	}
	allowed := !g.fileRules[0].include
	for _, rule := range g.fileRules {
		if matchFileGlob(rule.glob, name, recursive) {
			allowed = rule.include
		}
	}
	return allowed
}

func matchFileGlob(glob, name string, recursive bool) bool {
	name = filepath.ToSlash(name)
	if recursive {
		ok, err := path.Match(glob, path.Base(name))
		return err == nil && ok
	}
	for {
		if ok, err := path.Match(glob, name); err == nil && ok {
			return true
		}
		slash := strings.IndexByte(name, '/')
		if slash < 0 {
			return false
		}
		name = name[slash+1:]
	}
}

func orderedFileRules(args, includes, excludes []string) []fileRule {
	var rules []fileRule
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--" {
			break
		}
		if !strings.HasPrefix(arg, "--") {
			continue
		}
		name, value, hasValue := strings.Cut(strings.TrimPrefix(arg, "--"), "=")
		include := strings.HasPrefix("include", name)
		exclude := name == "exclude" ||
			(strings.HasPrefix("exclude", name) && !strings.HasPrefix("exclude-dir", name))
		if !include && !exclude {
			continue
		}
		if !hasValue && i+1 < len(args) {
			i++
			value = args[i]
		}
		rules = append(rules, fileRule{glob: value, include: include})
	}
	if len(rules) == 0 {
		for _, glob := range includes {
			rules = append(rules, fileRule{glob: glob, include: true})
		}
		for _, glob := range excludes {
			rules = append(rules, fileRule{glob: glob})
		}
	}
	return rules
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

	type bufferedLine struct {
		number int
		text   string
	}

	selected := 0
	if g.maxCount != 0 { // -m 0 selects nothing and reads nothing
		sc := bufio.NewScanner(br)
		sc.Buffer(make([]byte, 64*1024), 64*1024*1024)
		sc.Split(scanLinesKeepCR)
		lineNo := 0
		lastPrinted := 0
		afterRemaining := 0
		var leading []bufferedLine
		contextOutput := (g.before > 0 || g.after > 0) &&
			!g.count && !g.filesWith && !g.filesWout && !g.onlyMatching
		stopMatching := false
		for sc.Scan() {
			lineNo++
			line := sc.Text()
			matched := !stopMatching && g.matchLine(line) != g.invert
			if matched {
				selected++
				g.anyMatch = true
				if g.quiet {
					return
				}
				if g.filesWith {
					fmt.Fprintln(g.out, name)
					return
				}
				if !g.count && !g.filesWout {
					if binary {
						break // one summary line after the loop
					}
					if contextOutput {
						for _, prev := range leading {
							g.printContextLine(name, prev.number, prev.text, false, &lastPrinted)
						}
						g.printContextLine(name, lineNo, line, true, &lastPrinted)
						afterRemaining = max(afterRemaining, g.after)
					} else if g.onlyMatching && !g.invert {
						g.printMatches(name, lineNo, line)
					} else {
						g.printLine(name, lineNo, line)
					}
				}
				if g.maxCount > 0 && selected >= g.maxCount {
					stopMatching = true
					if !contextOutput || afterRemaining == 0 {
						break
					}
				}
			} else if contextOutput && afterRemaining > 0 {
				g.printContextLine(name, lineNo, line, false, &lastPrinted)
				afterRemaining--
				if stopMatching && afterRemaining == 0 {
					break
				}
			}

			if g.before > 0 {
				leading = append(leading, bufferedLine{lineNo, line})
				if len(leading) > g.before {
					leading = leading[len(leading)-g.before:]
				}
			}
		}
		if err := sc.Err(); err != nil {
			g.report(name, err)
			return
		}
	}

	if binary && selected > 0 && !g.count && !g.filesWith && !g.filesWout {
		fmt.Fprintf(g.out, "Binary file %s matches\n", name)
	}
	if g.count {
		if g.showName {
			fmt.Fprintf(g.out, "%s:%d\n", name, selected)
		} else {
			fmt.Fprintln(g.out, selected)
		}
	}
	if g.filesWout && selected == 0 {
		fmt.Fprintln(g.out, name)
		g.listedWout = true
	}
}

// printContextLine prints one line in a context group. Prefix fields use ':'
// for selected lines and '-' for context lines, and non-adjacent groups are
// separated by the GNU default "--" marker.
func (g *grepper) printContextLine(name string, n int, line string, matched bool, lastPrinted *int) {
	if n <= *lastPrinted {
		return
	}
	if (*lastPrinted == 0 && g.wroteGroup) || (*lastPrinted > 0 && n > *lastPrinted+1) {
		fmt.Fprintln(g.out, "--")
	}
	sep := byte('-')
	if matched {
		sep = ':'
	}
	var b strings.Builder
	if g.showName {
		b.WriteString(name)
		b.WriteByte(sep)
	}
	if g.lineNum {
		b.WriteString(strconv.Itoa(n))
		b.WriteByte(sep)
	}
	b.WriteString(line)
	b.WriteByte('\n')
	io.WriteString(g.out, b.String())
	*lastPrinted = n
	g.wroteGroup = true
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

func (g *grepper) printMatches(name string, lineNo int, line string) {
	for i := 0; i <= len(line); {
		loc := g.re.FindStringIndex(line[i:])
		if loc == nil {
			break
		}
		s, e := i+loc[0], i+loc[1]
		matchLen := e - s
		if matchLen > 0 {
			if !g.word || wordBoundaryOK(line, s, e) {
				g.printLine(name, lineNo, line[s:e])
			}
			i = e
		} else {
			i = s + 1
		}
	}
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
	io.WriteString(g.out, b.String())
}

func (g *grepper) report(name string, err error) {
	g.anyErr = true
	if g.silent {
		return
	}
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
