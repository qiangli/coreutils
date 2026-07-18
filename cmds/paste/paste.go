// Package pastecmd implements paste(1) per the GNU coreutils manual:
// write lines consisting of the sequentially corresponding lines from
// each FILE, separated by TABs, to standard output.
//
// Semantics verified against the GNU manual (delimiter cycling resets
// per output line in parallel mode and per file in serial mode; a
// delimiter position is consumed for every file, including exhausted
// ones, with the trailing delimiter of each line removed; "\0" in the
// -d LIST means "no delimiter").
package pastecmd

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"unicode/utf8"

	"github.com/qiangli/coreutils/tool"
)

var cmd = &tool.Tool{
	Name:     "paste",
	Synopsis: "Write lines consisting of the sequentially corresponding lines from each FILE, separated by TABs, to standard output.",
	Usage:    "paste [OPTION]... [FILE]...",
}

// Run is wired in init: a literal would create an initialization
// cycle (run's flag-error paths reference cmd).
func init() { cmd.Run = run; tool.Register(cmd) }

func run(rc *tool.RunContext, args []string) int {
	fs := tool.NewFlags(cmd.Name)
	delims := fs.StringP("delimiters", "d", "\t", "reuse characters from LIST instead of TABs")
	serial := fs.BoolP("serial", "s", false, "paste one file at a time instead of in parallel")
	zero := fs.BoolP("zero-terminated", "z", false, "line delimiter is NUL, not newline")
	operands, code := tool.Parse(rc, cmd, fs, tool.AliasHelpVersion(args))
	if code >= 0 {
		return code
	}

	dl, errMsg := parseDelims(*delims)
	if errMsg != "" {
		fmt.Fprintf(rc.Err, "paste: %s\n", errMsg)
		return 1
	}
	if len(operands) == 0 {
		operands = []string{"-"}
	}

	// Every "-" operand shares one buffered reader over the invocation's
	// stdin, so multiple "-" columns interleave lines (GNU behavior).
	var stdinR *bufio.Reader
	open := func(name string) (*bufio.Reader, io.Closer, error) {
		if name == "-" {
			if stdinR == nil {
				stdinR = bufio.NewReader(rc.In)
			}
			return stdinR, nil, nil
		}
		f, err := os.Open(rc.Path(name))
		if err != nil {
			return nil, nil, err
		}
		return bufio.NewReader(f), f, nil
	}

	out := bufio.NewWriter(rc.Out)
	dc := &delimCycle{list: dl}
	var status int
	lineEnd := byte('\n')
	if *zero {
		lineEnd = 0
	}
	if *serial {
		status = pasteSerial(rc, operands, open, dc, out, lineEnd)
	} else {
		status = pasteParallel(rc, operands, open, dc, out, lineEnd)
	}
	if err := out.Flush(); err != nil {
		fmt.Fprintf(rc.Err, "paste: write error: %v\n", err)
		status = 1
	}
	return status
}

// parseDelims expands the -d LIST escapes the GNU manual defines:
// \n \t \\ \b \f \r \v, and \0 meaning "no delimiter at this position".
// Any other backslash-escaped character stands for itself.
func parseDelims(list string) ([][]byte, string) {
	if list == "" {
		return nil, "no delimiters specified"
	}

	var out [][]byte
	for i := 0; i < len(list); i++ {
		c := list[i]
		if c != '\\' {
			_, size := utf8.DecodeRuneInString(list[i:])
			out = append(out, []byte(list[i:i+size]))
			i += size - 1
			continue
		}
		i++
		if i >= len(list) {
			return nil, fmt.Sprintf("delimiter list ends with an unescaped backslash: %s", list)
		}
		switch list[i] {
		case '0':
			out = append(out, nil) // empty delimiter
		case 'b':
			out = append(out, []byte{'\b'})
		case 'f':
			out = append(out, []byte{'\f'})
		case 'n':
			out = append(out, []byte{'\n'})
		case 'r':
			out = append(out, []byte{'\r'})
		case 't':
			out = append(out, []byte{'\t'})
		case 'v':
			out = append(out, []byte{'\v'})
		case '\\':
			out = append(out, []byte{'\\'})
		default:
			_, size := utf8.DecodeRuneInString(list[i:])
			out = append(out, []byte(list[i:i+size]))
			i += size - 1
		}
	}
	return out, ""
}

// delimCycle hands out delimiters one after the other, restarting at
// the head of the LIST when it runs out — and remembers the length of
// the last delimiter written so a trailing one can be removed.
type delimCycle struct {
	list [][]byte
	idx  int
	last int
}

func (d *delimCycle) write(buf *[]byte) {
	if len(d.list) == 0 {
		d.last = 0
		return
	}
	cur := d.list[d.idx]
	d.idx = (d.idx + 1) % len(d.list)
	*buf = append(*buf, cur...)
	d.last = len(cur)
}

func (d *delimCycle) trimTrailing(buf *[]byte) {
	*buf = (*buf)[:len(*buf)-d.last]
	d.last = 0
}

func (d *delimCycle) reset() {
	d.idx = 0
	d.last = 0
}

type opener func(name string) (*bufio.Reader, io.Closer, error)

func pasteParallel(rc *tool.RunContext, names []string, open opener, dc *delimCycle, out *bufio.Writer, lineEnd byte) int {
	type input struct {
		r *bufio.Reader
		c io.Closer
	}
	ins := make([]input, 0, len(names))
	closeAll := func() {
		for _, in := range ins {
			if in.c != nil {
				in.c.Close()
			}
		}
	}
	// GNU opens every file before producing output; one failure aborts.
	for _, name := range names {
		r, c, err := open(name)
		if err != nil {
			fmt.Fprintf(rc.Err, "paste: %s: %v\n", name, pathErr(err))
			closeAll()
			return 1
		}
		ins = append(ins, input{r, c})
	}
	defer closeAll()

	eof := make([]bool, len(ins))
	var buf []byte
	for {
		buf = buf[:0]
		dc.reset()
		eofCount := 0
		for i, in := range ins {
			if eof[i] {
				eofCount++
			} else {
				chunk, err := in.r.ReadBytes(lineEnd)
				if len(chunk) == 0 {
					eof[i] = true
					eofCount++
					if err != nil && !errors.Is(err, io.EOF) {
						fmt.Fprintf(rc.Err, "paste: %s: %v\n", names[i], err)
						return 1
					}
				} else {
					if chunk[len(chunk)-1] == lineEnd {
						chunk = chunk[:len(chunk)-1]
					}
					buf = append(buf, chunk...)
				}
			}
			dc.write(&buf)
		}
		if eofCount == len(ins) {
			return 0
		}
		dc.trimTrailing(&buf)
		buf = append(buf, lineEnd)
		out.Write(buf)
	}
}

func pasteSerial(rc *tool.RunContext, names []string, open opener, dc *delimCycle, out *bufio.Writer, lineEnd byte) int {
	status := 0
	var buf []byte
	for _, name := range names {
		r, closer, err := open(name)
		if err != nil {
			fmt.Fprintf(rc.Err, "paste: %s: %v\n", name, pathErr(err))
			status = 1
			continue
		}
		buf = buf[:0]
		dc.reset()
		for {
			chunk, rerr := r.ReadBytes(lineEnd)
			if len(chunk) == 0 {
				if rerr != nil && !errors.Is(rerr, io.EOF) {
					fmt.Fprintf(rc.Err, "paste: %s: %v\n", name, rerr)
					status = 1
				}
				break
			}
			if chunk[len(chunk)-1] == lineEnd {
				chunk = chunk[:len(chunk)-1]
			}
			buf = append(buf, chunk...)
			dc.write(&buf)
		}
		dc.trimTrailing(&buf)
		buf = append(buf, lineEnd)
		out.Write(buf)
		if closer != nil {
			closer.Close()
		}
	}
	return status
}

// pathErr unwraps *fs.PathError so diagnostics read "paste: f: no such
// file or directory" instead of repeating the operation and path.
func pathErr(err error) string {
	return tool.SysErrString(err)
}
