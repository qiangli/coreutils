// Package awkcmd implements awk(1) by embedding GoAWK.
//
// Backed by github.com/benhoyt/goawk (MIT).
package awkcmd

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/benhoyt/goawk/interp"
	"github.com/benhoyt/goawk/parser"
	"github.com/qiangli/coreutils/tool"
)

var cmd = &tool.Tool{
	Name:     "awk",
	Synopsis: "Pattern scanning and text processing language, backed by pure-Go GoAWK.",
	Usage:    "awk [-F fs] [-v var=val] [-f progfile | 'program'] [file ...]",
}

func init() { cmd.Run = run; tool.Register(cmd) }

func run(rc *tool.RunContext, args []string) int {
	fs := tool.NewFlags(cmd.Name)
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
		vars = append(vars, "FS", *fieldSep)
	}
	for _, assign := range assigns {
		name, value, ok := strings.Cut(assign, "=")
		if !ok || name == "" {
			return tool.UsageError(rc, cmd, "invalid -v assignment %q", assign)
		}
		vars = append(vars, name, value)
	}

	prog, err := parser.ParseProgram([]byte(source), nil)
	if err != nil {
		fmt.Fprintf(rc.Err, "awk: %v\n", err)
		return 2
	}

	status, err := interp.ExecProgram(prog, &interp.Config{
		Stdin:   readerOrEmpty(rc.In),
		Output:  rc.Out,
		Error:   rc.Err,
		Argv0:   cmd.Name,
		Args:    resolveFiles(rc, files),
		Vars:    vars,
		Environ: environPairs(rc.Env),
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
		data, err := os.ReadFile(rc.Path(name))
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
		if name == "-" {
			out[i] = name
		} else {
			out[i] = rc.Path(name)
		}
	}
	return out
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
