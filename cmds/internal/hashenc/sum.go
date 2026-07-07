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
	"strconv"
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
	var length *int
	if spec.NewLength != nil {
		// Only blake2b takes -l/--length (GNU: md5sum --length is an
		// unknown option).
		length = fs.IntP("length", "l", 0, "digest length in bits; must not exceed the max size and must be a multiple of 8")
	}
	operands, code := tool.Parse(rc, t, fs, args)
	if code >= 0 {
		return code
	}
	if *tag && *check {
		return tool.UsageError(rc, t, "the --tag option is meaningless when verifying checksums")
	}
	if *zero && *check {
		return tool.UsageError(rc, t, "the --zero option is not supported when verifying checksums")
	}
	if *tag && *text {
		return tool.UsageError(rc, t, "--tag does not support --text mode")
	}
	if !*check {
		// GNU rejects the check-only options outside --check.
		switch {
		case *ignoreMissing:
			return tool.UsageError(rc, t, "the --ignore-missing option is meaningful only when verifying checksums")
		case *status:
			return tool.UsageError(rc, t, "the --status option is meaningful only when verifying checksums")
		case *warn:
			return tool.UsageError(rc, t, "the --warn option is meaningful only when verifying checksums")
		case *quiet:
			return tool.UsageError(rc, t, "the --quiet option is meaningful only when verifying checksums")
		case *strict:
			return tool.UsageError(rc, t, "the --strict option is meaningful only when verifying checksums")
		}
	}
	runSpec := spec
	if length != nil {
		if *length < 0 || *length%8 != 0 || *length > spec.Bits {
			return tool.UsageError(rc, t, "invalid digest length: %d", *length)
		}
		if *length != 0 {
			// -l 0 keeps the default digest size (GNU).
			runSpec.Bits = *length
		}
	}
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
			// Verification ignores -l: the digest length is taken from
			// each line (GNU auto-detection), so pass the pristine spec.
			if checkSumsFile(rc, t, spec, op, opts) != 0 {
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
			label := runSpec.Algo
			if spec.NewLength != nil && runSpec.Bits < spec.Bits {
				// GNU b2sum prints the digest length in the tag when
				// it is not the default: "BLAKE2b-256 (f) = …".
				label = fmt.Sprintf("%s-%d", runSpec.Algo, runSpec.Bits)
			}
			fmt.Fprintf(rc.Out, "%s%s (%s) = %s%s", prefix, label, name, sum, lineEnd)
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
	display := op
	if isStdin {
		// GNU prints the check-list name through quotef, which shell-
		// quotes the translated "standard input" string.
		display = "'standard input'"
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
	lineNo := 0
	exit := 0
	for {
		line, rerr := br.ReadString('\n')
		lineNo++
		l := strings.TrimSuffix(line, "\n")
		// Blank lines and '#' comment lines are skipped without
		// counting as improperly formatted (GNU behavior).
		if l != "" && !strings.HasPrefix(l, "#") {
			digest, path, name, bits, ok := parseCheckLine(spec, l)
			// A "-" entry while the list itself is read from stdin
			// cannot be verified (GNU counts it as improperly
			// formatted).
			if !ok || (isStdin && path == "-") {
				badFormat++
				if opts.warn {
					fmt.Fprintf(rc.Err, "%s: %s: %d: improperly formatted %s checksum line\n",
						t.Name, display, lineNo, spec.Algo)
				}
			} else {
				valid++
				lineSpec := spec
				lineSpec.Bits = bits
				sum, err := digestOf(rc, lineSpec, path)
				switch {
				case err != nil:
					if !(opts.ignoreMissing && errors.Is(err, fs.ErrNotExist)) {
						if !opts.status {
							fmt.Fprintf(rc.Err, "%s: %s: %s\n", t.Name, name, gnuErrMsg(err))
							fmt.Fprintf(rc.Out, "%s: FAILED open or read\n", name)
						}
						unreadable++
						exit = 1
					}
				case strings.EqualFold(sum, digest):
					if !opts.status && !opts.quiet {
						fmt.Fprintf(rc.Out, "%s: OK\n", name)
					}
				default:
					if !opts.status {
						fmt.Fprintf(rc.Out, "%s: FAILED\n", name)
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
			fmt.Fprintf(rc.Err, "%s: %s: no properly formatted checksum lines found\n", t.Name, display)
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
// GNU accepts: BSD tagged ("ALGO (name) = hex", with an "ALGO-BITS"
// tag for variable-length digests), untagged ("hex  name" /
// "hex *name"), and single-space ("hex name"). A leading backslash
// marks an escaped filename (\n, \r, \\). path is the unescaped name
// to hash; display is the name as written in the file (what the
// OK/FAILED line shows). bits is the digest length the line carries:
// for fixed-size tools it is always spec.Bits; for variable-length
// tools (b2sum) it comes from the tag suffix on tagged lines and from
// the digest's hex-digit count on untagged lines (GNU auto-detection,
// applied even when -l was given).
func parseCheckLine(spec SumSpec, line string) (digest, path, display string, bits int, ok bool) {
	escaped := false
	l := line
	if strings.HasPrefix(l, "\\") {
		escaped = true
		l = l[1:]
	}
	variable := spec.NewLength != nil

	// BSD tagged format; the algorithm label must match this tool.
	if rest, found := strings.CutPrefix(l, spec.Algo); found {
		tagBits := spec.Bits
		tagOK := true
		if spec.NewLength != nil {
			// b2sum accepts "BLAKE2b-BITS (name) = hex".
			if suffix, hasLen := strings.CutPrefix(rest, "-"); hasLen {
				j := 0
				for j < len(suffix) && suffix[j] >= '0' && suffix[j] <= '9' {
					j++
				}
				n, err := strconv.Atoi(suffix[:j])
				if j == 0 || err != nil || n <= 0 || n%8 != 0 || n > spec.Bits {
					tagOK = false
				} else {
					tagBits = n
					rest = suffix[j:]
				}
			}
		}
		if tagOK {
			if body, found := strings.CutPrefix(rest, " ("); found {
				if i := strings.LastIndex(body, ") = "); i >= 0 {
					name, d := body[:i], body[i+4:]
					if name != "" && isHexN(d, tagBits/4) {
						return d, unescapeIf(escaped, name), name, tagBits, true
					}
				}
			}
		}
		// Fall through: a malformed tagged line can still never be a
		// valid untagged line (it starts with the algo name, not hex).
	}

	i := strings.IndexByte(l, ' ')
	if i < 0 {
		return "", "", "", 0, false
	}
	d := l[:i]
	dBits := spec.Bits
	if variable {
		// Auto-detect the digest length from the hex-digit count
		// (GNU b2sum -c accepts any valid BLAKE2b length).
		if len(d) < 2 || len(d)%2 != 0 || len(d) > spec.Bits/4 {
			return "", "", "", 0, false
		}
		dBits = len(d) * 4
	}
	if !isHexN(d, dBits/4) {
		return "", "", "", 0, false
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
		return "", "", "", 0, false
	}
	return d, unescapeIf(escaped, name), name, dBits, true
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

// EscapeFilename exposes GNU digest-output filename escaping for the
// sibling checksum tools (cksum).
func EscapeFilename(name string) (escaped, prefix string) {
	return escapeFilename(name)
}

// UnescapeFilename exposes the check-mode unescaper for the sibling
// checksum tools (cksum).
func UnescapeFilename(escaped bool, name string) string {
	return unescapeIf(escaped, name)
}

// GNUErrMsg exposes the C-locale strerror mapping for the sibling
// checksum tools (cksum).
func GNUErrMsg(err error) string {
	return gnuErrMsg(err)
}

// ErrIsDirectory is the shared sentinel for hashing a directory.
var ErrIsDirectory = errIsDirectory

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
