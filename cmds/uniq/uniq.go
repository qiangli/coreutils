// Portions adapted from https://github.com/u-root/u-root cmds/core/uniq/uniq.go (BSD-3-Clause).
// Changes: rewired to tool framework; added -f/-s/-w key extraction, -c GNU "%7d "
// count format, first-of-group output buffering, and the [INPUT [OUTPUT]] operands.

// Package uniqcmd implements uniq(1) per the GNU coreutils manual:
// filter adjacent matching lines from INPUT (or standard input),
// writing to OUTPUT (or standard output).
//
// Implemented flags: -c -d -u -i -f N -s N -w N. A field is the maximal
// string matched by the POSIX basic regular expression
// [[:blank:]]*[^[:blank:]]*, and -s skips characters after those fields.
// Both the <blank> set and the character unit come from the invocation's
// LC_CTYPE in POSIX mode (Issue 7 XCU uniq ENVIRONMENT VARIABLES);
// outside POSIX mode the historical C/byte model is kept. Comparisons are
// byte-wise, with -i folding ASCII case (C-locale semantics).
package uniqcmd

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/qiangli/coreutils/pkg/ctype"
	"github.com/qiangli/coreutils/pkg/locale"
	"github.com/qiangli/coreutils/tool"
)

var cmd = &tool.Tool{
	Name:     "uniq",
	Synopsis: "Filter adjacent matching lines from INPUT, writing to OUTPUT.",
	Usage:    "uniq [OPTION]... [INPUT [OUTPUT]]\n\nWith no INPUT, or when INPUT is -, read standard input.",
}

// Run is wired in init: a literal would create an initialization
// cycle (run's flag-error paths reference cmd).
func init() { cmd.Run = run; tool.Register(cmd) }

func run(rc *tool.RunContext, args []string) int {
	fs := tool.NewFlags(cmd.Name)
	posix := envPresent(rc.Env, "POSIXLY_CORRECT")
	if !posix {
		args = tool.AliasHelpVersion(args)
	}
	count := fs.BoolP("count", "c", false, "prefix lines by the number of occurrences")
	repeated := fs.BoolP("repeated", "d", false, "only print duplicate lines, one for each group")
	unique := fs.BoolP("unique", "u", false, "only print unique lines")
	ignoreCase := fs.BoolP("ignore-case", "i", false, "ignore differences in case when comparing")
	allRepeated := fs.StringP("all-repeated", "D", "", "print all duplicate lines; delimit-method may be none, prepend, or separate")
	fs.Lookup("all-repeated").NoOptDefVal = "none"
	group := fs.String("group", "", "show all items, separating groups with METHOD: separate, prepend, append, or both")
	fs.Lookup("group").NoOptDefVal = "separate"
	zero := fs.BoolP("zero-terminated", "z", false, "line delimiter is NUL, not newline")
	skipFields := fs.IntP("skip-fields", "f", 0, "avoid comparing the first N fields")
	skipChars := fs.IntP("skip-chars", "s", 0, "avoid comparing the first N characters")
	checkChars := fs.IntP("check-chars", "w", 0, "compare no more than N characters in lines")
	args = normalizeArgs(args, posix)
	var operands []string
	var code int
	if posix {
		operands, code = tool.ParseRequireOrder(rc, cmd, fs, args)
	} else {
		operands, code = tool.Parse(rc, cmd, fs, args)
	}
	if code >= 0 {
		return code
	}

	if *skipFields < 0 || posix && fs.Changed("skip-fields") && *skipFields == 0 {
		return tool.UsageError(rc, cmd, "invalid number of fields to skip: '%d'", *skipFields)
	}
	if *skipChars < 0 || posix && fs.Changed("skip-chars") && *skipChars == 0 {
		return tool.UsageError(rc, cmd, "invalid number of bytes to skip: '%d'", *skipChars)
	}
	if *checkChars < 0 {
		return tool.UsageError(rc, cmd, "invalid number of bytes to compare: '%d'", *checkChars)
	}
	if len(operands) > 2 {
		return tool.UsageError(rc, cmd, "extra operand '%s'", operands[2])
	}
	model, err := loadKeyModel(rc.Env, posix)
	if err != nil {
		fmt.Fprintf(rc.Err, "uniq: %v\n", err)
		return 1
	}
	delimMode := delimNone
	if fs.Changed("group") {
		if *count || *repeated || fs.Changed("all-repeated") || *unique {
			return tool.UsageError(rc, cmd, "--group is mutually exclusive with -c, -d, -D, and -u")
		}
		var ok bool
		delimMode, ok = parseDelimMode(*group, true)
		if !ok {
			return tool.UsageError(rc, cmd, "invalid group method %q", *group)
		}
	} else if fs.Changed("all-repeated") {
		var ok bool
		delimMode, ok = parseDelimMode(*allRepeated, false)
		if !ok {
			return tool.UsageError(rc, cmd, "invalid delimit method %q", *allRepeated)
		}
		if *count {
			return tool.UsageError(rc, cmd, "printing all duplicated lines and repeat counts is meaningless")
		}
		*repeated = true
	}

	input := "-"
	if len(operands) > 0 {
		input = operands[0]
	}
	lineEnd := byte('\n')
	if *zero {
		lineEnd = 0
	}
	lines, err := readLines(rc, input, lineEnd)
	if err != nil {
		fmt.Fprintf(rc.Err, "uniq: %s: %v\n", input, pathErr(err))
		return 1
	}

	var w io.Writer = rc.Out
	if len(operands) == 2 && operands[1] != "-" {
		f, err := os.Create(rc.Path(operands[1]))
		if err != nil {
			fmt.Fprintf(rc.Err, "uniq: %s: %v\n", operands[1], pathErr(err))
			return 1
		}
		defer f.Close()
		w = f
	}
	bw := bufio.NewWriter(w)

	limitChars := fs.Changed("check-chars")
	keyOf := func(line string) string {
		k := skipKey(line, *skipFields, *skipChars, model)
		if limitChars && len(k) > *checkChars {
			k = k[:*checkChars]
		}
		return k
	}
	equal := func(a, b string) bool {
		if *ignoreCase {
			return asciiEqualFold(a, b)
		}
		return a == b
	}

	writeTerm := func() {
		_ = bw.WriteByte(lineEnd)
	}
	writeLine := func(line string, n int) {
		if *count {
			fmt.Fprintf(bw, "%7d %s", n, line)
		} else {
			fmt.Fprint(bw, line)
		}
		writeTerm()
	}
	shouldPrint := func(group []string) bool {
		n := len(group)
		// GNU shouldPrint: -d drops singleton groups, -u drops repeated
		// groups (so -d -u prints nothing).
		if (*repeated && n == 1) || (*unique && n > 1) {
			return false
		}
		return true
	}
	flush := func(group []string) bool {
		n := len(group)
		if !shouldPrint(group) {
			return false
		}
		if fs.Changed("group") || fs.Changed("all-repeated") {
			for _, line := range group {
				writeLine(line, n)
			}
		} else {
			writeLine(group[0], n)
		}
		return true
	}

	var groupLines []string
	firstPrinted := false
	var prevKey string
	for _, line := range lines {
		k := keyOf(line)
		if len(groupLines) > 0 && equal(prevKey, k) {
			groupLines = append(groupLines, line)
			continue
		}
		if len(groupLines) > 0 {
			firstPrinted = flushWithDelimiter(bw, groupLines, shouldPrint(groupLines), flush, delimMode, firstPrinted, lineEnd)
		}
		groupLines, prevKey = []string{line}, k
	}
	if len(groupLines) > 0 {
		firstPrinted = flushWithDelimiter(bw, groupLines, shouldPrint(groupLines), flush, delimMode, firstPrinted, lineEnd)
	}
	if err := bw.Flush(); err != nil {
		fmt.Fprintf(rc.Err, "uniq: write failed: %v\n", err)
		return 1
	}
	return 0
}

// normalizeArgs supports GNU's -D[delimit-method] spelling and POSIX's
// obsolescent -N/+N forms for skipping fields/chars.
func normalizeArgs(args []string, posix bool) []string {
	out := make([]string, 0, len(args))
	rest := false
	needsValue := false
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--" {
			rest = true
			out = append(out, arg)
			needsValue = false
			continue
		}
		if rest {
			out = append(out, arg)
			continue
		}
		if needsValue {
			out = append(out, arg)
			needsValue = false
			continue
		}
		if posix && (arg == "-" || !strings.HasPrefix(arg, "-") && !strings.HasPrefix(arg, "+")) {
			rest = true
			out = append(out, arg)
			continue
		}
		if len(arg) > 2 && arg[0] == '-' && arg[1] == 'D' && arg[2] != '=' {
			out = append(out, "-D="+arg[2:])
			continue
		}
		// uutils documents the delimiter method as a separate argument to
		// -D, while GNU also accepts the optional argument attached to it.
		// Consume only recognized methods so an ordinary operand after -D
		// keeps its normal meaning.
		if arg == "-D" && i+1 < len(args) && isDelimiterMethod(args[i+1]) {
			out = append(out, "-D="+args[i+1])
			i++
			continue
		}
		if legacySkipNumber(arg) {
			switch arg[0] {
			case '-':
				out = append(out, "-f", arg[1:])
			case '+':
				out = append(out, "-s", arg[1:])
			}
			continue
		}
		out = append(out, arg)
		needsValue = optionNeedsValue(arg)
	}
	return out
}

func optionNeedsValue(arg string) bool {
	switch arg {
	case "-f", "--skip-fields", "-s", "--skip-chars", "-w", "--check-chars":
		return true
	default:
		return false
	}
}

func legacySkipNumber(arg string) bool {
	if len(arg) < 2 || (arg[0] != '-' && arg[0] != '+') {
		return false
	}
	for i := 1; i < len(arg); i++ {
		if arg[i] < '0' || arg[i] > '9' {
			return false
		}
	}
	return true
}

func isDelimiterMethod(s string) bool {
	switch s {
	case "none", "prepend", "separate":
		return true
	default:
		return false
	}
}

type delimiterMode int

const (
	delimNone delimiterMode = iota
	delimPrepend
	delimSeparate
	delimAppend
	delimBoth
)

func parseDelimMode(s string, group bool) (delimiterMode, bool) {
	switch s {
	case "", "none":
		return delimNone, !group
	case "prepend":
		return delimPrepend, true
	case "separate":
		return delimSeparate, true
	case "append":
		return delimAppend, group
	case "both":
		return delimBoth, group
	default:
		return delimNone, false
	}
}

func flushWithDelimiter(w *bufio.Writer, group []string, shouldPrint bool, flush func([]string) bool, mode delimiterMode, firstPrinted bool, lineEnd byte) bool {
	if !shouldPrint {
		return firstPrinted
	}
	printed := false
	if mode == delimPrepend || mode == delimBoth || (mode == delimSeparate && firstPrinted) {
		_ = w.WriteByte(lineEnd)
	}
	printed = flush(group)
	if printed && (mode == delimAppend || mode == delimBoth) {
		_ = w.WriteByte(lineEnd)
	}
	return firstPrinted || printed
}

// ctypeProvider is the narrow slice of pkg/ctype.Provider that uniq needs:
// LC_CTYPE says which characters constitute a <blank>.
type ctypeProvider interface {
	IsBlank(byte) (bool, error)
	Close() error
}

// openCTypeFn is the injection point for the single-byte LC_CTYPE provider.
var openCTypeFn = func(name string) (ctypeProvider, error) { return ctype.Open(name) }

// keyModel carries the LC_CTYPE-dependent halves of key extraction: the
// character unit that -s counts, and the <blank> set that delimits a field.
type keyModel struct {
	utf8  bool
	blank [256]bool
}

// cKeyModel is the C/POSIX model: one byte is one character and <blank> is
// exactly <space> and <tab>.
func cKeyModel() *keyModel {
	m := new(keyModel)
	m.blank[' '], m.blank['\t'] = true, true
	return m
}

// loadKeyModel resolves LC_CTYPE for the invocation. Outside POSIX mode the
// historical C/byte model is kept unconditionally, so no provider is opened
// and no locale can make uniq fail. In POSIX mode a UTF-8 codeset selects
// multi-byte characters with the locale's <blank> characters, and any other
// non-C locale is answered by the single-byte provider rather than silently
// inheriting C's <blank> set.
func loadKeyModel(env []string, posix bool) (*keyModel, error) {
	if !posix {
		return cKeyModel(), nil
	}
	name := locale.Resolve(env, locale.CType)
	if name == "C" || name == "POSIX" {
		return cKeyModel(), nil
	}
	if isUTF8Locale(name) {
		m := cKeyModel()
		m.utf8 = true
		return m, nil
	}
	p, err := openCTypeFn(name)
	if err != nil {
		return nil, fmt.Errorf("LC_CTYPE %q: %w", name, err)
	}
	m := new(keyModel)
	for i := 0; i < 256; i++ {
		ok, classifyErr := p.IsBlank(byte(i))
		if classifyErr != nil {
			err = classifyErr
			break
		}
		m.blank[i] = ok
	}
	if closeErr := p.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return nil, fmt.Errorf("LC_CTYPE %q: %w", name, err)
	}
	return m, nil
}

// isUTF8Locale reports whether the locale name selects the UTF-8 codeset.
func isUTF8Locale(name string) bool {
	name, _, _ = strings.Cut(name, "@")
	if dot := strings.LastIndexByte(name, '.'); dot >= 0 {
		name = name[dot+1:]
	}
	name = strings.ToUpper(strings.ReplaceAll(strings.ReplaceAll(name, "-", ""), "_", ""))
	return name == "UTF8"
}

// nextChar decodes one character of line at i and reports its width in
// bytes together with whether it is a <blank> in the selected locale. A
// byte that does not begin a valid multi-byte character is one character
// wide, so a malformed input line still advances and is never a <blank>.
func (m *keyModel) nextChar(line string, i int) (size int, blank bool) {
	if !m.utf8 {
		return 1, m.blank[line[i]]
	}
	r, size := utf8.DecodeRuneInString(line[i:])
	if r == utf8.RuneError && size <= 1 {
		return 1, false
	}
	return size, r == '\t' || unicode.Is(unicode.Zs, r)
}

// skipKey implements the Issue 7 uniq key: skip the first fields fields,
// where a field is the maximal string matched by the basic regular
// expression [[:blank:]]*[^[:blank:]]*, then skip chars characters. Both
// counts clamp to the end of the line, where the standard requires a null
// string to be used for comparison.
func skipKey(line string, fields, chars int, m *keyModel) string {
	i := 0
	for n := 0; n < fields && i < len(line); n++ {
		for i < len(line) {
			size, blank := m.nextChar(line, i)
			if !blank {
				break
			}
			i += size
		}
		for i < len(line) {
			size, blank := m.nextChar(line, i)
			if blank {
				break
			}
			i += size
		}
	}
	for n := 0; n < chars && i < len(line); n++ {
		size, _ := m.nextChar(line, i)
		i += size
	}
	if i > len(line) {
		i = len(line)
	}
	return line[i:]
}

func envPresent(env []string, key string) bool {
	prefix := key + "="
	for _, entry := range env {
		if entry == key || strings.HasPrefix(entry, prefix) {
			return true
		}
	}
	return false
}

// asciiEqualFold is C-locale case-insensitive equality (bytewise ASCII
// upcasing — deliberately not Unicode folding).
func asciiEqualFold(a, b string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := 0; i < len(a); i++ {
		if upperByte(a[i]) != upperByte(b[i]) {
			return false
		}
	}
	return true
}

func upperByte(c byte) byte {
	if c >= 'a' && c <= 'z' {
		return c - ('a' - 'A')
	}
	return c
}

func readLines(rc *tool.RunContext, operand string, lineEnd byte) ([]string, error) {
	var data []byte
	var err error
	if operand == "-" {
		data, err = io.ReadAll(rc.In)
	} else {
		data, err = os.ReadFile(rc.Path(operand))
	}
	if err != nil {
		return nil, err
	}
	if len(data) == 0 {
		return nil, nil
	}
	s := strings.TrimSuffix(string(data), string([]byte{lineEnd}))
	return strings.Split(s, string([]byte{lineEnd})), nil
}

func pathErr(err error) error {
	return tool.SysErr(err)
}
