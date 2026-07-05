// Package diffcmd implements diff(1) per GNU diffutils
// (https://www.gnu.org/software/diffutils/manual/): compare files
// line by line, in the default "normal" format or unified format
// (-u / -U NUM / --unified[=NUM]), with -r recursive directory
// comparison, -q brief mode, -N absent-as-empty, and the -i / -w / -b
// comparison-relaxing flags. Exit status follows GNU: 0 inputs are
// the same, 1 they differ, 2 trouble.
//
// -u and -U are pre-parsed from the argument list (GNU's -u has no
// long spelling of its own and -U requires an attached or separate
// NUM, neither of which pflag models); everything else goes through
// the standard tool.NewFlags / tool.Parse contract.
package diffcmd

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	iofs "io/fs"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/qiangli/coreutils/tool"
)

var cmd = &tool.Tool{
	Name:     "diff",
	Synopsis: "Compare FILES line by line.",
	Usage: "diff [OPTION]... FILES\n" +
		"FILES are 'FILE1 FILE2' or 'DIR1 DIR2' or 'DIR FILE' or 'FILE DIR'.\n" +
		"'-' as a FILE means standard input.\n" +
		"Unified format: -u, -U NUM, --unified[=NUM]\n" +
		"                output NUM (default 3) lines of unified context",
}

// Run is wired in init: a literal would create an initialization
// cycle (run's flag-error paths reference cmd).
func init() { cmd.Run = run; tool.Register(cmd) }

type options struct {
	unified           bool
	context           int
	recursive         bool
	brief             bool
	newFile           bool
	ignoreCase        bool
	ignoreAllSpace    bool
	ignoreSpaceChange bool
	// optTokens echoes the option arguments as the user typed them, for
	// the per-pair "diff OPTS FILE1 FILE2" header lines GNU prints in
	// directory comparisons.
	optTokens []string
}

func run(rc *tool.RunContext, args []string) int {
	rest, opts, perr := prescan(args)
	if perr != "" {
		return tool.UsageError(rc, cmd, "%s", perr)
	}
	flags := tool.NewFlags(cmd.Name)
	recursive := flags.BoolP("recursive", "r", false, "recursively compare any subdirectories found")
	brief := flags.BoolP("brief", "q", false, "report only when files differ")
	newFile := flags.BoolP("new-file", "N", false, "treat absent files as empty")
	ignoreCase := flags.BoolP("ignore-case", "i", false, "ignore case differences in file contents")
	ignoreSpaceChange := flags.BoolP("ignore-space-change", "b", false, "ignore changes in the amount of white space")
	ignoreAllSpace := flags.BoolP("ignore-all-space", "w", false, "ignore all white space")
	operands, code := tool.Parse(rc, cmd, flags, rest)
	if code >= 0 {
		return code
	}
	opts.recursive = *recursive
	opts.brief = *brief
	opts.newFile = *newFile
	opts.ignoreCase = *ignoreCase
	opts.ignoreSpaceChange = *ignoreSpaceChange
	opts.ignoreAllSpace = *ignoreAllSpace

	switch len(operands) {
	case 0:
		return tool.UsageError(rc, cmd, "missing operand after 'diff'")
	case 1:
		return tool.UsageError(rc, cmd, "missing operand after '%s'", operands[0])
	case 2:
	default:
		return tool.UsageError(rc, cmd, "extra operand '%s'", operands[2])
	}

	// Buffer diff output (it is often large and emitted line by line).
	bw := bufio.NewWriter(rc.Out)
	brc := &tool.RunContext{Ctx: rc.Ctx, Dir: rc.Dir, Env: rc.Env,
		FS:    rc.FS,
		Stdio: tool.Stdio{In: rc.In, Out: bw, Err: rc.Err}}
	status := dispatch(brc, &opts, operands[0], operands[1])
	bw.Flush()
	return status
}

// prescan extracts -u / -U NUM / --unified[=NUM] (which pflag cannot
// model) and records every option token verbatim for the directory-
// mode header lines. Returns the remaining args for tool.Parse.
func prescan(args []string) ([]string, options, string) {
	opts := options{context: 3}
	var rest []string
	for i := 0; i < len(args); i++ {
		tok := args[i]
		if tok == "--" {
			rest = append(rest, args[i:]...)
			break
		}
		if tok == "-" || !strings.HasPrefix(tok, "-") {
			rest = append(rest, tok)
			continue
		}
		if strings.HasPrefix(tok, "--") {
			if tok == "--unified" {
				opts.unified = true
				opts.optTokens = append(opts.optTokens, tok)
				continue
			}
			if v, ok := strings.CutPrefix(tok, "--unified="); ok {
				n, perr := parseContext(v)
				if perr != "" {
					return nil, opts, perr
				}
				opts.unified, opts.context = true, n
				opts.optTokens = append(opts.optTokens, tok)
				continue
			}
			rest = append(rest, tok)
			opts.optTokens = append(opts.optTokens, tok)
			continue
		}
		// Short-option cluster: pull out u/U, keep the rest for pflag.
		body := tok[1:]
		var keep []byte
		took := false
	scan:
		for j := 0; j < len(body); j++ {
			switch body[j] {
			case 'u':
				opts.unified = true
			case 'U':
				opts.unified = true
				var v string
				switch {
				case j+1 < len(body):
					v = body[j+1:]
				case i+1 < len(args):
					v = args[i+1]
					took = true
				default:
					return nil, opts, "option requires an argument -- 'U'"
				}
				n, perr := parseContext(v)
				if perr != "" {
					return nil, opts, perr
				}
				opts.context = n
				break scan
			default:
				keep = append(keep, body[j])
			}
		}
		if len(keep) > 0 {
			rest = append(rest, "-"+string(keep))
		}
		opts.optTokens = append(opts.optTokens, tok)
		if took {
			opts.optTokens = append(opts.optTokens, args[i+1])
			i++
		}
	}
	return rest, opts, ""
}

func parseContext(v string) (int, string) {
	n, err := strconv.Atoi(v)
	if err != nil || n < 0 {
		return 0, fmt.Sprintf("invalid context length '%s'", v)
	}
	return n, ""
}

// dispatch applies GNU's operand-type rules: dir+dir compares
// directories; dir+file compares the file against the same base name
// inside the directory; otherwise a plain file pair.
func dispatch(rc *tool.RunContext, opts *options, a, b string) int {
	stat := func(name string) (isDir, missing bool, err error) {
		if name == "-" {
			return false, false, nil
		}
		fi, err := os.Stat(rc.Path(name))
		if err != nil {
			if opts.newFile && errors.Is(err, iofs.ErrNotExist) {
				return false, true, nil
			}
			return false, false, err
		}
		return fi.IsDir(), false, nil
	}
	bad := false
	aIsDir, aMissing, ea := stat(a)
	if ea != nil {
		fmt.Fprintf(rc.Err, "diff: %s: %s\n", a, errText(ea))
		bad = true
	}
	bIsDir, bMissing, eb := stat(b)
	if eb != nil {
		fmt.Fprintf(rc.Err, "diff: %s: %s\n", b, errText(eb))
		bad = true
	}
	if bad {
		return 2
	}
	if aMissing && bMissing {
		fmt.Fprintf(rc.Err, "diff: %s: No such file or directory\n", a)
		fmt.Fprintf(rc.Err, "diff: %s: No such file or directory\n", b)
		return 2
	}
	switch {
	case aIsDir && bIsDir:
		return compareDirs(rc, opts, a, b)
	case aIsDir:
		if bMissing {
			fmt.Fprintf(rc.Err, "diff: %s: No such file or directory\n", b)
			return 2
		}
		a = joinDisplay(a, filepath.Base(b))
	case bIsDir:
		if aMissing {
			fmt.Fprintf(rc.Err, "diff: %s: No such file or directory\n", a)
			return 2
		}
		b = joinDisplay(b, filepath.Base(a))
	}
	return compareFiles(rc, opts, a, b, false)
}

// ---------------------------------------------------------------------------
// Directory comparison

func compareDirs(rc *tool.RunContext, opts *options, da, db string) int {
	la, ea := readDirNames(rc, opts, da)
	lb, eb := readDirNames(rc, opts, db)
	if ea != nil {
		fmt.Fprintf(rc.Err, "diff: %s: %s\n", da, errText(ea))
	}
	if eb != nil {
		fmt.Fprintf(rc.Err, "diff: %s: %s\n", db, errText(eb))
	}
	if ea != nil || eb != nil {
		return 2
	}
	status := 0
	for _, name := range sortedUnion(la, lb) {
		var st int
		switch {
		case la[name] && lb[name]:
			st = comparePair(rc, opts, joinDisplay(da, name), joinDisplay(db, name))
		case la[name]:
			st = onlyIn(rc, opts, da, name, joinDisplay(da, name), joinDisplay(db, name), true)
		default:
			st = onlyIn(rc, opts, db, name, joinDisplay(db, name), joinDisplay(da, name), false)
		}
		if st > status {
			status = st
		}
	}
	return status
}

func readDirNames(rc *tool.RunContext, opts *options, dir string) (map[string]bool, error) {
	ents, err := os.ReadDir(rc.Path(dir))
	if err != nil {
		if opts.newFile && errors.Is(err, iofs.ErrNotExist) {
			return map[string]bool{}, nil // -N: absent directory reads as empty
		}
		return nil, err
	}
	m := make(map[string]bool, len(ents))
	for _, e := range ents {
		m[e.Name()] = true
	}
	return m, nil
}

func sortedUnion(a, b map[string]bool) []string {
	names := make([]string, 0, len(a)+len(b))
	for n := range a {
		names = append(names, n)
	}
	for n := range b {
		if !a[n] {
			names = append(names, n)
		}
	}
	sort.Strings(names) // C-locale byte order
	return names
}

// comparePair handles an entry present in both directories.
func comparePair(rc *tool.RunContext, opts *options, pa, pb string) int {
	fa, err := os.Stat(rc.Path(pa))
	if err != nil {
		fmt.Fprintf(rc.Err, "diff: %s: %s\n", pa, errText(err))
		return 2
	}
	fb, err := os.Stat(rc.Path(pb))
	if err != nil {
		fmt.Fprintf(rc.Err, "diff: %s: %s\n", pb, errText(err))
		return 2
	}
	switch {
	case fa.IsDir() && fb.IsDir():
		if opts.recursive {
			return compareDirs(rc, opts, pa, pb)
		}
		fmt.Fprintf(rc.Out, "Common subdirectories: %s and %s\n", pa, pb)
		return 0
	case fa.IsDir() != fb.IsDir():
		fmt.Fprintf(rc.Out, "File %s is %s while file %s is %s\n",
			pa, kindWord(fa), pb, kindWord(fb))
		return 1
	default:
		return compareFiles(rc, opts, pa, pb, !opts.brief)
	}
}

// onlyIn handles an entry present in exactly one directory: GNU's
// "Only in DIR: name" line, or with -N a diff against the absent side.
func onlyIn(rc *tool.RunContext, opts *options, dir, name, present, absent string, presentIsOld bool) int {
	if !opts.newFile {
		fmt.Fprintf(rc.Out, "Only in %s: %s\n", dir, name)
		return 1
	}
	fi, err := os.Stat(rc.Path(present))
	if err != nil {
		fmt.Fprintf(rc.Err, "diff: %s: %s\n", present, errText(err))
		return 2
	}
	if fi.IsDir() {
		if opts.recursive {
			if presentIsOld {
				return compareDirs(rc, opts, present, absent)
			}
			return compareDirs(rc, opts, absent, present)
		}
		fmt.Fprintf(rc.Out, "Only in %s: %s\n", dir, name)
		return 1
	}
	if presentIsOld {
		return compareFiles(rc, opts, present, absent, !opts.brief)
	}
	return compareFiles(rc, opts, absent, present, !opts.brief)
}

func kindWord(fi iofs.FileInfo) string {
	if fi.IsDir() {
		return "a directory"
	}
	return "a regular file"
}

func joinDisplay(dir, name string) string {
	if strings.HasSuffix(dir, "/") {
		return dir + name
	}
	return dir + "/" + name
}

// ---------------------------------------------------------------------------
// File-pair comparison

type side struct {
	name  string // as displayed (and used in headers/messages)
	data  []byte
	lines []string
	keys  []int
	noEOL bool
	mtime time.Time
}

func loadSide(rc *tool.RunContext, opts *options, name string) (*side, error) {
	s := &side{name: name}
	if name == "-" {
		data, err := io.ReadAll(rc.In)
		if err != nil {
			return nil, err
		}
		s.data = data
		s.mtime = time.Now()
		return s, nil
	}
	path := rc.Path(name)
	fi, err := os.Stat(path)
	if err != nil {
		if opts.newFile && errors.Is(err, iofs.ErrNotExist) {
			s.mtime = time.Unix(0, 0) // GNU stamps absent files with the epoch
			return s, nil
		}
		return nil, err
	}
	s.mtime = fi.ModTime()
	if s.data, err = os.ReadFile(path); err != nil {
		return nil, err
	}
	return s, nil
}

func compareFiles(rc *tool.RunContext, opts *options, a, b string, withHeader bool) int {
	sa, err := loadSide(rc, opts, a)
	if err != nil {
		fmt.Fprintf(rc.Err, "diff: %s: %s\n", a, errText(err))
		return 2
	}
	sb, err := loadSide(rc, opts, b)
	if err != nil {
		fmt.Fprintf(rc.Err, "diff: %s: %s\n", b, errText(err))
		return 2
	}
	if bytes.Equal(sa.data, sb.data) {
		return 0
	}
	if isBinary(sa.data) || isBinary(sb.data) {
		if opts.brief {
			fmt.Fprintf(rc.Out, "Files %s and %s differ\n", a, b)
		} else {
			if withHeader {
				printPairHeader(rc, opts, a, b)
			}
			fmt.Fprintf(rc.Out, "Binary files %s and %s differ\n", a, b)
		}
		return 1
	}
	intern := map[string]int{}
	prep(sa, opts, intern)
	prep(sb, opts, intern)
	gs := diffGroups(sa.keys, sb.keys)
	if len(gs) == 0 {
		return 0 // equal under -i/-w/-b
	}
	if opts.brief {
		fmt.Fprintf(rc.Out, "Files %s and %s differ\n", a, b)
		return 1
	}
	if withHeader {
		printPairHeader(rc, opts, a, b)
	}
	if opts.unified {
		emitUnified(rc.Out, opts, sa, sb, gs)
	} else {
		emitNormal(rc.Out, sa, sb, gs)
	}
	return 1
}

func printPairHeader(rc *tool.RunContext, opts *options, a, b string) {
	parts := append([]string{"diff"}, opts.optTokens...)
	parts = append(parts, a, b)
	fmt.Fprintln(rc.Out, strings.Join(parts, " "))
}

func isBinary(data []byte) bool { return bytes.IndexByte(data, 0) >= 0 }

// prep splits a side into lines and interns each line's comparison
// key. A missing final newline is folded into the last line's key so
// "x\n" and "x" compare unequal, exactly as GNU reports them — except
// under -b / -w, where GNU treats the missing newline as an ignorable
// white-space difference.
func prep(s *side, opts *options, intern map[string]int) {
	if len(s.data) == 0 {
		return
	}
	d := s.data
	if d[len(d)-1] == '\n' {
		d = d[:len(d)-1]
	} else {
		s.noEOL = true
	}
	s.lines = strings.Split(string(d), "\n")
	s.keys = make([]int, len(s.lines))
	for i, ln := range s.lines {
		k := canon(ln, opts)
		if s.noEOL && i == len(s.lines)-1 && !opts.ignoreAllSpace && !opts.ignoreSpaceChange {
			k += "\x00"
		}
		id, ok := intern[k]
		if !ok {
			id = len(intern)
			intern[k] = id
		}
		s.keys[i] = id
	}
}

// canon produces the comparison key for one line under -w / -b / -i.
// C-locale semantics throughout (byte-wise, ASCII case fold).
func canon(line string, opts *options) string {
	switch {
	case opts.ignoreAllSpace: // -w: remove all white space
		b := make([]byte, 0, len(line))
		for i := 0; i < len(line); i++ {
			if !isSpaceByte(line[i]) {
				b = append(b, line[i])
			}
		}
		line = string(b)
	case opts.ignoreSpaceChange: // -b: collapse runs, ignore trailing
		b := make([]byte, 0, len(line))
		run := false
		for i := 0; i < len(line); i++ {
			c := line[i]
			if isSpaceByte(c) {
				run = true
				continue
			}
			if run {
				b = append(b, ' ')
				run = false
			}
			b = append(b, c)
		}
		line = string(b)
	}
	if opts.ignoreCase {
		b := []byte(line)
		for i, c := range b {
			if c >= 'A' && c <= 'Z' {
				b[i] = c + 32
			}
		}
		line = string(b)
	}
	return line
}

func isSpaceByte(c byte) bool {
	switch c {
	case ' ', '\t', '\v', '\f', '\r':
		return true
	}
	return false
}

func errText(err error) string {
	if errors.Is(err, iofs.ErrNotExist) {
		return "No such file or directory"
	}
	return tool.SysErrString(err)
}

// ---------------------------------------------------------------------------
// Edit-script grouping and output formats

// group is one maximal run of non-equal edits: lines [a0,a1) of the
// old file replaced by lines [b0,b1) of the new file (either side may
// be empty).
type group struct{ a0, a1, b0, b1 int }

// diffGroups runs the Myers engine and converts the edit script into
// change groups, normalizing ambiguous run placement the way GNU does
// (boundary shifting: a run that can be expressed one line lower is,
// merging runs that touch).
func diffGroups(ka, kb []int) []group {
	ops := myersOps(ka, kb)
	ca := make([]bool, len(ka))
	cb := make([]bool, len(kb))
	ai, bi := 0, 0
	for _, k := range ops {
		switch k {
		case opEq:
			ai++
			bi++
		case opDel:
			ca[ai] = true
			ai++
		case opIns:
			cb[bi] = true
			bi++
		}
	}
	slideDown(ca, ka)
	slideDown(cb, kb)
	// Pair the two changed maps back into groups: at each synchronized
	// point, one group absorbs every consecutive changed line on both
	// sides (same construction GNU documents for its edit scripts).
	var gs []group
	i, j := 0, 0
	for i < len(ka) || j < len(kb) {
		if (i < len(ka) && ca[i]) || (j < len(kb) && cb[j]) {
			g := group{a0: i, b0: j}
			for i < len(ka) && ca[i] {
				i++
			}
			for j < len(kb) && cb[j] {
				j++
			}
			g.a1, g.b1 = i, j
			gs = append(gs, g)
		} else {
			i++
			j++
		}
	}
	return gs
}

// slideDown canonicalizes ambiguous run placement: when the first
// line of a changed run equals the unchanged line just past it, the
// same edit can be expressed one line lower. GNU shifts every run as
// low as content allows, merging runs that become adjacent.
func slideDown(changed []bool, keys []int) {
	n := len(keys)
	i := 0
	for i < n {
		if !changed[i] {
			i++
			continue
		}
		start := i
		for i < n && changed[i] {
			i++
		}
		// run is [start, i); slide while the line past the run repeats
		// the run's first line
		for i < n && !changed[i] && keys[start] == keys[i] {
			changed[start] = false
			changed[i] = true
			start++
			i++
			for i < n && changed[i] { // absorb a following run
				i++
			}
		}
	}
}

const noNewline = `\ No newline at end of file`

// emitNormal writes GNU's default format: NcM / NaM / NdM hunks with
// "< " old lines, "---", "> " new lines.
func emitNormal(w io.Writer, sa, sb *side, gs []group) {
	for _, g := range gs {
		da, db := g.a1-g.a0, g.b1-g.b0
		switch {
		case da > 0 && db > 0:
			fmt.Fprintf(w, "%sc%s\n", normalRange(g.a0, da), normalRange(g.b0, db))
		case da > 0:
			fmt.Fprintf(w, "%sd%d\n", normalRange(g.a0, da), g.b0)
		default:
			fmt.Fprintf(w, "%da%s\n", g.a0, normalRange(g.b0, db))
		}
		for i := g.a0; i < g.a1; i++ {
			fmt.Fprintf(w, "< %s\n", sa.lines[i])
			if sa.noEOL && i == len(sa.lines)-1 {
				fmt.Fprintln(w, noNewline)
			}
		}
		if da > 0 && db > 0 {
			fmt.Fprintln(w, "---")
		}
		for i := g.b0; i < g.b1; i++ {
			fmt.Fprintf(w, "> %s\n", sb.lines[i])
			if sb.noEOL && i == len(sb.lines)-1 {
				fmt.Fprintln(w, noNewline)
			}
		}
	}
}

func normalRange(start0, count int) string {
	if count == 1 {
		return strconv.Itoa(start0 + 1)
	}
	return fmt.Sprintf("%d,%d", start0+1, start0+count)
}

// GNU's unified header timestamp shape: mtime to nanoseconds + zone.
const stampLayout = "2006-01-02 15:04:05.000000000 -0700"

// emitUnified writes unified format: ---/+++ headers with mtimes,
// hunks merged when separated by at most 2*context unchanged lines
// (GNU's rule), @@ ranges with the count omitted when 1 and the
// preceding line number used when 0.
func emitUnified(w io.Writer, opts *options, sa, sb *side, gs []group) {
	fmt.Fprintf(w, "--- %s\t%s\n", sa.name, sa.mtime.Format(stampLayout))
	fmt.Fprintf(w, "+++ %s\t%s\n", sb.name, sb.mtime.Format(stampLayout))
	ctx := opts.context
	for i := 0; i < len(gs); {
		j := i
		for j+1 < len(gs) && gs[j+1].a0-gs[j].a1 <= 2*ctx {
			j++
		}
		hs := gs[i].a0 - ctx
		if hs < 0 {
			hs = 0
		}
		he := gs[j].a1 + ctx
		if he > len(sa.lines) {
			he = len(sa.lines)
		}
		hbs := gs[i].b0 - (gs[i].a0 - hs)
		hbe := gs[j].b1 + (he - gs[j].a1)
		fmt.Fprintf(w, "@@ -%s +%s @@\n", unifiedRange(hs, he-hs), unifiedRange(hbs, hbe-hbs))
		ai, bi := hs, hbs
		// Context lines are printed from the old file (GNU's choice —
		// visible under -b/-w when the sides differ in white space), so
		// the no-newline marker tracks the old side only.
		context := func() {
			fmt.Fprintf(w, " %s\n", sa.lines[ai])
			if sa.noEOL && ai == len(sa.lines)-1 {
				fmt.Fprintln(w, noNewline)
			}
			ai++
			bi++
		}
		for g := i; g <= j; g++ {
			for ai < gs[g].a0 {
				context()
			}
			for ; ai < gs[g].a1; ai++ {
				fmt.Fprintf(w, "-%s\n", sa.lines[ai])
				if sa.noEOL && ai == len(sa.lines)-1 {
					fmt.Fprintln(w, noNewline)
				}
			}
			for ; bi < gs[g].b1; bi++ {
				fmt.Fprintf(w, "+%s\n", sb.lines[bi])
				if sb.noEOL && bi == len(sb.lines)-1 {
					fmt.Fprintln(w, noNewline)
				}
			}
		}
		for ai < he {
			context()
		}
		i = j + 1
	}
}

func unifiedRange(start0, count int) string {
	switch count {
	case 1:
		return strconv.Itoa(start0 + 1)
	case 0:
		return fmt.Sprintf("%d,0", start0)
	default:
		return fmt.Sprintf("%d,%d", start0+1, count)
	}
}
