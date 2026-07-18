// Package awkcmd implements awk(1) by embedding GoAWK.
//
// Backed by github.com/benhoyt/goawk (MIT).
package awkcmd

import (
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"

	"github.com/benhoyt/goawk/interp"
	"github.com/benhoyt/goawk/lexer"
	"github.com/benhoyt/goawk/parser"
	"github.com/qiangli/coreutils/tool"
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
		src, ok := readProgramFiles(rc, progFiles)
		if !ok {
			return 2
		}
		source = src
		files = operands
	} else {
		if len(operands) == 0 {
			return tool.UsageError(rc, cmd, "missing program")
		}
		source = operands[0]
		files = operands[1:]
	}

	vars := []string{}
	if *fieldSep != "" {
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

	prog, err := parser.ParseProgram([]byte(source), nil)
	if err != nil {
		fmt.Fprintf(rc.Err, "awk: %v\n", err)
		return 2
	}

	status, err := interp.ExecProgram(prog, &interp.Config{
		Stdin:  readerOrEmpty(rc.In),
		Output: rc.Out,
		// Deterministic LF output on every platform (GoAWK's default
		// SmartNewlineMode emits CRLF on Windows, violating the LC_ALL=C
		// no-platform-variance contract).
		NewlineOutput: interp.RawNewlineMode,
		Error:         rc.Err,
		Argv0:         cmd.Name,
		Args:          resolveFiles(rc, files),
		Vars:          vars,
		Environ:       environPairs(rc.Env),
	})
	if err != nil {
		fmt.Fprintf(rc.Err, "awk: %v\n", err)
		return 2
	}
	return status
}

func readProgramFiles(rc *tool.RunContext, names []string) (string, bool) {
	var b strings.Builder
	for i, name := range names {
		var data []byte
		var err error
		if name == "-" {
			data, err = io.ReadAll(readerOrEmpty(rc.In))
		} else {
			data, err = os.ReadFile(rc.Path(name))
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

func resolveFiles(rc *tool.RunContext, files []string) []string {
	out := make([]string, len(files))
	for i, name := range files {
		switch {
		case name == "" || name == "-" || assignRegex.MatchString(name):
			// Not filenames: "" is skipped, "-" is stdin, and var=value
			// operands are assignments GoAWK executes in ARGV order.
			out[i] = name
		default:
			out[i] = rc.Path(name)
		}
	}
	return out
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
