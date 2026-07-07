// Portions adapted from https://github.com/guonaihong/coreutils
// hashcore/hashcore.go (Apache-2.0) and https://github.com/u-root/u-root
// cmds/core/md5sum, cmds/core/shasum (BSD-3-Clause).
// Changes: rewired to the tool framework (RunContext stdio/cwd, strict
// flag layer), check-mode parsing/warnings/exit codes reworked to match
// the GNU coreutils manual (tagged + untagged + single-space line
// formats, blank/comment line skipping, escaped filenames, exact
// WARNING wording), BSD --tag output and filename escaping added.

// Package hashenc is the shared, non-registered engine behind the
// checksum tools (md5sum, sha1sum, sha224sum, sha256sum, sha384sum,
// sha512sum) and the base-encoding tools (base64, base32). Each
// cmds/<name> package registers a thin tool built by NewSumTool /
// NewBaseTool.
package hashenc

import (
	"bufio"
	"encoding/hex"
	"errors"
	"fmt"
	"hash"
	"io"
	"io/fs"
	"os"
	"strings"

	"github.com/qiangli/coreutils/tool"
)

// SumSpec describes one GNU checksum tool. The engine is parameterized
// by hash constructor + name: write the semantics once, instantiate
// six times.
type SumSpec struct {
	Name      string           // command name, e.g. "md5sum"
	Algo      string           // --tag label and tagged-line matcher, e.g. "MD5"
	Bits      int              // digest size in bits (hex length = Bits/4)
	New       func() hash.Hash // digest constructor
	NewLength func(bits int) (hash.Hash, error)
}

// NewSumTool builds the registered Tool for one checksum command.
func NewSumTool(spec SumSpec) *tool.Tool {
	t := &tool.Tool{
		Name:     spec.Name,
		Synopsis: fmt.Sprintf("Print or check %s (%d-bit) checksums. With no FILE, or when FILE is -, read standard input.", spec.Algo, spec.Bits),
		Usage:    fmt.Sprintf("%s [OPTION]... [FILE]...", spec.Name),
	}
	t.Run = func(rc *tool.RunContext, args []string) int {
		return runSum(rc, t, spec, args)
	}
	return t
}

func runSum(rc *tool.RunContext, t *tool.Tool, spec SumSpec, args []string) int {
	fs := tool.NewFlags(t.Name)
	binary := fs.BoolP("binary", "b", false, "read in binary mode")
	check := fs.BoolP("check", "c", false, "read checksums from the FILEs and check them")
	tag := fs.Bool("tag", false, "create a BSD-style checksum")
	text := fs.BoolP("text", "t", false, "read in text mode")
	zero := fs.BoolP("zero", "z", false, "end each output line with NUL, not newline, and disable file name escaping")
	warn := fs.BoolP("warn", "w", false, "warn about improperly formatted checksum lines")
	status := fs.Bool("status", false, "don't output anything, status code shows success")
	quiet := fs.Bool("quiet", false, "don't print OK for each successfully verified file")
	strict := fs.Bool("strict", false, "exit non-zero for improperly formatted checksum lines")
	ignoreMissing := fs.Bool("ignore-missing", false, "don't fail or report status for missing files")
	length := fs.IntP("length", "l", spec.Bits, "digest length in bits")
	operands, code := tool.Parse(rc, t, fs, args)
	if code >= 0 {
		return code
	}
	_ = text
	if *tag && *check {
		return tool.UsageError(rc, t, "the --tag option is meaningless when verifying checksums")
	}
	if *zero && *check {
		return tool.UsageError(rc, t, "the --zero option is not supported when verifying checksums")
	}
	if *length != spec.Bits && spec.NewLength == nil {
		return tool.UsageError(rc, t, "--length is not supported for %s", t.Name)
	}
	if *length <= 0 || *length%8 != 0 || *length > spec.Bits {
		return tool.UsageError(rc, t, "invalid digest length: %d", *length)
	}
	runSpec := spec
	runSpec.Bits = *length
	if len(operands) == 0 {
		operands = []string{"-"}
	}

	if *check {
		opts := checkOptions{
			warn:          *warn,
			status:        *status,
			quiet:         *quiet,
			strict:        *strict,
			ignoreMissing: *ignoreMissing,
		}
		exit := 0
		for _, op := range operands {
			if checkSumsFile(rc, t, runSpec, op, opts) != 0 {
				exit = 1
			}
		}
		return exit
	}

	// Compute mode. Note on -b/--binary: the digest is computed over
	// the raw bytes on every platform (this implementation never does
	// text-mode translation), so -b only switches the output separator
	// from "  " to " *" — content is identical with and without it,
	// exactly as on GNU/Linux.
	exit := 0
	for _, op := range operands {
		sum, err := digestOf(rc, runSpec, op)
		if err != nil {
			fmt.Fprintf(rc.Err, "%s: %s: %s\n", t.Name, op, gnuErrMsg(err))
			exit = 1
			continue
		}
		name, prefix := op, ""
		if !*zero {
			name, prefix = escapeFilename(op)
		}
		lineEnd := "\n"
		if *zero {
			lineEnd = "\x00"
		}
		if *tag {
			fmt.Fprintf(rc.Out, "%s%s (%s) = %s%s", prefix, runSpec.Algo, name, sum, lineEnd)
		} else {
			sep := "  "
			if *binary {
				sep = " *"
			}
			fmt.Fprintf(rc.Out, "%s%s%s%s%s", prefix, sum, sep, name, lineEnd)
		}
	}
	return exit
}

// errIsDirectory carries the GNU diagnostic for hashing a directory.
var errIsDirectory = errors.New("Is a directory")

// digestOf hashes one operand ("-" = the invocation's stdin) and
// returns the lowercase hex digest.
func digestOf(rc *tool.RunContext, spec SumSpec, name string) (string, error) {
	var h hash.Hash
	if spec.NewLength != nil {
		var err error
		h, err = spec.NewLength(spec.Bits)
		if err != nil {
			return "", err
		}
	} else {
		h = spec.New()
	}
	if name == "-" {
		if rc.In != nil {
			if _, err := io.Copy(h, rc.In); err != nil {
				return "", err
			}
		}
	} else {
		f, err := os.Open(rc.Path(name))
		if err != nil {
			return "", err
		}
		defer f.Close()
		if fi, err := f.Stat(); err == nil && fi.IsDir() {
			return "", errIsDirectory
		}
		if _, err := io.Copy(h, f); err != nil {
			return "", err
		}
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// checkSumsFile verifies one checksum-list operand (GNU -c semantics).
// Returns 0 when every listed file verified, 1 otherwise.
type checkOptions struct {
	warn          bool
	status        bool
	quiet         bool
	strict        bool
	ignoreMissing bool
}

func checkSumsFile(rc *tool.RunContext, t *tool.Tool, spec SumSpec, op string, opts checkOptions) int {
	var r io.Reader
	isStdin := op == "-"
	if isStdin {
		r = rc.In
		if r == nil {
			r = strings.NewReader("")
		}
	} else {
		f, err := os.Open(rc.Path(op))
		if err != nil {
			fmt.Fprintf(rc.Err, "%s: %s: %s\n", t.Name, op, gnuErrMsg(err))
			return 1
		}
		defer f.Close()
		r = f
	}

	br := bufio.NewReader(r)
	var valid, badFormat, mismatched, unreadable int
	exit := 0
	for {
		line, rerr := br.ReadString('\n')
		l := strings.TrimSuffix(line, "\n")
		// Blank lines and '#' comment lines are skipped without
		// counting as improperly formatted (GNU behavior).
		if l != "" && !strings.HasPrefix(l, "#") {
			digest, path, display, ok := parseCheckLine(spec, l)
			// A "-" entry while the list itself is read from stdin
			// cannot be verified (GNU counts it as improperly
			// formatted).
			if !ok || (isStdin && path == "-") {
				badFormat++
			} else {
				valid++
				sum, err := digestOf(rc, spec, path)
				switch {
				case err != nil:
					if !(opts.ignoreMissing && errors.Is(err, fs.ErrNotExist)) {
						if !opts.status {
							fmt.Fprintf(rc.Err, "%s: %s: %s\n", t.Name, display, gnuErrMsg(err))
							fmt.Fprintf(rc.Out, "%s: FAILED open or read\n", display)
						}
						unreadable++
						exit = 1
					}
				case strings.EqualFold(sum, digest):
					if !opts.status && !opts.quiet {
						fmt.Fprintf(rc.Out, "%s: OK\n", display)
					}
				default:
					if !opts.status {
						fmt.Fprintf(rc.Out, "%s: FAILED\n", display)
					}
					mismatched++
					exit = 1
				}
			}
		}
		if rerr != nil {
			break
		}
	}

	if valid == 0 {
		if !opts.status {
			fmt.Fprintf(rc.Err, "%s: %s: no properly formatted checksum lines found\n", t.Name, op)
		}
		return 1
	}
	if badFormat > 0 && !opts.status {
		fmt.Fprintf(rc.Err, "%s: WARNING: %d %s\n", t.Name, badFormat,
			plural(badFormat, "line is improperly formatted", "lines are improperly formatted"))
	}
	if badFormat > 0 && opts.strict {
		exit = 1
	}
	if mismatched > 0 && !opts.status {
		fmt.Fprintf(rc.Err, "%s: WARNING: %d %s\n", t.Name, mismatched,
			plural(mismatched, "computed checksum did NOT match", "computed checksums did NOT match"))
	}
	if unreadable > 0 && !opts.status {
		fmt.Fprintf(rc.Err, "%s: WARNING: %d %s\n", t.Name, unreadable,
			plural(unreadable, "listed file could not be read", "listed files could not be read"))
	}
	return exit
}

// parseCheckLine parses one checksum line in any of the three formats
// GNU accepts: BSD tagged ("ALGO (name) = hex"), untagged
// ("hex  name" / "hex *name"), and single-space ("hex name"). A
// leading backslash marks an escaped filename (\n, \r, \\). path is
// the unescaped name to hash; display is the name as written in the
// file (what the OK/FAILED line shows).
func parseCheckLine(spec SumSpec, line string) (digest, path, display string, ok bool) {
	escaped := false
	l := line
	if strings.HasPrefix(l, "\\") {
		escaped = true
		l = l[1:]
	}
	hexLen := spec.Bits / 4

	// BSD tagged format; the algorithm label must match this tool.
	if rest, found := strings.CutPrefix(l, spec.Algo+" ("); found {
		if i := strings.LastIndex(rest, ") = "); i >= 0 {
			name, d := rest[:i], rest[i+4:]
			if name != "" && isHexN(d, hexLen) {
				return d, unescapeIf(escaped, name), name, true
			}
		}
		// Fall through: a malformed tagged line can still never be a
		// valid untagged line (it starts with the algo name, not hex).
	}

	i := strings.IndexByte(l, ' ')
	if i < 0 {
		return "", "", "", false
	}
	d := l[:i]
	if !isHexN(d, hexLen) {
		return "", "", "", false
	}
	rest := l[i+1:]
	var name string
	switch {
	case strings.HasPrefix(rest, " "), strings.HasPrefix(rest, "*"):
		name = rest[1:] // "hex  name" or "hex *name"
	default:
		name = rest // "hex name" (single separator space)
	}
	if name == "" {
		return "", "", "", false
	}
	return d, unescapeIf(escaped, name), name, true
}

func isHexN(s string, n int) bool {
	if len(s) != n {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		if !(c >= '0' && c <= '9' || c >= 'a' && c <= 'f' || c >= 'A' && c <= 'F') {
			return false
		}
	}
	return true
}

func plural(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}

// escapeFilename implements GNU digest-output escaping: filenames
// containing backslash, newline, or carriage return are escaped and
// the whole line gets a leading "\".
func escapeFilename(name string) (escaped, prefix string) {
	e := strings.NewReplacer("\\", "\\\\", "\n", "\\n", "\r", "\\r").Replace(name)
	if e != name {
		return e, "\\"
	}
	return name, ""
}

// unescapeIf reverses escapeFilename for check-mode lines that carry
// the leading "\" marker. Unknown escapes pass through unchanged
// (matching GNU's lenient unescaper).
func unescapeIf(escaped bool, name string) string {
	if !escaped || !strings.Contains(name, "\\") {
		return name
	}
	var b strings.Builder
	b.Grow(len(name))
	for i := 0; i < len(name); i++ {
		if name[i] == '\\' && i+1 < len(name) {
			switch name[i+1] {
			case '\\':
				b.WriteByte('\\')
				i++
				continue
			case 'n':
				b.WriteByte('\n')
				i++
				continue
			case 'r':
				b.WriteByte('\r')
				i++
				continue
			}
		}
		b.WriteByte(name[i])
	}
	return b.String()
}

// gnuErrMsg maps a Go file error onto the C-locale strerror text GNU
// tools print, so output is identical on every platform.
func gnuErrMsg(err error) string {
	switch {
	case errors.Is(err, errIsDirectory):
		return "Is a directory"
	case errors.Is(err, fs.ErrNotExist):
		return "No such file or directory"
	case errors.Is(err, fs.ErrPermission):
		return "Permission denied"
	}
	var pe *fs.PathError
	if errors.As(err, &pe) {
		return pe.Err.Error()
	}
	return err.Error()
}
