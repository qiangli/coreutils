// Package csplitcmd implements a practical csplit(1) subset: split an
// input file at line numbers or regular-expression matches.
package csplitcmd

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"os"
	"regexp"
	"strconv"
	"strings"

	"github.com/qiangli/coreutils/tool"
)

var cmd = &tool.Tool{
	Name:     "csplit",
	Synopsis: "Split a file into sections determined by context lines.",
	Usage:    "csplit [OPTION]... FILE PATTERN...",
}

func init() { cmd.Run = run; tool.Register(cmd) }

type splitPoint struct {
	line int
	skip bool
}

func run(rc *tool.RunContext, args []string) int {
	fs := tool.NewFlags(cmd.Name)
	prefix := fs.StringP("prefix", "f", "xx", "use PREFIX instead of 'xx'")
	digits := fs.IntP("digits", "n", 2, "use DIGITS digits in output file names")
	silent := fs.BoolP("silent", "s", false, "do not print output file sizes")
	keep := fs.BoolP("keep-files", "k", false, "do not remove output files on errors")
	operands, code := tool.Parse(rc, cmd, fs, args)
	if code >= 0 {
		return code
	}
	if len(operands) < 2 {
		return tool.UsageError(rc, cmd, "missing operand")
	}
	if *digits < 1 {
		return tool.UsageError(rc, cmd, "invalid number of digits: '%d'", *digits)
	}

	lines, err := readLines(rc, operands[0])
	if err != nil {
		fmt.Fprintf(rc.Err, "csplit: cannot open '%s' for reading: %v\n", operands[0], tool.SysErr(err))
		return 1
	}
	points, code := resolvePatterns(rc, lines, operands[1:])
	if code >= 0 {
		return code
	}
	created, err := writePieces(rc, lines, points, *prefix, *digits, *silent)
	if err != nil {
		if !*keep {
			for _, name := range created {
				_ = os.Remove(rc.Path(name))
			}
		}
		fmt.Fprintf(rc.Err, "csplit: %v\n", err)
		return 1
	}
	return 0
}

func readLines(rc *tool.RunContext, name string) ([]string, error) {
	var r io.Reader
	if name == "-" {
		r = rc.In
		if r == nil {
			r = strings.NewReader("")
		}
	} else {
		f, err := os.Open(rc.Path(name))
		if err != nil {
			return nil, err
		}
		defer f.Close()
		r = f
	}
	br := bufio.NewReader(r)
	var lines []string
	for {
		line, err := br.ReadString('\n')
		if line != "" {
			lines = append(lines, line)
		}
		if err == io.EOF {
			return lines, nil
		}
		if err != nil {
			return nil, err
		}
	}
}

func resolvePatterns(rc *tool.RunContext, lines []string, patterns []string) ([]splitPoint, int) {
	points := make([]splitPoint, 0, len(patterns))
	start := 0
	for _, pattern := range patterns {
		if pattern == "{*}" {
			return nil, tool.NotSupported(rc, cmd, "repeat pattern {*} in this subset")
		}
		if n, err := strconv.Atoi(pattern); err == nil {
			if n < 1 || n > len(lines)+1 {
				return nil, tool.UsageError(rc, cmd, "line number out of range: '%s'", pattern)
			}
			idx := n - 1
			points = append(points, splitPoint{line: idx})
			start = idx
			continue
		}
		skip := strings.HasPrefix(pattern, "%") && strings.HasSuffix(pattern, "%")
		if !(skip || strings.HasPrefix(pattern, "/") && strings.HasSuffix(pattern, "/")) {
			return nil, tool.NotSupported(rc, cmd, fmt.Sprintf("pattern form '%s' (supported: line numbers, /REGEXP/, %%REGEXP%%)", pattern))
		}
		expr := pattern[1 : len(pattern)-1]
		re, err := regexp.Compile(expr)
		if err != nil {
			return nil, tool.UsageError(rc, cmd, "invalid regular expression '%s'", expr)
		}
		found := -1
		for i := start; i < len(lines); i++ {
			if re.MatchString(strings.TrimSuffix(lines[i], "\n")) {
				found = i
				break
			}
		}
		if found < 0 {
			return nil, tool.UsageError(rc, cmd, "match not found: '%s'", pattern)
		}
		points = append(points, splitPoint{line: found, skip: skip})
		start = found + 1
	}
	return points, -1
}

func writePieces(rc *tool.RunContext, lines []string, points []splitPoint, prefix string, digits int, silent bool) ([]string, error) {
	var created []string
	start := 0
	seq := 0
	for _, point := range points {
		if point.line < start {
			return created, fmt.Errorf("split point moved backwards")
		}
		if !point.skip {
			name, err := writePiece(rc, lines[start:point.line], prefix, digits, seq, silent)
			if err != nil {
				return created, err
			}
			created = append(created, name)
			seq++
		}
		start = point.line
	}
	name, err := writePiece(rc, lines[start:], prefix, digits, seq, silent)
	if err != nil {
		return created, err
	}
	created = append(created, name)
	return created, nil
}

func writePiece(rc *tool.RunContext, lines []string, prefix string, digits, seq int, silent bool) (string, error) {
	name := fmt.Sprintf("%s%0*d", prefix, digits, seq)
	var buf bytes.Buffer
	for _, line := range lines {
		buf.WriteString(line)
	}
	if err := os.WriteFile(rc.Path(name), buf.Bytes(), 0o666); err != nil {
		return name, err
	}
	if !silent {
		fmt.Fprintf(rc.Out, "%d\n", buf.Len())
	}
	return name, nil
}
