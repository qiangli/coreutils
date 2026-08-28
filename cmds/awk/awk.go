// Package awkcmd implements awk(1) by embedding GoAWK.
//
// Backed by github.com/benhoyt/goawk (MIT).
package awkcmd

import (
	"fmt"
	"io"
	"os"
	"regexp"
	"runtime"
	"strings"

	"github.com/benhoyt/goawk/interp"
	"github.com/benhoyt/goawk/lexer"
	"github.com/benhoyt/goawk/parser"
	awkregex "github.com/benhoyt/goawk/regex"
	"github.com/qiangli/coreutils/pkg/bre"
	"github.com/qiangli/coreutils/pkg/collate"
	"github.com/qiangli/coreutils/pkg/ctype"
	"github.com/qiangli/coreutils/pkg/locale"
	"github.com/qiangli/coreutils/tool"
	"golang.org/x/text/cases"
	textcollate "golang.org/x/text/collate"
	"golang.org/x/text/language"
)

// assignRegex matches a var=value operand or -v option-argument; the name
// part mirrors GoAWK's own ARGV assignment detection (interp varRegex).
var assignRegex = regexp.MustCompile(`^([_a-zA-Z][_a-zA-Z0-9]*)=`)

var cmd = &tool.Tool{
	Name:     "awk",
	Synopsis: "Pattern scanning and text processing language, backed by pure-Go GoAWK.",
	Usage:    "awk [-F fs] [-v var=val] [-f progfile | 'program'] [file ...]",
}

func init() { cmd.Run = run; tool.Register(cmd) }

func run(rc *tool.RunContext, args []string) int {
	return runWithLocales(rc, args, func(name string) (ctypeProvider, error) { return ctype.Open(name) }, func(name string) (collateProvider, error) { return collate.Open(name) })
}

type ctypeProvider interface {
	bre.ByteCtype
	ToUpper([]byte) ([]byte, error)
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

func runWithCType(rc *tool.RunContext, args []string, opener ctypeOpener) int {
	return runWithLocales(rc, args, opener, func(string) (collateProvider, error) {
		return identityCollation{}, nil
	})
}

type identityCollation struct{}

func (identityCollation) Equivalents(value byte) ([]byte, error) { return []byte{value}, nil }
func (identityCollation) EquivalenceClasses() ([]bool, error) {
	result := make([]bool, 256)
	for i := range result {
		result[i] = true
	}
	return result, nil
}
func (identityCollation) CollationWeights() ([]byte, error) {
	result := make([]byte, 256)
	for i := range result {
		result[i] = byte(i)
	}
	return result, nil
}
func (identityCollation) CollatingElements() ([]bool, error) {
	result := make([]bool, 256)
	for i := range result {
		result[i] = true
	}
	return result, nil
}
func (identityCollation) Compare(a, b string) (int, error) { return strings.Compare(a, b), nil }
func (identityCollation) Close() error                     { return nil }

func runWithLocales(rc *tool.RunContext, args []string, ctypeOpen ctypeOpener, collateOpen collateOpener) (status int) {
	fs := tool.NewFlags(cmd.Name)
	// Option processing ends at the first operand (the program text or,
	// with -f, the first input file) — anything after it is a file operand
	// or var=value assignment, per POSIX awk and the gawk manual.
	fs.SetInterspersed(false)
	fieldSep := fs.StringP("field-separator", "F", "", "use fs for the input field separator")
	var assigns []string
	fs.StringArrayVarP(&assigns, "assign", "v", nil, "assign var=value before program execution")
	var progFiles []string
	fs.StringArrayVarP(&progFiles, "file", "f", nil, "read awk program source from progfile")
	operands, code := tool.Parse(rc, cmd, fs, args)
	if code >= 0 {
		return code
	}

	var source string
	var files []string
	if len(progFiles) > 0 {
		files = operands
	} else {
		if len(operands) == 0 {
			return tool.UsageError(rc, cmd, "missing program")
		}
		source = operands[0]
		files = operands[1:]
	}

	vars := []string{}
	if fs.Changed("field-separator") {
		// POSIX: -F sepstring is equivalent to -v FS=sepstring, so the
		// value undergoes the same escape processing (-F '\t' is a tab).
		vars = append(vars, "FS", unescape(*fieldSep))
	}
	for _, assign := range assigns {
		if !assignRegex.MatchString(assign) {
			return tool.UsageError(rc, cmd, "invalid -v assignment %q", assign)
		}
		name, value, _ := strings.Cut(assign, "=")
		// POSIX: -v values undergo escape-sequence processing.
		vars = append(vars, name, unescape(value))
	}

	compiler := awkERECompiler{}
	lcCType := locale.Resolve(rc.Env, locale.CType)
	lcCollate := locale.Resolve(rc.Env, locale.Collate)
	lcNumeric := locale.Resolve(rc.Env, locale.Numeric)
	decimalPoint, ok := awkNumericDecimalPoint(lcNumeric)
	if !ok {
		fmt.Fprintf(rc.Err, "awk: LC_NUMERIC %q: unsupported locale; expected C/POSIX, a carried UTF-8 locale, or de_DE.ISO-8859-1\n", lcNumeric)
		return 2
	}
	var tables *bre.LocaleByteTables
	ctypeTag, chars := awkUTF8Tag(lcCType)
	lower, upper := asciiCaseMaps()
	if !awkCLocale(lcCType) && !chars {
		provider, err := ctypeOpen(lcCType)
		if err != nil {
			fmt.Fprintf(rc.Err, "awk: LC_CTYPE %q: %v\n", lcCType, err)
			return 2
		}
		var snapshotErr error
		tables, snapshotErr = bre.SnapshotLocaleByteCtypeTables(provider)
		if snapshotErr == nil {
			lower, upper, snapshotErr = snapshotCaseMaps(provider)
		}
		closeErr := provider.Close()
		if snapshotErr != nil {
			fmt.Fprintf(rc.Err, "awk: LC_CTYPE %q: %v\n", lcCType, snapshotErr)
			return 2
		}
		if closeErr != nil {
			fmt.Fprintf(rc.Err, "awk: LC_CTYPE %q: %v\n", lcCType, closeErr)
			return 2
		}
	} else {
		tables, _ = bre.SnapshotLocaleByteCtypeTables(nil)
	}
	var stringCompare interp.StringCompareFunc
	collateTag, collateUTF8 := awkUTF8Tag(lcCollate)
	if collateUTF8 && !awkCUTF8Locale(lcCollate) {
		collator := textcollate.New(collateTag)
		stringCompare = func(a, b string) (int, error) { return collator.CompareString(a, b), nil }
	} else if !awkCLocale(lcCollate) && !collateUTF8 {
		provider, err := collateOpen(lcCollate)
		if err != nil {
			fmt.Fprintf(rc.Err, "awk: LC_COLLATE %q: %v\n", lcCollate, err)
			return 2
		}
		var snapshotErr error
		tables, snapshotErr = tables.WithCollation(provider)
		if snapshotErr != nil {
			_ = provider.Close()
			fmt.Fprintf(rc.Err, "awk: LC_COLLATE %q: %v\n", lcCollate, snapshotErr)
			return 2
		}
		stringCompare = provider.Compare
		defer func() {
			if err := provider.Close(); err != nil {
				fmt.Fprintf(rc.Err, "awk: LC_COLLATE %q: %v\n", lcCollate, err)
				if status == 0 {
					status = 2
				}
			}
		}()
	}
	if !awkCLocale(lcCType) && !chars || !awkCLocale(lcCollate) && !collateUTF8 {
		compiler.tables = tables
	}
	compiler.utf8 = chars
	if len(progFiles) > 0 {
		src, ok := readProgramFiles(rc, progFiles)
		if !ok {
			return 2
		}
		source = src
	}

	prog, err := parser.ParseProgram([]byte(source), &parser.ParserConfig{
		RegexCompiler: compiler,
	})
	if err != nil {
		fmt.Fprintf(rc.Err, "awk: %v\n", err)
		return 2
	}

	argv0 := rc.InvocationName
	if argv0 == "" {
		argv0 = cmd.Name
	}
	shellCommand := []string(nil)
	if runtime.GOOS != "windows" {
		if shell := rc.ResolveCommand("sh"); shell != "" {
			shellCommand = []string{shell, "-c"}
		}
	}
	status, err = interp.ExecProgram(prog, &interp.Config{
		Stdin:  readerOrEmpty(rc.In),
		Output: rc.Out,
		// Deterministic LF output on every platform (GoAWK's default
		// SmartNewlineMode emits CRLF on Windows, violating the LC_ALL=C
		// no-platform-variance contract).
		NewlineOutput: interp.RawNewlineMode,
		Error:         rc.Err,
		DecimalPoint:  decimalPoint,
		Chars:         chars,
		StringCompare: stringCompare,
		ToLower:       caseMapper(lower, ctypeTag, chars, false),
		ToUpper:       caseMapper(upper, ctypeTag, chars, true),
		Argv0:         argv0,
		Args:          resolveFiles(rc, files),
		Vars:          vars,
		Environ:       environPairs(rc.Env),
		OpenFile: func(name string, flag int, perm os.FileMode) (*os.File, error) {
			return rc.OpenFile(rc.Path(name), flag, perm)
		},
		ShellCommand: shellCommand,
		CommandDir:   rc.Dir,
		CommandEnv:   append([]string{}, rc.Env...),
	})
	if err != nil {
		fmt.Fprintf(rc.Err, "awk: %v\n", err)
		return 2
	}
	return status
}

func awkNumericDecimalPoint(name string) (byte, bool) {
	if awkCLocale(name) || awkUTF8Locale(name) && !strings.EqualFold(awkLocaleBase(name), "de_DE") {
		return '.', true
	}
	switch strings.ToLower(name) {
	case "de_de.iso-8859-1", "de_de.iso88591", "de_de.utf-8", "de_de.utf8":
		return ',', true
	default:
		return 0, false
	}
}

func awkCLocale(name string) bool { return name == "C" || name == "POSIX" }

func awkUTF8Locale(name string) bool {
	_, ok := awkUTF8Tag(name)
	return ok
}

func awkUTF8Tag(name string) (language.Tag, bool) {
	base, codeset := awkLocaleParts(name)
	codeset = strings.ToUpper(strings.NewReplacer("-", "", "_", "").Replace(codeset))
	if codeset != "UTF8" {
		return language.Und, false
	}
	switch {
	case base == "C" || base == "POSIX", strings.EqualFold(base, "en_US"):
		return language.AmericanEnglish, true
	case strings.EqualFold(base, "de_DE"):
		return language.German, true
	case strings.EqualFold(base, "ja_JP"):
		return language.Japanese, true
	default:
		return language.Und, false
	}
}

func awkCUTF8Locale(name string) bool {
	base, _ := awkLocaleParts(name)
	return (base == "C" || base == "POSIX") && awkUTF8Locale(name)
}

func awkLocaleBase(name string) string {
	base, _ := awkLocaleParts(name)
	return base
}

func awkLocaleParts(name string) (string, string) {
	name, _, _ = strings.Cut(name, "@")
	base, codeset, _ := strings.Cut(name, ".")
	return base, codeset
}

func asciiCaseMaps() (lower, upper [256]byte) {
	for i := range 256 {
		lower[i], upper[i] = byte(i), byte(i)
	}
	for b := byte('A'); b <= 'Z'; b++ {
		lower[b] = b + ('a' - 'A')
	}
	for b := byte('a'); b <= 'z'; b++ {
		upper[b] = b - ('a' - 'A')
	}
	return lower, upper
}

func snapshotCaseMaps(provider ctypeProvider) (lower, upper [256]byte, err error) {
	all := make([]byte, 256)
	for i := range all {
		all[i] = byte(i)
	}
	lowerBytes, err := provider.ToLower(all)
	if err != nil {
		return lower, upper, err
	}
	upperBytes, err := provider.ToUpper(all)
	if err != nil {
		return lower, upper, err
	}
	if len(lowerBytes) != 256 || len(upperBytes) != 256 {
		return lower, upper, fmt.Errorf("locale case map returned invalid table size")
	}
	copy(lower[:], lowerBytes)
	copy(upper[:], upperBytes)
	return lower, upper, nil
}

func caseMapper(table [256]byte, tag language.Tag, utf8Locale, upper bool) interp.CaseMapFunc {
	if utf8Locale {
		mapper := cases.Lower(tag)
		if upper {
			mapper = cases.Upper(tag)
		}
		return func(s string) (string, error) { return mapper.String(s), nil }
	}
	return func(s string) (string, error) {
		mapped := []byte(s)
		for i := range mapped {
			mapped[i] = table[mapped[i]]
		}
		return string(mapped), nil
	}
}

// awkERECompiler adapts coreutils' POSIX ERE matcher to GoAWK's expression
// regex seam. AWK requires period to match newline and matching to select the
// longest of the leftmost matches. Keep the unmodified AWK source separately:
// pkg/bre translates some ERE constructs before compiling them, but GoAWK uses
// String for stable diagnostics and disassembly.
type awkERECompiler struct {
	tables *bre.LocaleByteTables
	utf8   bool
}

func (c awkERECompiler) Compile(source string) (awkregex.Regexp, error) {
	if c.tables != nil {
		re, err := bre.CompileLocaleByteRegexpTables([]byte(source), c.tables, bre.ByteRegexpOptions{
			Syntax: bre.ByteRegexpERE, DotAll: true,
		})
		if err != nil {
			return nil, err
		}
		return &awkLocaleERERegexp{source: source, re: re}, nil
	}
	if c.utf8 {
		re, err := bre.CompileCUTF8WithFlags(source, "(?s)", true, nil)
		if err != nil {
			return nil, err
		}
		re.Longest()
		return &awkERERegexp{source: source, re: re}, nil
	}
	re, err := bre.CompileEREWithFlags(source, "(?s)")
	if err != nil {
		return nil, err
	}
	re.Longest()
	return &awkERERegexp{source: source, re: re}, nil
}

type awkERERegexp struct {
	source string
	re     awkEREBackend
}

type awkEREBackend interface {
	MatchString(string) bool
	FindStringIndex(string) []int
	FindAllStringSubmatchIndex(string, int) [][]int
}

func (r *awkERERegexp) String() string { return r.source }
func (r *awkERERegexp) MatchString(s string) (bool, error) {
	if re, ok := r.re.(*bre.Regexp); ok {
		return re.MatchStringErr(s)
	}
	return r.re.MatchString(s), nil
}
func (r *awkERERegexp) FindStringIndex(s string) ([]int, error) {
	if re, ok := r.re.(*bre.Regexp); ok {
		return re.FindStringIndexErr(s)
	}
	return r.re.FindStringIndex(s), nil
}
func (r *awkERERegexp) FindAllStringIndex(s string, n int) ([][]int, error) {
	var matches [][]int
	if re, ok := r.re.(*bre.Regexp); ok {
		var err error
		matches, err = re.FindAllStringSubmatchIndexErr(s, n)
		if err != nil {
			return nil, err
		}
	} else {
		matches = r.re.FindAllStringSubmatchIndex(s, n)
	}
	indices := make([][]int, len(matches))
	for i, match := range matches {
		indices[i] = match[:2]
	}
	return indices, nil
}
func (r *awkERERegexp) FindIndex(b []byte) ([]int, error) {
	return r.FindStringIndex(string(b))
}
func (r *awkERERegexp) Split(s string, n int) ([]string, error) {
	return splitRegexp(s, n, r.FindAllStringIndex)
}
func (r *awkERERegexp) ReplaceAllStringFunc(s string, repl func(string) string) (string, error) {
	return replaceRegexp(s, repl, r.FindAllStringIndex)
}

type awkLocaleERERegexp struct {
	source string
	re     *bre.LocaleByteRegexp
}

func (r *awkLocaleERERegexp) String() string                     { return r.source }
func (r *awkLocaleERERegexp) MatchString(s string) (bool, error) { return r.re.MatchString(s) }
func (r *awkLocaleERERegexp) FindStringIndex(s string) ([]int, error) {
	match, err := r.re.FindSubmatchIndex([]byte(s))
	if err != nil || match == nil {
		return match, err
	}
	return match[:2], nil
}
func (r *awkLocaleERERegexp) FindAllStringIndex(s string, n int) ([][]int, error) {
	matches, err := r.re.FindAllStringSubmatchIndex(s, n)
	if err != nil {
		return nil, err
	}
	indices := make([][]int, len(matches))
	for i, match := range matches {
		indices[i] = match[:2]
	}
	return indices, nil
}
func (r *awkLocaleERERegexp) FindIndex(b []byte) ([]int, error) {
	match, err := r.re.FindSubmatchIndex(b)
	if err != nil || match == nil {
		return match, err
	}
	return match[:2], nil
}
func (r *awkLocaleERERegexp) Split(s string, n int) ([]string, error) {
	return splitRegexp(s, n, r.FindAllStringIndex)
}

type findAllFunc func(string, int) ([][]int, error)

func splitRegexp(s string, n int, findAll findAllFunc) ([]string, error) {
	if n == 0 {
		return nil, nil
	}
	if s == "" {
		return []string{""}, nil
	}
	matches, err := findAll(s, n)
	if err != nil {
		return nil, err
	}
	parts := make([]string, 0, len(matches)+1)
	begin, end := 0, 0
	for _, match := range matches {
		if n > 0 && len(parts) >= n-1 {
			break
		}
		end = match[0]
		if match[1] != 0 {
			parts = append(parts, s[begin:end])
		}
		begin = match[1]
	}
	if end != len(s) {
		parts = append(parts, s[begin:])
	}
	return parts, nil
}
func (r *awkLocaleERERegexp) ReplaceAllStringFunc(s string, repl func(string) string) (string, error) {
	return replaceRegexp(s, repl, r.FindAllStringIndex)
}

func replaceRegexp(s string, repl func(string) string, findAll findAllFunc) (string, error) {
	matches, err := findAll(s, -1)
	if err != nil {
		return "", err
	}
	var b strings.Builder
	last := 0
	for _, match := range matches {
		b.WriteString(s[last:match[0]])
		b.WriteString(repl(s[match[0]:match[1]]))
		last = match[1]
	}
	b.WriteString(s[last:])
	return b.String(), nil
}

func readProgramFiles(rc *tool.RunContext, names []string) (string, bool) {
	var b strings.Builder
	for i, name := range names {
		var data []byte
		var err error
		if name == "-" {
			data, err = io.ReadAll(readerOrEmpty(rc.In))
		} else {
			var f *os.File
			f, err = rc.OpenFile(rc.Path(name), os.O_RDONLY, 0)
			if err == nil {
				data, err = io.ReadAll(f)
				if closeErr := f.Close(); err == nil {
					err = closeErr
				}
			}
		}
		if err != nil {
			fmt.Fprintf(rc.Err, "awk: %s: %v\n", name, err)
			return "", false
		}
		if i > 0 {
			b.WriteByte('\n')
		}
		b.Write(data)
	}
	return b.String(), true
}

func resolveFiles(_ *tool.RunContext, files []string) []string {
	// POSIX exposes the operand spelling through ARGV and FILENAME. File opens
	// are routed separately through RunContext.OpenFile, so embedded callers do
	// not need to sacrifice that visible value to honor a virtual cwd.
	return append([]string(nil), files...)
}

func unescape(s string) string {
	u, err := lexer.Unescape(s)
	if err != nil {
		return s
	}
	return u
}

func environPairs(env []string) []string {
	pairs := make([]string, 0, len(env)*2)
	for _, entry := range env {
		name, value, ok := strings.Cut(entry, "=")
		if !ok {
			continue
		}
		pairs = append(pairs, name, value)
	}
	return pairs
}

func readerOrEmpty(r io.Reader) io.Reader {
	if r != nil {
		return r
	}
	return strings.NewReader("")
}
