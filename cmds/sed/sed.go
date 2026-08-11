// Package sedcmd implements a pure-Go drop-in for GNU sed: a stream editor that
// applies a script to each line of its input.
//
// The script engine is the vendored Go.Sed (MIT — see internal/gosed/LICENSE),
// adapted for GNU compatibility: patterns default to POSIX Basic Regular
// Expressions (BRE), switching to ERE under -E/-r, via coreutils/pkg/bre (the
// same matcher grep uses); s/// replacements use GNU `\1`/`&` form. The full
// command set is supported — s, y, d, D, p, P, n, N, g, G, h, H, x, b, t,
// :label, a, i, c, r, w, q, = and address ranges.
//
// Flags: -n (suppress auto-print), -e SCRIPT, -f FILE, -E/-r (ERE), -s (treat
// files separately), -i[SUFFIX] (edit in place). Unsupported flags fail loudly.
package sedcmd

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/qiangli/coreutils/cmds/sed/internal/gosed"
	"github.com/qiangli/coreutils/pkg/bre"
	"github.com/qiangli/coreutils/pkg/ctype"
	"github.com/qiangli/coreutils/pkg/locale"
	"github.com/qiangli/coreutils/pkg/nudge"
	"github.com/qiangli/coreutils/tool"
)

var cmd = &tool.Tool{
	Name:     "sed",
	Synopsis: "Stream editor with GNU addr,+N ranges and in-place editing.",
	Usage:    "sed [-nErs] [-e SCRIPT]... [-f FILE]... [-i[SUFFIX]] [SCRIPT] [FILE...]",
}

func init() { cmd.Run = run; tool.Register(cmd) }

func run(rc *tool.RunContext, args []string) int {
	// A failed sed is where a BSD idiom shows up (`sed -i ''` leaves the
	// real script read as a filename). Hint on the ERROR PATH only.
	code := runCommandWithCType(rc, args, openCType)
	if code != 0 {
		nudge.OnFailure(rc.Err, append([]string{cmd.Name}, args...), rc.Env)
	}
	return code
}

func runCommand(rc *tool.RunContext, args []string) int {
	return runCommandWithCType(rc, args, openCType)
}

type ctypeProvider interface {
	bre.ByteCtype
	Close() error
}

type ctypeOpener func(string) (ctypeProvider, error)

func openCType(name string) (ctypeProvider, error) { return ctype.Open(name) }

func runCommandWithCType(rc *tool.RunContext, args []string, opener ctypeOpener) int {
	// GNU's -i takes an optional attached suffix (`-i.bak`), which pflag
	// cannot model without displaying an internal sentinel in --help.
	// Extract every supported spelling before ordinary flag parsing.
	helpRequested := hasHelpFlag(args)
	args, inPlaceFlag, inPlaceSuffix := extractInPlace(args)

	fs := tool.NewFlags(cmd.Name)
	// POSIX utility syntax stops option recognition at the first operand. In
	// particular, a later file named -n or -- is an operand, not an option.
	fs.SetInterspersed(false)
	quiet := fs.BoolP("quiet", "n", false, "suppress automatic printing of pattern space")
	fs.BoolVar(quiet, "silent", false, "same as -n")
	var scripts []string
	fs.StringArrayVarP(&scripts, "expression", "e", nil, "add SCRIPT to the commands to be executed")
	var scriptFiles []string
	fs.StringArrayVarP(&scriptFiles, "file", "f", nil, "add the contents of FILE to the commands")
	ereE := fs.BoolP("regexp-extended", "E", false, "use extended regular expressions")
	ereR := fs.BoolP("regexp-extended-r", "r", false, "same as -E")
	separate := fs.BoolP("separate", "s", false, "consider files as separate rather than one continuous stream")
	// This flag remains registered for the generated help text. extractInPlace
	// removes it before parsing during normal execution.
	inPlace := fs.StringP("in-place", "i", "", "edit files in place (optional backup SUFFIX)")
	if helpRequested {
		fs.Lookup("in-place").NoOptDefVal = "SUFFIX"
	}

	operands, code := tool.Parse(rc, cmd, fs, args)
	if code >= 0 {
		return code
	}

	// Assemble the program: -e / -f in command-line order; else the first
	// operand is the script. pflag retains order within each StringArray but not
	// between the two flags, so recover the mixed order from the option prefix.
	var program string
	switch {
	case len(scripts) > 0 || len(scriptFiles) > 0:
		var parts []string
		sources := orderedScriptSources(args)
		if len(sources) != len(scripts)+len(scriptFiles) {
			// Keep uncommon pflag spellings (notably abbreviated long options)
			// working even if this order-only scanner does not recognize them.
			sources = nil
			for _, script := range scripts {
				sources = append(sources, scriptSource{value: script})
			}
			for _, file := range scriptFiles {
				sources = append(sources, scriptSource{value: file, file: true})
			}
		}
		for _, source := range sources {
			if !source.file {
				parts = append(parts, source.value)
				continue
			}
			b, err := os.ReadFile(rc.Path(source.value))
			if err != nil {
				fmt.Fprintf(rc.Err, "sed: %s: %v\n", source.value, err)
				return 2
			}
			parts = append(parts, string(b))
		}
		program = strings.Join(parts, "\n")
	case len(operands) > 0:
		program = operands[0]
		operands = operands[1:]
	default:
		return tool.UsageError(rc, cmd, "no script specified")
	}

	files := operands
	var suffix string

	if inPlaceFlag {
		if len(files) == 0 {
			return tool.UsageError(rc, cmd, "-i may not be used with stdin")
		}
		suffix = inPlaceSuffix
		if fs.Lookup("in-place").Changed {
			suffix = *inPlace
		}
	}

	opts := gosed.Options{ExtendedRegex: *ereE || *ereR}
	lcCType := locale.Resolve(rc.Env, locale.CType)
	if lcCType != "C" && lcCType != "POSIX" {
		provider, err := opener(lcCType)
		if err != nil {
			fmt.Fprintf(rc.Err, "sed: LC_CTYPE %q: %v\n", lcCType, err)
			return 2
		}
		tables, snapshotErr := bre.SnapshotLocaleByteTables(provider)
		closeErr := provider.Close()
		if snapshotErr != nil {
			fmt.Fprintf(rc.Err, "sed: LC_CTYPE %q: %v\n", lcCType, snapshotErr)
			return 2
		}
		if closeErr != nil {
			fmt.Fprintf(rc.Err, "sed: LC_CTYPE %q: %v\n", lcCType, closeErr)
			return 2
		}
		opts.LocaleTables = tables
	}

	if err := validateProgram(program, *quiet, opts); err != nil {
		fmt.Fprintf(rc.Err, "sed: %v\n", err)
		return 2
	}

	// In-place editing requires real files; rewrite each independently.
	if inPlaceFlag {
		rc2 := 0
		for _, f := range files {
			if err := editInPlace(rc, program, *quiet, opts, f, suffix); err != nil {
				fmt.Fprintf(rc.Err, "sed: %s: %v\n", f, err)
				rc2 = 2
			}
		}
		return rc2
	}

	// Stream mode: stdin, or files concatenated (one stream) / separate (-s).
	if len(files) == 0 {
		if err := apply(rc, program, *quiet, opts, rc.In, rc.Out); err != nil {
			fmt.Fprintf(rc.Err, "sed: %v\n", err)
			return 2
		}
		return 0
	}

	status := 0
	if *separate {
		for _, f := range files {
			r, err := openInput(rc, f)
			if err != nil {
				fmt.Fprintf(rc.Err, "sed: %s: %v\n", f, err)
				status = 2
				continue
			}
			err = apply(rc, program, *quiet, opts, r, rc.Out)
			closeIf(r)
			if err != nil {
				fmt.Fprintf(rc.Err, "sed: %v\n", err)
				status = 2
			}
		}
		return status
	}

	// One continuous stream across all files.
	var readers []io.Reader
	var closers []io.Closer
	for _, f := range files {
		r, err := openInput(rc, f)
		if err != nil {
			fmt.Fprintf(rc.Err, "sed: %s: %v\n", f, err)
			status = 2
			continue
		}
		readers = append(readers, r)
		if c, ok := r.(io.Closer); ok && f != "-" {
			closers = append(closers, c)
		}
	}
	if err := apply(rc, program, *quiet, opts, io.MultiReader(readers...), rc.Out); err != nil {
		fmt.Fprintf(rc.Err, "sed: %v\n", err)
		status = 2
	}
	for _, c := range closers {
		c.Close()
	}
	return status
}

func validateProgram(program string, quiet bool, opts gosed.Options) error {
	if _, _, err := parseSimpleSubstitution(program, opts); err != nil {
		return err
	}
	readFile := func(string) ([]byte, error) { return nil, nil }
	prepareWrite := func(string) error { return nil }
	writeFile := func(string, string) error { return nil }
	if quiet {
		_, err := gosed.NewQuietWithReadWriteFileOptions(strings.NewReader(program), readFile, prepareWrite, writeFile, opts)
		return err
	}
	_, err := gosed.NewWithReadWriteFileOptions(strings.NewReader(program), readFile, prepareWrite, writeFile, opts)
	return err
}

type scriptSource struct {
	value string
	file  bool
}

// orderedScriptSources extracts -e/--expression and -f/--file arguments from
// the option prefix. Flag validation remains pflag's job; this pass exists
// solely to retain ordering across the two repeatable option kinds.
func orderedScriptSources(args []string) []scriptSource {
	var sources []scriptSource
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--" || arg == "-" || !strings.HasPrefix(arg, "-") {
			break
		}
		switch {
		case arg == "-e" || arg == "-f" || arg == "--expression" || arg == "--file":
			if i+1 >= len(args) {
				continue
			}
			i++
			sources = append(sources, scriptSource{value: args[i], file: arg == "-f" || arg == "--file"})
		case strings.HasPrefix(arg, "--expression="):
			sources = append(sources, scriptSource{value: strings.TrimPrefix(arg, "--expression=")})
		case strings.HasPrefix(arg, "--file="):
			sources = append(sources, scriptSource{value: strings.TrimPrefix(arg, "--file="), file: true})
		case strings.HasPrefix(arg, "-") && !strings.HasPrefix(arg, "--"):
			cluster := arg[1:]
			for pos := 0; pos < len(cluster); pos++ {
				if cluster[pos] != 'e' && cluster[pos] != 'f' {
					continue
				}
				value := cluster[pos+1:]
				if value == "" && i+1 < len(args) {
					i++
					value = args[i]
				}
				sources = append(sources, scriptSource{value: value, file: cluster[pos] == 'f'})
				break // a value-taking short flag consumes the cluster remainder
			}
		}
	}
	return sources
}

func hasHelpFlag(args []string) bool {
	for _, arg := range args {
		if arg == "--" {
			return false
		}
		if arg == "--help" || arg == "-h" {
			return true
		}
	}
	return false
}

// extractInPlace recognizes GNU's optional-argument spellings. A short
// optional argument must be attached: "-i .bak" means no backup suffix and
// leaves ".bak" as an operand, while "-i.bak" supplies the suffix.
func extractInPlace(args []string) (filtered []string, changed bool, suffix string) {
	filtered = make([]string, 0, len(args))
	for i, arg := range args {
		if arg == "--" {
			filtered = append(filtered, args[i:]...)
			break
		}
		switch {
		case strings.HasPrefix(arg, "--"):
			name, value, hasValue := strings.Cut(strings.TrimPrefix(arg, "--"), "=")
			if name != "" && strings.HasPrefix("in-place", name) {
				changed, suffix = true, ""
				if hasValue {
					suffix = value
				}
				continue
			}
			filtered = append(filtered, arg)
			continue
		case arg == "-i":
			changed, suffix = true, ""
			continue
		case len(arg) > 2 && arg[0] == '-' && arg[1] != '-':
			cluster := arg[1:]
			pos := strings.IndexByte(cluster, 'i')
			if pos < 0 || strings.ContainsAny(cluster[:pos], "ef") {
				filtered = append(filtered, arg)
				continue
			}
			changed, suffix = true, cluster[pos+1:]
			if pos > 0 {
				filtered = append(filtered, "-"+cluster[:pos])
			}
			continue
		default:
			filtered = append(filtered, arg)
		}
	}
	return filtered, changed, suffix
}

// apply compiles the program and streams input→output through the engine.
func apply(rc *tool.RunContext, program string, quiet bool, opts gosed.Options, in io.Reader, out io.Writer) error {
	if !quiet {
		if subst, ok, err := parseSimpleSubstitution(program, opts); ok || err != nil {
			if err != nil {
				return err
			}
			return applySimpleSubstitution(subst, in, out)
		}
	}

	eng, err := newEngine(rc, program, quiet, opts)
	if err != nil {
		return err
	}
	_, err = io.Copy(out, eng.Wrap(in))
	return err
}

type simpleSubstitution struct {
	pattern     simplePattern
	replacement []byte
	global      bool
}

type simplePattern interface {
	FindAllSubmatchIndex([]byte, int) ([][]int, error)
	Expand([]byte, []byte, []byte, []int) ([]byte, error)
}

func parseSimpleSubstitution(program string, opts gosed.Options) (*simpleSubstitution, bool, error) {
	i := skipProgramSpace(program, 0)
	if i >= len(program) || program[i] != 's' {
		return nil, false, nil
	}
	i++
	if i >= len(program) {
		return nil, false, nil
	}
	delimiter, size := utf8.DecodeRuneInString(program[i:])
	if delimiter == utf8.RuneError && size == 1 {
		delimiter = rune(program[i])
	}
	i += size
	if delimiter == '\n' {
		return nil, false, nil
	}

	pattern, next, ok := readFastDelimited(program, i, delimiter, false)
	if !ok {
		return nil, false, nil
	}
	// An empty pattern is the null RE — "the last RE used". This fast path
	// exists only for a program that IS one s/// command, so there is no
	// earlier RE to stand in for it; hand it to the full engine, whose
	// null-RE handling reports that as the error it is.
	if pattern == "" {
		return nil, false, nil
	}
	replacement, next, ok := readFastDelimited(program, next, delimiter, true)
	if !ok {
		return nil, false, nil
	}

	modsStart := next
	for next < len(program) {
		r, sz := utf8.DecodeRuneInString(program[next:])
		if r == ';' || unicode.IsSpace(r) {
			break
		}
		next += sz
	}
	mods := program[modsStart:next]
	if mods != "" && mods != "g" {
		return nil, false, nil
	}
	if skipProgramSpace(program, next) != len(program) {
		return nil, false, nil
	}

	rx, repl, err := gosed.CompileSimpleSubstitution(pattern, replacement, opts)
	if err != nil {
		return nil, true, err
	}
	return &simpleSubstitution{pattern: rx, replacement: repl, global: mods == "g"}, true, nil
}

func skipProgramSpace(s string, i int) int {
	for i < len(s) {
		r, sz := utf8.DecodeRuneInString(s[i:])
		if !unicode.IsSpace(r) {
			break
		}
		i += sz
	}
	return i
}

func readFastDelimited(s string, i int, delimiter rune, replacement bool) (string, int, bool) {
	var b strings.Builder
	var previous rune
	for i < len(s) {
		start := i
		r, sz := utf8.DecodeRuneInString(s[i:])
		i += sz
		raw := s[start:i]
		if r == utf8.RuneError && sz == 1 {
			r = rune(raw[0])
		}
		if r == '\n' {
			return "", 0, false
		}
		if r == delimiter && (replacement || previous != '\\') {
			return b.String(), i, true
		}
		if replacement {
			if r == '\r' {
				continue
			}
			if r == '\\' {
				if i >= len(s) {
					return "", 0, false
				}
				nextStart := i
				next, nextSize := utf8.DecodeRuneInString(s[i:])
				i += nextSize
				nextRaw := s[nextStart:i]
				if next == utf8.RuneError && nextSize == 1 {
					next = rune(nextRaw[0])
				}
				if next == delimiter {
					b.WriteString(nextRaw)
				} else {
					b.WriteByte('\\')
					b.WriteString(nextRaw)
				}
				previous = next
				continue
			}
		}
		b.WriteString(raw)
		previous = r
	}
	return "", 0, false
}

func applySimpleSubstitution(subst *simpleSubstitution, in io.Reader, out io.Writer) error {
	src, err := io.ReadAll(in)
	if err != nil {
		return err
	}

	w := bufio.NewWriter(out)
	dst := make([]byte, 0, 4096)
	for len(src) > 0 {
		line := src
		hasNewline := false
		if i := bytes.IndexByte(src, '\n'); i >= 0 {
			line = src[:i]
			src = src[i+1:]
			hasNewline = true
		} else {
			src = nil
		}

		dst, err = applySimpleSubstitutionLine(dst[:0], subst, line)
		if err != nil {
			return err
		}
		if _, err := w.Write(dst); err != nil {
			return err
		}
		if hasNewline {
			if err := w.WriteByte('\n'); err != nil {
				return err
			}
		}
	}
	return w.Flush()
}

func applySimpleSubstitutionLine(dst []byte, subst *simpleSubstitution, line []byte) ([]byte, error) {
	limit := 1
	if subst.global {
		limit = -1
	}
	matches, err := subst.pattern.FindAllSubmatchIndex(line, limit)
	if err != nil {
		return dst, err
	}
	if len(matches) == 0 {
		return append(dst, line...), nil
	}

	// Build separately from dst so an expansion failure cannot mutate even the
	// caller's backing array before the error is returned.
	replaced := make([]byte, 0, len(line))
	end := 0
	for _, match := range matches {
		replaced = append(replaced, line[end:match[0]]...)
		replaced, err = subst.pattern.Expand(replaced, subst.replacement, line, match)
		if err != nil {
			return dst, err
		}
		end = match[1]
	}
	replaced = append(replaced, line[end:]...)
	return append(dst, replaced...), nil
}

func editInPlace(rc *tool.RunContext, program string, quiet bool, opts gosed.Options, file, suffix string) error {
	path := rc.Path(file)
	src, err := os.Open(path)
	if err != nil {
		return err
	}
	defer src.Close()
	fi, err := src.Stat()
	if err != nil {
		return err
	}
	eng, err := newEngine(rc, program, quiet, opts)
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".sed-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	keepTemp := false
	defer func() {
		tmp.Close()
		if !keepTemp {
			os.Remove(tmpName)
		}
	}()
	if err := tmp.Chmod(fi.Mode().Perm()); err != nil {
		return err
	}
	if _, err := io.Copy(tmp, eng.Wrap(src)); err != nil {
		return err
	}
	if err := src.Close(); err != nil {
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}

	if suffix != "" {
		backup := path + suffix
		if strings.Contains(suffix, "*") {
			backup = strings.ReplaceAll(suffix, "*", path)
		}
		if filepath.Clean(backup) == filepath.Clean(path) {
			if err := replaceExisting(tmpName, path); err != nil {
				return err
			}
			keepTemp = true
			return nil
		}
		// GNU replaces an existing backup of the same name.
		if err := os.Remove(backup); err != nil && !os.IsNotExist(err) {
			return err
		}
		if err := os.Rename(path, backup); err != nil {
			return err
		}
		if err := os.Rename(tmpName, path); err != nil {
			_ = os.Rename(backup, path)
			return err
		}
		keepTemp = true
		return nil
	}

	if err := replaceExisting(tmpName, path); err != nil {
		return err
	}
	keepTemp = true
	return nil
}

func replaceExisting(src, dst string) error {
	if err := os.Rename(src, dst); err == nil {
		return nil
	}
	// Windows cannot rename over an existing file. Displace the destination
	// first, but retain it until the replacement succeeds so a failed rename
	// never destroys the original.
	placeholder, err := os.CreateTemp(filepath.Dir(dst), ".sed-old-*")
	if err != nil {
		return err
	}
	old := placeholder.Name()
	if err := placeholder.Close(); err != nil {
		return err
	}
	if err := os.Remove(old); err != nil {
		return err
	}
	if err := os.Rename(dst, old); err != nil {
		return err
	}
	if err := os.Rename(src, dst); err != nil {
		_ = os.Rename(old, dst)
		return err
	}
	return os.Remove(old)
}

func newEngine(rc *tool.RunContext, program string, quiet bool, opts gosed.Options) (*gosed.Engine, error) {
	readFile := func(name string) ([]byte, error) {
		return os.ReadFile(rc.Path(name))
	}
	prepareWriteFile := func(name string) error {
		f, err := os.OpenFile(rc.Path(name), os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o666)
		if err != nil {
			return err
		}
		return f.Close()
	}
	writeFile := func(name, pattern string) error {
		f, err := os.OpenFile(rc.Path(name), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o666)
		if err != nil {
			return err
		}
		defer f.Close()
		if _, err := f.WriteString(pattern); err != nil {
			return err
		}
		_, err = f.WriteString("\n")
		return err
	}
	if quiet {
		return gosed.NewQuietWithReadWriteFileOptions(strings.NewReader(program), readFile, prepareWriteFile, writeFile, opts)
	}
	return gosed.NewWithReadWriteFileOptions(strings.NewReader(program), readFile, prepareWriteFile, writeFile, opts)
}

func openInput(rc *tool.RunContext, f string) (io.Reader, error) {
	if f == "-" {
		return rc.In, nil
	}
	return os.Open(rc.Path(f))
}

func closeIf(r io.Reader) {
	if c, ok := r.(io.Closer); ok {
		c.Close()
	}
}
