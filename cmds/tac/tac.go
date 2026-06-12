// Package taccmd implements tac(1) per the GNU coreutils manual:
// concatenate and print files in reverse (record order).
//
// Fresh implementation against the GNU manual (prior art consulted:
// guonaihong/coreutils tac; its seek-backwards machinery was replaced
// by a simple read-all + boundary scan, which is exact for the
// supported default + -s modes).
package taccmd

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
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
		tacWrite(w, data, []byte(*sep))
	}
	if err := w.Flush(); err != nil {
		fmt.Fprintf(rc.Err, "tac: write error: %v\n", err)
		return 1
	}
	return exit
}

// tacWrite emits the records of data in reverse order. A record is a
// chunk of input ending with (and including) the separator; a final
// chunk without a trailing separator is also a record and is emitted
// first, without a separator — matching GNU's default "attach the
// separator after the record" behavior.
func tacWrite(w io.Writer, data, sep []byte) {
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
		w.Write(data[recs[k].s:recs[k].e])
	}
}

func sysErr(err error) error {
	var pe *fs.PathError
	if errors.As(err, &pe) {
		return pe.Err
	}
	return err
}
