// Package taccmd implements tac(1) per the GNU coreutils manual:
// concatenate and print files in reverse (record order).
//
// Implemented flags: -b, -r, -s.
package taccmd

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"

	"github.com/qiangli/coreutils/tool"
)

var cmd = &tool.Tool{
	Name:     "tac",
	Synopsis: "Write each FILE to standard output, last line first.\nWith no FILE, or when FILE is -, read standard input.",
	Usage:    "tac [OPTION]... [FILE]...",
}

func init() { cmd.Run = run; tool.Register(cmd) }

func run(rc *tool.RunContext, args []string) int {
	fs := tool.NewFlags(cmd.Name)
	before := fs.BoolP("before", "b", false, "attach the separator before instead of after")
	regex := fs.BoolP("regex", "r", false, "interpret the separator as a regular expression")
	sep := fs.StringP("separator", "s", "\n", "use STRING as the separator instead of newline")
	operands, code := tool.Parse(rc, cmd, fs, args)
	if code >= 0 {
		return code
	}
	if *sep == "" {
		fmt.Fprintln(rc.Err, "tac: separator cannot be empty")
		return 1
	}

	files := operands
	if len(files) == 0 {
		files = []string{"-"}
	}

	var re *regexp.Regexp
	if *regex {
		var err error
		re, err = regexp.Compile(*sep)
		if err != nil {
			fmt.Fprintf(rc.Err, "tac: invalid regex separator '%s': %v\n", *sep, err)
			return 1
		}
	}

	w := bufio.NewWriter(rc.Out)
	exit := 0
	for _, name := range files {
		var r io.Reader
		var closer io.Closer
		if name == "-" {
			r = rc.In
			if r == nil {
				r = strings.NewReader("")
			}
		} else {
			f, err := os.Open(rc.Path(name))
			if err != nil {
				fmt.Fprintf(rc.Err, "tac: failed to open '%s' for reading: %v\n", name, sysErr(err))
				exit = 1
				continue
			}
			r = f
			closer = f
		}
		data, err := io.ReadAll(r)
		if closer != nil {
			closer.Close()
		}
		if err != nil {
			fmt.Fprintf(rc.Err, "tac: %s: %v\n", name, sysErr(err))
			exit = 1
			continue
		}
		if re != nil {
			tacWriteRegex(w, data, re, *before)
		} else {
			tacWrite(w, data, []byte(*sep), *before)
		}
	}
	if err := w.Flush(); err != nil {
		fmt.Fprintf(rc.Err, "tac: write error: %v\n", err)
		return 1
	}
	return exit
}

func tacWrite(w io.Writer, data, sep []byte, before bool) {
	if len(data) == 0 {
		return
	}
	type span struct{ s, e int }
	var recs []span
	i := 0
	for {
		j := bytes.Index(data[i:], sep)
		if j < 0 {
			break
		}
		end := i + j + len(sep)
		recs = append(recs, span{i, end})
		i = end
	}
	if i < len(data) {
		recs = append(recs, span{i, len(data)})
	}
	for k := len(recs) - 1; k >= 0; k-- {
		if before && k < len(recs)-1 {
			w.Write(sep)
		}
		s, e := recs[k].s, recs[k].e
		if before {
			e -= len(sep)
		}
		w.Write(data[s:e])
	}
}

func tacWriteRegex(w io.Writer, data []byte, re *regexp.Regexp, before bool) {
	if len(data) == 0 {
		return
	}
	matches := re.FindAllIndex(data, -1)
	type span struct{ s, e int }
	var recs []span
	last := 0
	for _, m := range matches {
		if before {
			recs = append(recs, span{last, m[0]})
			last = m[1]
		} else {
			recs = append(recs, span{last, m[1]})
			last = m[1]
		}
	}
	if last < len(data) {
		recs = append(recs, span{last, len(data)})
	}
	for k := len(recs) - 1; k >= 0; k-- {
		w.Write(data[recs[k].s:recs[k].e])
	}
}

func sysErr(err error) error {
	return tool.SysErr(err)
}
