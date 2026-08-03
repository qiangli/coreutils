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
	code := runCommand(rc, args)
	if code != 0 {
		nudge.OnFailure(rc.Err, append([]string{cmd.Name}, args...), rc.Env)
	}
	return code
}

func runCommand(rc *tool.RunContext, args []string) int {
	// GNU's -i takes an optional attached suffix (`-i.bak`), which pflag
	// cannot model without displaying an internal sentinel in --help.
	// Extract every supported spelling before ordinary flag parsing.
	helpRequested := hasHelpFlag(args)
	args, inPlaceFlag, inPlaceSuffix := extractInPlace(args)

	fs := tool.NewFlags(cmd.Name)
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

	// Assemble the program: -e / -f in order; else the first operand is the script.
	var program string
	switch {
	case len(scripts) > 0 || len(scriptFiles) > 0:
		var parts []string
		parts = append(parts, scripts...)
		for _, f := range scriptFiles {
			b, err := os.ReadFile(rc.Path(f))
			if err != nil {
				fmt.Fprintf(rc.Err, "sed: %s: %v\n", f, err)
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

	gosed.ExtendedRegex = *ereE || *ereR

	files := operands

	// In-place editing requires real files; rewrite each independently.
	if inPlaceFlag {
		if len(files) == 0 {
			return tool.UsageError(rc, cmd, "-i may not be used with stdin")
		}
		suffix := inPlaceSuffix
		if fs.Lookup("in-place").Changed {
			suffix = *inPlace
		}
		rc2 := 0
		for _, f := range files {
			if err := editInPlace(rc, program, *quiet, f, suffix); err != nil {
				fmt.Fprintf(rc.Err, "sed: %s: %v\n", f, err)
				rc2 = 2
			}
		}
		return rc2
	}

	// Stream mode: stdin, or files concatenated (one stream) / separate (-s).
	if len(files) == 0 {
		if err := apply(rc, program, *quiet, rc.In, rc.Out); err != nil {
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
			err = apply(rc, program, *quiet, r, rc.Out)
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
	if err := apply(rc, program, *quiet, io.MultiReader(readers...), rc.Out); err != nil {
		fmt.Fprintf(rc.Err, "sed: %v\n", err)
		status = 2
	}
	for _, c := range closers {
		c.Close()
	}
	return status
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
func apply(rc *tool.RunContext, program string, quiet bool, in io.Reader, out io.Writer) error {
	if !quiet {
		if subst, ok, err := parseSimpleSubstitution(program); ok || err != nil {
			if err != nil {
				return err
			}
			return applySimpleSubstitution(subst, in, out)
		}
	}

	eng, err := newEngine(rc, program, quiet)
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
	FindAllSubmatchIndex([]byte, int) [][]int
	Expand([]byte, []byte, []byte, []int) []byte
}

func parseSimpleSubstitution(program string) (*simpleSubstitution, bool, error) {
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
		return nil, false, nil
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

	rx, repl, err := gosed.CompileSimpleSubstitution(pattern, replacement)
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
		r, sz := utf8.DecodeRuneInString(s[i:])
		i += sz
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
				next, nextSize := utf8.DecodeRuneInString(s[i:])
				i += nextSize
				if next == delimiter {
					b.WriteRune(delimiter)
				} else {
					b.WriteRune('\\')
					b.WriteRune(next)
				}
				previous = next
				continue
			}
		}
		b.WriteRune(r)
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

		dst = applySimpleSubstitutionLine(dst[:0], subst, line)
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

func applySimpleSubstitutionLine(dst []byte, subst *simpleSubstitution, line []byte) []byte {
	limit := 1
	if subst.global {
		limit = -1
	}
	matches := subst.pattern.FindAllSubmatchIndex(line, limit)
	if len(matches) == 0 {
		return append(dst, line...)
	}

	end := 0
	for _, match := range matches {
		dst = append(dst, line[end:match[0]]...)
		dst = subst.pattern.Expand(dst, subst.replacement, line, match)
		end = match[1]
	}
	return append(dst, line[end:]...)
}

func editInPlace(rc *tool.RunContext, program string, quiet bool, file, suffix string) error {
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
	eng, err := newEngine(rc, program, quiet)
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

func newEngine(rc *tool.RunContext, program string, quiet bool) (*gosed.Engine, error) {
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
		return gosed.NewQuietWithReadWriteFile(strings.NewReader(program), readFile, prepareWriteFile, writeFile)
	}
	return gosed.NewWithReadWriteFile(strings.NewReader(program), readFile, prepareWriteFile, writeFile)
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
