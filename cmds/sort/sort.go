// Package sortcmd implements sort(1) per the GNU coreutils manual:
// sort lines of text files.
//
// Comparisons use byte order in the C/POSIX locale and the narrowly supported
// LC_COLLATE provider for its accepted non-C locale aliases. Numeric -n keys
// also honor the narrowly supported LC_NUMERIC radix and thousands bytes.
// Implemented flags: -b -c -C -d -f -g -h -i -k -m -M -n -o -r
// -R --random-source -s -t -T -u -V -z --files0-from.
// GNU's last-resort whole-line comparison applies unless -s/-u, and the
// global ordering options are inherited by keys that carry no options
// of their own, exactly as documented in the manual.
package sortcmd

import (
	"bufio"
	"bytes"
	"crypto/rand"
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
	"unsafe"

	"github.com/qiangli/coreutils/pkg/collate"
	"github.com/qiangli/coreutils/pkg/ctype"
	"github.com/qiangli/coreutils/pkg/locale"
	"github.com/qiangli/coreutils/tool"
)

var cmd = &tool.Tool{
	Name:     "sort",
	Synopsis: "Write sorted concatenation of all FILE(s) to standard output.",
	Usage:    "sort [OPTION]... [FILE]...\n\nWith no FILE, or when FILE is -, read standard input.",
}

func init() { cmd.Run = run; tool.Register(cmd) }

type sorter struct {
	keys        []keySpec
	tab         int // field separator byte; -1 = blank/non-blank transition
	reverse     bool
	stable      bool
	unique      bool
	dict        bool
	generalNum  bool
	ignoreNP    bool
	month       bool
	version     bool
	random      bool
	randSrc     io.Reader
	checkSilent bool
	zeroTerm    bool
	merge       bool
	collator    *collatorAdapter
	ctype       *ctypeTables
	decPt       byte
	thousSep    byte
}

func run(rc *tool.RunContext, args []string) int {
	return runWithProviders(rc, args, func(name string) (stringCollator, error) {
		return collate.Open(name)
	}, openCType)
}

// openCType is the production LC_CTYPE opener wired in run. It reaches the
// shared invocation-owned pkg/ctype provider, which is fail-closed on the
// unsupported platforms exactly as collate.Open is.
func openCType(name string) (ctypeProvider, error) { return ctype.Open(name) }

// runWithCollator keeps collation setup invocation-local. Tests that exercise
// only the LC_COLLATE seam supply a fake collator opener; the LC_CTYPE seam
// stays on the production opener (never reached unless a non-C LC_CTYPE and a
// -f/-d/-i key coincide).
func runWithCollator(rc *tool.RunContext, args []string, openCollator collatorOpener) int {
	return runWithProviders(rc, args, openCollator, openCType)
}

// runWithProviders keeps both the LC_COLLATE and LC_CTYPE provider setups
// invocation-local. Tests supply fake openers; production supplies collate.Open
// and ctype.Open. No provider state is shared between sort invocations.
func runWithProviders(rc *tool.RunContext, args []string, openCollator collatorOpener, openCtype ctypeOpener) int {
	fs := tool.NewFlags(cmd.Name)
	blanks := fs.BoolP("ignore-leading-blanks", "b", false, "ignore leading blanks")
	check := fs.BoolP("check", "c", false, "check for sorted input; do not sort")
	checkSilent := fs.BoolP("check-silent", "C", false, "like -c, but do not report first bad line")
	dict := fs.BoolP("dictionary-order", "d", false, "consider only blanks and alphanumeric characters")
	fold := fs.BoolP("ignore-case", "f", false, "fold lower case to upper case characters")
	generalNum := fs.BoolP("general-numeric-sort", "g", false, "compare according to general numerical value")
	human := fs.BoolP("human-numeric-sort", "h", false, "compare human readable numbers (e.g., 2K 1G)")
	ignoreNP := fs.BoolP("ignore-nonprinting", "i", false, "consider only printable characters")
	keyDefs := fs.StringArrayP("key", "k", nil, "sort via a key; KEYDEF gives location and type")
	merge := fs.BoolP("merge", "m", false, "merge already sorted files; do not sort")
	month := fs.BoolP("month-sort", "M", false, "compare (unknown) < 'JAN' < ... < 'DEC'")
	numeric := fs.BoolP("numeric-sort", "n", false, "compare according to string numerical value")
	output := fs.StringP("output", "o", "", "write result to FILE instead of standard output")
	randSort := fs.BoolP("random-sort", "R", false, "shuffle, but group identical keys")
	randSource := fs.StringP("random-source", "", "", "get random bytes from FILE")
	sortMode := fs.String("sort", "", "sort according to WORD: general-numeric, human-numeric, month, numeric, random, version")
	reverse := fs.BoolP("reverse", "r", false, "reverse the result of comparisons")
	stable := fs.BoolP("stable", "s", false, "stabilize sort by disabling last-resort comparison")
	sep := fs.StringP("field-separator", "t", "", "use SEP instead of non-blank to blank transition")
	tmpDir := fs.StringP("temporary-directory", "T", "", "use DIR for temporaries, not $TMPDIR or /tmp; implies --debug")
	fs.String("batch-size", "", "merge at most N inputs at once (accepted as an in-memory no-op)")
	fs.StringP("buffer-size", "S", "", "use SIZE for main memory buffer (accepted as an in-memory no-op)")
	fs.String("compress-program", "", "compress temporary files with PROG (accepted as an in-memory no-op)")
	fs.Bool("debug", false, "annotate the part of the line used to sort; accepted as a no-op")
	fs.String("parallel", "", "change the number of sorts run concurrently (accepted as an in-memory no-op)")
	unique := fs.BoolP("unique", "u", false, "with -c, check for strict ordering; without -c, output only the first of an equal run")
	versionSort := fs.BoolP("version-sort", "V", false, "natural sort of (version) numbers within text")
	zeroTerm := fs.BoolP("zero-terminated", "z", false, "line delimiter is NUL, not newline")
	files0 := fs.StringP("files0-from", "", "", "read input from the files specified by NUL-terminated names in file F")
	operands, code := tool.Parse(rc, cmd, fs, args)
	if code >= 0 {
		return code
	}

	if *sortMode != "" {
		switch *sortMode {
		case "general-numeric", "g":
			*generalNum = true
		case "human-numeric", "human", "h":
			*human = true
		case "month", "M":
			*month = true
		case "numeric", "n":
			*numeric = true
		case "random", "R":
			*randSort = true
		case "version", "V":
			*versionSort = true
		default:
			return tool.UsageError(rc, cmd, "invalid --sort argument %q", *sortMode)
		}
	}
	if *numeric && *human {
		fmt.Fprintf(rc.Err, "sort: options '-hn' are incompatible\n")
		return 2
	}
	if *numeric && *generalNum {
		fmt.Fprintf(rc.Err, "sort: options '-ng' are incompatible\n")
		return 2
	}

	s := &sorter{
		tab: -1, reverse: *reverse, stable: *stable, unique: *unique,
		dict: *dict, generalNum: *generalNum, ignoreNP: *ignoreNP,
		month: *month, version: *versionSort,
		random: *randSort, checkSilent: *checkSilent,
		zeroTerm: *zeroTerm, merge: *merge,
	}
	if fs.Changed("field-separator") {
		switch {
		case *sep == "":
			fmt.Fprintf(rc.Err, "sort: empty tab\n")
			return 2
		case *sep == `\0`:
			s.tab = 0
		case len(*sep) == 1:
			s.tab = int((*sep)[0])
		default:
			fmt.Fprintf(rc.Err, "sort: multi-character tab '%s'\n", *sep)
			return 2
		}
	}

	// Validate and parse every key before locale initialization so malformed
	// keys win over locale/provider errors and no partial key state escapes.
	for _, def := range *keyDefs {
		k, errMsg, badType := parseKeySpec(def)
		if badType != 0 {
			return tool.NotSupported(rc, cmd, fmt.Sprintf("key type letter '%c' (in '-k %s')", badType, def))
		}
		if errMsg != "" {
			fmt.Fprintf(rc.Err, "sort: %s: invalid field specification '%s'\n", errMsg, def)
			return 2
		}
		if k.opts.numeric && k.opts.human {
			fmt.Fprintf(rc.Err, "sort: options '-hn' are incompatible\n")
			return 2
		}
		s.keys = append(s.keys, k)
	}

	gOpts := keyOpts{numeric: *numeric, human: *human, fold: *fold,
		dict: *dict, generalNum: *generalNum, ignoreNP: *ignoreNP,
		month: *month, version: *versionSort, random: *randSort,
		skipSB: *blanks, skipEB: *blanks, reverse: *reverse}
	for i := range s.keys {
		k := &s.keys[i]
		if !k.opts.hasMods() && !k.opts.reverse {
			k.opts = gOpts
		}
	}
	if len(s.keys) == 0 && gOpts.hasMods() {
		s.keys = append(s.keys, keySpec{sword: 0, schar: 0, eword: -1, echar: 0, opts: gOpts})
	}

	// Initialize locale providers only after all keys have been validated and
	// effective key modes are known. This keeps invalid-key diagnostics first
	// and avoids LC_NUMERIC for a global -n overridden by key-local -f.
	if name := locale.Resolve(rc.Env, locale.Collate); name != "C" && name != "POSIX" {
		provider, err := openCollator(name)
		if err != nil {
			fmt.Fprintf(rc.Err, "sort: LC_COLLATE=%s: %v\n", name, err)
			return 2
		}
		s.collator = newCollatorAdapter(provider)
		defer s.collator.Close()
	}
	usesNumeric := false
	for _, k := range s.keys {
		usesNumeric = usesNumeric || k.opts.numeric
	}
	if usesNumeric {
		if name := locale.Resolve(rc.Env, locale.Numeric); name != "C" && name != "POSIX" {
			switch strings.ToLower(name) {
			case "de_de.iso-8859-1", "de_de.iso88591":
				s.decPt, s.thousSep = ',', '.'
			default:
				fmt.Fprintf(rc.Err, "sort: LC_NUMERIC=%s: not supported\n", name)
				return 2
			}
		}
	}
	usesTextClass := false
	for _, k := range s.keys {
		usesTextClass = usesTextClass || k.opts.fold || k.opts.dict || k.opts.ignoreNP
	}
	if usesTextClass {
		if name := locale.Resolve(rc.Env, locale.CType); name != "C" && name != "POSIX" {
			provider, err := openCtype(name)
			if err != nil {
				fmt.Fprintf(rc.Err, "sort: LC_CTYPE=%s: %v\n", name, err)
				return 2
			}
			tables, snapErr := snapshotCtypeTables(provider)
			closeErr := provider.Close()
			if snapErr != nil {
				fmt.Fprintf(rc.Err, "sort: LC_CTYPE=%s: %v\n", name, snapErr)
				return 2
			}
			if closeErr != nil {
				fmt.Fprintf(rc.Err, "sort: LC_CTYPE=%s: %v\n", name, closeErr)
				return 2
			}
			s.ctype = tables
		}
	}

	if *randSource != "" {
		f, err := os.Open(rc.Path(*randSource))
		if err != nil {
			fmt.Fprintf(rc.Err, "sort: %s: cannot open\n", *randSource)
			return 2
		}
		defer f.Close()
		s.randSrc = f
	}

	_ = *tmpDir

	if *files0 != "" {
		files, err := readFiles0From(rc, *files0)
		if err != nil {
			fmt.Fprintf(rc.Err, "sort: %s: %v\n", *files0, err)
			return 2
		}
		operands = append(files, operands...)
	}

	if len(operands) == 0 {
		operands = []string{"-"}
	}

	if *check || *checkSilent {
		if s.merge {
			fmt.Fprintf(rc.Err, "sort: options '-cm' are incompatible\n")
			return 2
		}
		if *output != "" {
			fmt.Fprintf(rc.Err, "sort: options '-co' are incompatible\n")
			return 2
		}
		if len(operands) > 1 {
			fmt.Fprintf(rc.Err, "sort: extra operand '%s' not allowed with -c\n", operands[1])
			return 2
		}
	}

	if s.merge && s.random {
		return tool.NotSupported(rc, cmd, "combining --merge with --random-sort")
	}

	inputs, badOperand, err := openOperands(rc, operands, func(path string) (io.ReadCloser, error) {
		return os.Open(path)
	})
	if err != nil {
		fmt.Fprintf(rc.Err, "sort: cannot read: %s: %v\n", badOperand, pathErr(err))
		return 2
	}
	defer closeOperands(inputs)

	if *check || *checkSilent {
		return s.checkSorted(rc, inputs[0])
	}

	if s.merge {
		return s.mergeFiles(rc, inputs, *output)
	}

	if s.random && !s.hasNonTrivialKeys() {
		return s.randomShuffle(rc, inputs, *output)
	}

	if s.canUsePreparedNumeric() {
		nlines := numericLineSet{allInt: true}
		for _, input := range inputs {
			if err := nlines.read(input.reader, s.zeroTerm); err != nil {
				fmt.Fprintf(rc.Err, "sort: cannot read: %s: %v\n", input.name, pathErr(err))
				return 2
			}
		}
		s.sortNumericLines(nlines.lines, nlines.allInt)
		if err := s.collationErr(); err != nil {
			return s.collationFailure(rc, err)
		}
		if s.unique {
			out := nlines.lines[:0]
			for i, l := range nlines.lines {
				if i == 0 {
					out = append(out, l)
					continue
				}
				prev := out[len(out)-1]
				if compareNumericKey(prev.text, prev.num, l.text, l.num) != 0 {
					out = append(out, l)
				}
			}
			nlines.lines = out
		}
		return writeNumericLines(rc, *output, nlines.lines, s.zeroTerm)
	}

	var lines lineSet
	for _, input := range inputs {
		if err := lines.read(input.reader, s.zeroTerm); err != nil {
			fmt.Fprintf(rc.Err, "sort: cannot read: %s: %v\n", input.name, pathErr(err))
			return 2
		}
	}

	s.sortLines(lines.lines)
	if err := s.collationErr(); err != nil {
		return s.collationFailure(rc, err)
	}

	if s.unique {
		out := lines.lines[:0]
		for i, l := range lines.lines {
			if i == 0 || s.compareEqual(out[len(out)-1], l) != 0 {
				out = append(out, l)
			}
		}
		lines.lines = out
		if err := s.collationErr(); err != nil {
			return s.collationFailure(rc, err)
		}
	}

	return writeStringLines(rc, *output, lines.lines, s.zeroTerm)
}

func writeStringLines(rc *tool.RunContext, output string, lines []string, zeroTerm bool) int {
	var w io.Writer = rc.Out
	if output != "" {
		f, err := os.Create(rc.Path(output))
		if err != nil {
			fmt.Fprintf(rc.Err, "sort: open failed: %s: %v\n", output, pathErr(err))
			return 2
		}
		defer f.Close()
		w = f
	}
	bw := bufio.NewWriter(w)
	delim := byte('\n')
	if zeroTerm {
		delim = 0
	}
	for _, l := range lines {
		if _, err := bw.WriteString(l); err != nil {
			fmt.Fprintf(rc.Err, "sort: write failed: %v\n", err)
			return 2
		}
		if err := bw.WriteByte(delim); err != nil {
			fmt.Fprintf(rc.Err, "sort: write failed: %v\n", err)
			return 2
		}
	}
	if err := bw.Flush(); err != nil {
		fmt.Fprintf(rc.Err, "sort: write failed: %v\n", err)
		return 2
	}
	return 0
}

func writeNumericLines(rc *tool.RunContext, output string, lines []numericLine, zeroTerm bool) int {
	var w io.Writer = rc.Out
	if output != "" {
		f, err := os.Create(rc.Path(output))
		if err != nil {
			fmt.Fprintf(rc.Err, "sort: open failed: %s: %v\n", output, pathErr(err))
			return 2
		}
		defer f.Close()
		w = f
	}
	bw := bufio.NewWriter(w)
	delim := byte('\n')
	if zeroTerm {
		delim = 0
	}
	for _, l := range lines {
		if _, err := bw.WriteString(l.text); err != nil {
			fmt.Fprintf(rc.Err, "sort: write failed: %v\n", err)
			return 2
		}
		if err := bw.WriteByte(delim); err != nil {
			fmt.Fprintf(rc.Err, "sort: write failed: %v\n", err)
			return 2
		}
	}
	if err := bw.Flush(); err != nil {
		fmt.Fprintf(rc.Err, "sort: write failed: %v\n", err)
		return 2
	}
	return 0
}

func (s *sorter) canUsePreparedNumeric() bool {
	if s.collator != nil || s.decPt != 0 || s.thousSep != 0 {
		return false
	}
	if len(s.keys) != 1 {
		return false
	}
	k := &s.keys[0]
	return k.sword == 0 && k.schar == 0 && k.eword == -1 && k.echar == 0 &&
		k.opts.numeric && !k.opts.human && !k.opts.fold && !k.opts.dict &&
		!k.opts.generalNum && !k.opts.ignoreNP && !k.opts.month &&
		!k.opts.random && !k.opts.version && !k.opts.skipSB && !k.opts.skipEB
}

func (s *sorter) hasNonTrivialKeys() bool {
	for _, k := range s.keys {
		if k.sword != 0 || k.schar != 0 || k.eword != -1 || k.echar != 0 {
			return true
		}
	}
	return false
}

func (s *sorter) compare(a, b string) int {
	if len(s.keys) > 0 {
		if d := s.compareKeys(a, b); d != 0 || s.stable || s.unique {
			return d
		}
	}
	d := s.textCompare(a, b)
	if s.reverse {
		return -d
	}
	return d
}

func (s *sorter) compareEqual(a, b string) int {
	if len(s.keys) > 0 {
		return s.compareKeys(a, b)
	}
	return s.textCompare(a, b)
}

func (s *sorter) compareKeys(a, b string) int {
	for i := range s.keys {
		k := &s.keys[i]
		ka := extractKey(a, k, s.tab)
		kb := extractKey(b, k, s.tab)
		var d int
		switch {
		case k.opts.random:
			d = 0
		case k.opts.generalNum:
			d = generalNumCompare(ka, kb)
		case k.opts.numeric:
			d = s.numCompare(ka, kb)
		case k.opts.human:
			d = humanCompare(ka, kb)
		case k.opts.month:
			d = monthCompare(ka, kb)
		case k.opts.version:
			d = versionCompare(ka, kb)
		default:
			d = s.textKeyCompare(ka, kb, k.opts)
		}
		if d != 0 {
			if k.opts.reverse {
				return -d
			}
			return d
		}
	}
	return 0
}

func (s *sorter) textCompare(a, b string) int {
	if s.collator != nil {
		return s.collator.Compare(a, b)
	}
	return strings.Compare(a, b)
}

func (s *sorter) textKeyCompare(a, b string, o keyOpts) int {
	if o.dict || o.ignoreNP || o.fold {
		a, b = s.normalizeTextKey(a, o), s.normalizeTextKey(b, o)
	}
	return s.textCompare(a, b)
}

// normalizeTextKey applies the -d/-i/-f transforms using the invocation's
// LC_CTYPE snapshot when one is active, and the ASCII byte tables otherwise
// (the correct classes for the C/POSIX locale).
func (s *sorter) normalizeTextKey(str string, o keyOpts) string {
	if s.ctype != nil {
		return s.ctype.normalize(str, o)
	}
	return normalizeTextKey(str, o)
}

func (s *sorter) collationErr() error {
	if s.collator == nil {
		return nil
	}
	return s.collator.Err()
}

func (s *sorter) collationFailure(rc *tool.RunContext, err error) int {
	fmt.Fprintf(rc.Err, "sort: LC_COLLATE comparison failed: %v\n", err)
	return 2
}

type numericKey struct {
	sign                           int8
	ipStart, ipLen, fpStart, fpLen int32
}

type numericLine struct {
	text string
	num  numericKey
	val  int64
}

type numericLineSet struct {
	buffers [][]byte
	lines   []numericLine
	allInt  bool
}

func (ls *numericLineSet) read(r io.Reader, zeroTerm bool) error {
	data, err := io.ReadAll(r)
	if err != nil {
		return err
	}
	ls.buffers = append(ls.buffers, data)
	if !splitNumericLines(data, &ls.lines, zeroTerm) {
		ls.allInt = false
	}
	return nil
}

func parseNumericKey(s string) numericKey {
	i := 0
	for i < len(s) && isBlank(s[i]) {
		i++
	}
	neg := false
	if i < len(s) && s[i] == '-' {
		neg = true
		i++
	}
	j := i
	for j < len(s) && isDigit(s[j]) {
		j++
	}
	ipStart, ipEnd := i, j
	for ipStart < ipEnd && s[ipStart] == '0' {
		ipStart++
	}
	fpStart, fpEnd := 0, 0
	if j < len(s) && s[j] == '.' {
		k := j + 1
		for k < len(s) && isDigit(s[k]) {
			k++
		}
		fpStart, fpEnd = j+1, k
		for fpEnd > fpStart && s[fpEnd-1] == '0' {
			fpEnd--
		}
	}
	if ipStart == ipEnd && fpStart == fpEnd {
		return numericKey{}
	}
	sign := int8(1)
	if neg {
		sign = -1
	}
	return numericKey{
		sign:    sign,
		ipStart: int32(ipStart),
		ipLen:   int32(ipEnd - ipStart),
		fpStart: int32(fpStart),
		fpLen:   int32(fpEnd - fpStart),
	}
}

func compareNumericKey(as string, a numericKey, bs string, b numericKey) int {
	if a.sign != b.sign {
		return cmpInt(int(a.sign), int(b.sign))
	}
	m := cmpInt(int(a.ipLen), int(b.ipLen))
	if m == 0 {
		ai, bi := int(a.ipStart), int(b.ipStart)
		m = strings.Compare(as[ai:ai+int(a.ipLen)], bs[bi:bi+int(b.ipLen)])
	}
	if m == 0 {
		af, bf := int(a.fpStart), int(b.fpStart)
		m = compareFraction(as[af:af+int(a.fpLen)], bs[bf:bf+int(b.fpLen)])
	}
	if a.sign < 0 {
		return -m
	}
	return m
}

func (s *sorter) compareNumericLines(a, b numericLine) int {
	if d := compareNumericKey(a.text, a.num, b.text, b.num); d != 0 || s.stable || s.unique {
		if d != 0 && s.keys[0].opts.reverse {
			return -d
		}
		return d
	}
	d := s.textCompare(a.text, b.text)
	if s.reverse {
		return -d
	}
	return d
}

func (s *sorter) checkSorted(rc *tool.RunContext, input inputOperand) int {
	var lines lineSet
	if err := lines.read(input.reader, s.zeroTerm); err != nil {
		fmt.Fprintf(rc.Err, "sort: cannot read: %s: %v\n", input.name, pathErr(err))
		return 2
	}
	for i := 1; i < len(lines.lines); i++ {
		d := s.compare(lines.lines[i-1], lines.lines[i])
		if err := s.collationErr(); err != nil {
			return s.collationFailure(rc, err)
		}
		if d > 0 || (s.unique && d == 0) {
			if !s.checkSilent {
				fmt.Fprintf(rc.Err, "sort: %s:%d: disorder: %s\n", input.name, i+1, lines.lines[i])
			}
			return 1
		}
	}
	return 0
}

// mergeFiles implements POSIX -m: a true k-way merge of runs that are
// each assumed already sorted by the active ordering options. The runs
// are interleaved lazily — disorder inside a run is passed through, not
// re-sorted — and ties take the earlier file, so the merge is stable
// across operands. Global -r is already reflected in s.compare, so
// reverse-sorted runs merge without a separate reversal pass.
func (s *sorter) mergeFiles(rc *tool.RunContext, inputs []inputOperand, output string) int {
	runs := make([][]string, 0, len(inputs))
	for _, input := range inputs {
		var ls lineSet
		if err := ls.read(input.reader, s.zeroTerm); err != nil {
			fmt.Fprintf(rc.Err, "sort: cannot read: %s: %v\n", input.name, pathErr(err))
			return 2
		}
		runs = append(runs, ls.lines)
	}
	// Pairwise-merge the sorted runs until one remains; mergeRuns
	// prefers the left (earlier-file) run on ties.
	for len(runs) > 1 {
		next := make([][]string, 0, len(runs)/2+1)
		i := 0
		for ; i+1 < len(runs); i += 2 {
			a, b := runs[i], runs[i+1]
			dst := make([]string, len(a)+len(b))
			mergeRuns(dst, a, b, s.compare)
			next = append(next, dst)
		}
		if i < len(runs) {
			next = append(next, runs[i])
		}
		runs = next
	}
	if err := s.collationErr(); err != nil {
		return s.collationFailure(rc, err)
	}
	merged := runs[0]
	if s.unique {
		out := merged[:0]
		for i, l := range merged {
			if i == 0 || s.compareEqual(out[len(out)-1], l) != 0 {
				out = append(out, l)
			}
		}
		merged = out
		if err := s.collationErr(); err != nil {
			return s.collationFailure(rc, err)
		}
	}
	return writeStringLines(rc, output, merged, s.zeroTerm)
}

func (s *sorter) randomShuffle(rc *tool.RunContext, inputs []inputOperand, output string) int {
	var lines lineSet
	for _, input := range inputs {
		if err := lines.read(input.reader, s.zeroTerm); err != nil {
			fmt.Fprintf(rc.Err, "sort: cannot read: %s: %v\n", input.name, pathErr(err))
			return 2
		}
	}
	src := s.randSrc
	if src == nil {
		src = rand.Reader
	}
	rnd := make([]uint64, len(lines.lines))
	var b [8]byte
	for i := range lines.lines {
		if _, err := io.ReadFull(src, b[:]); err != nil {
			fmt.Fprintf(rc.Err, "sort: random-source exhausted\n")
			return 2
		}
		rnd[i] = binary.LittleEndian.Uint64(b[:])
	}
	sort.SliceStable(lines.lines, func(i, j int) bool {
		return rnd[i] < rnd[j]
	})
	if s.unique {
		out := lines.lines[:0]
		for i, l := range lines.lines {
			if i == 0 || s.compareEqual(out[len(out)-1], l) != 0 {
				out = append(out, l)
			}
		}
		lines.lines = out
		if err := s.collationErr(); err != nil {
			return s.collationFailure(rc, err)
		}
	}
	return writeStringLines(rc, output, lines.lines, s.zeroTerm)
}

type lineSet struct {
	buffers [][]byte
	lines   []string
}

func (ls *lineSet) read(r io.Reader, zeroTerm bool) error {
	data, err := io.ReadAll(r)
	if err != nil {
		return err
	}
	ls.buffers = append(ls.buffers, data)
	splitLines(data, &ls.lines, zeroTerm)
	return nil
}

type inputOperand struct {
	name   string
	reader io.Reader
	closer io.Closer
}

// openOperands validates every named operand before any operand is consumed.
// This prevents an endless first input from hiding an invalid later operand.
// Each "-" retains the shared stdin reader so duplicate stdin operands keep
// their normal sequential EOF behavior.
func openOperands(
	rc *tool.RunContext,
	operands []string,
	open func(string) (io.ReadCloser, error),
) ([]inputOperand, string, error) {
	inputs := make([]inputOperand, 0, len(operands))
	for _, operand := range operands {
		if operand == "-" {
			inputs = append(inputs, inputOperand{name: operand, reader: rc.In})
			continue
		}
		f, err := open(rc.Path(operand))
		if err != nil {
			closeOperands(inputs)
			return nil, operand, err
		}
		inputs = append(inputs, inputOperand{name: operand, reader: f, closer: f})
	}
	return inputs, "", nil
}

func closeOperands(inputs []inputOperand) {
	for _, input := range inputs {
		if input.closer != nil {
			_ = input.closer.Close()
		}
	}
}

func splitLines(data []byte, lines *[]string, zeroTerm bool) {
	delim := byte('\n')
	if zeroTerm {
		delim = 0
	}
	if len(data) == 0 {
		return
	}
	if data[len(data)-1] == delim {
		data = data[:len(data)-1]
	}
	if len(data) == 0 {
		*lines = append(*lines, "")
		return
	}
	n := bytes.Count(data, []byte{delim}) + 1
	old := len(*lines)
	if cap(*lines)-old < n {
		next := make([]string, old, old+n)
		copy(next, *lines)
		*lines = next
	}
	for {
		i := bytes.IndexByte(data, delim)
		if i < 0 {
			*lines = append(*lines, bytesString(data))
			return
		}
		*lines = append(*lines, bytesString(data[:i]))
		data = data[i+1:]
	}
}

func splitNumericLines(data []byte, lines *[]numericLine, zeroTerm bool) bool {
	delim := byte('\n')
	if zeroTerm {
		delim = 0
	}
	allInt := true
	if len(data) == 0 {
		return allInt
	}
	if data[len(data)-1] == delim {
		data = data[:len(data)-1]
	}
	if len(data) == 0 {
		*lines = append(*lines, numericLine{})
		return allInt
	}
	n := bytes.Count(data, []byte{delim}) + 1
	old := len(*lines)
	if cap(*lines)-old < n {
		next := make([]numericLine, old, old+n)
		copy(next, *lines)
		*lines = next
	}
	appendLine := func(l string) {
		num := parseNumericKey(l)
		val, ok := intKeyVal(l, num)
		if !ok {
			allInt = false
		}
		*lines = append(*lines, numericLine{text: l, num: num, val: val})
	}
	for {
		i := bytes.IndexByte(data, delim)
		if i < 0 {
			appendLine(bytesString(data))
			return allInt
		}
		appendLine(bytesString(data[:i]))
		data = data[i+1:]
	}
}

func bytesString(b []byte) string {
	if len(b) == 0 {
		return ""
	}
	return unsafe.String(unsafe.SliceData(b), len(b))
}

func readFiles0From(rc *tool.RunContext, path string) ([]string, error) {
	var data []byte
	var err error
	if path == "-" {
		data, err = io.ReadAll(rc.In)
	} else {
		data, err = os.ReadFile(rc.Path(path))
	}
	if err != nil {
		return nil, err
	}
	var files []string
	for _, name := range bytes.Split(data, []byte{0}) {
		s := string(name)
		if s != "" {
			files = append(files, s)
		}
	}
	return files, nil
}

func pathErr(err error) error {
	return tool.SysErr(err)
}
