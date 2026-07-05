// Package cutcmd implements cut(1) per the GNU coreutils manual:
// print selected parts of lines from each FILE to standard output.
//
// Portions adapted from https://github.com/guonaihong/coreutils cut/cut.go
// (Apache-2.0).
// Changes: rewired to the tool framework; replaced the list parser with a
// port of GNU set-fields semantics (exact diagnostics, decreasing-range and
// numbered-from-1 errors); --complement applied at selection time; -d/-s
// mode validation; multi-file and "-" stdin handling through RunContext.
package cutcmd

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"math"
	"os"

	"github.com/qiangli/coreutils/tool"
)

var cmd = &tool.Tool{
	Name:     "cut",
	Synopsis: "Print selected parts of lines from each FILE to standard output.",
	Usage:    "cut OPTION... [FILE]...",
}

// Run is wired in init: a literal would create an initialization
// cycle (run's flag-error paths reference cmd).
func init() { cmd.Run = run; tool.Register(cmd) }

type rangePair struct{ lo, hi int }

type cutter struct {
	pairs         []rangePair
	complement    bool
	fieldMode     bool
	delim         byte
	onlyDelimited bool
}

func run(rc *tool.RunContext, args []string) int {
	fs := tool.NewFlags(cmd.Name)
	bytesList := fs.StringP("bytes", "b", "", "select only these bytes")
	charsList := fs.StringP("characters", "c", "", "select only these characters")
	delim := fs.StringP("delimiter", "d", "", "use DELIM instead of TAB for field delimiter")
	fieldsList := fs.StringP("fields", "f", "", "select only these fields; also print any line that contains no delimiter character, unless the -s option is specified")
	complement := fs.Bool("complement", false, "complement the set of selected bytes, characters or fields")
	onlyDelimited := fs.BoolP("only-delimited", "s", false, "do not print lines not containing delimiters")
	operands, code := tool.Parse(rc, cmd, fs, args)
	if code >= 0 {
		return code
	}

	nmodes := 0
	var list string
	fieldMode := false
	if fs.Changed("bytes") {
		nmodes++
		list = *bytesList
	}
	if fs.Changed("characters") {
		nmodes++
		list = *charsList
	}
	if fs.Changed("fields") {
		nmodes++
		list = *fieldsList
		fieldMode = true
	}
	if nmodes == 0 {
		return tool.UsageError(rc, cmd, "you must specify a list of bytes, characters, or fields")
	}
	if nmodes > 1 {
		return tool.UsageError(rc, cmd, "only one type of list may be specified")
	}

	delimByte := byte('\t')
	if fs.Changed("delimiter") {
		if !fieldMode {
			return tool.UsageError(rc, cmd, "an input delimiter may be specified only when operating on fields")
		}
		if len(*delim) > 1 {
			return tool.UsageError(rc, cmd, "the delimiter must be a single character")
		}
		// GNU: -d '' means "use the NUL byte as the delimiter".
		if len(*delim) == 1 {
			delimByte = (*delim)[0]
		} else {
			delimByte = 0
		}
	}
	if *onlyDelimited && !fieldMode {
		return tool.UsageError(rc, cmd, "suppressing non-delimited lines makes sense\n\tonly when operating on fields")
	}

	pairs, errMsg := parseList(list, fieldMode)
	if errMsg != "" {
		return tool.UsageError(rc, cmd, "%s", errMsg)
	}

	c := &cutter{
		pairs:         pairs,
		complement:    *complement,
		fieldMode:     fieldMode,
		delim:         delimByte,
		onlyDelimited: *onlyDelimited,
	}

	if len(operands) == 0 {
		operands = []string{"-"}
	}
	status := 0
	out := bufio.NewWriter(rc.Out)
	for _, name := range operands {
		var r io.Reader
		var closer io.Closer
		if name == "-" {
			r = rc.In
		} else {
			f, err := os.Open(rc.Path(name))
			if err != nil {
				fmt.Fprintf(rc.Err, "cut: %s: %v\n", name, pathErr(err))
				status = 1
				continue
			}
			r = f
			closer = f
		}
		if err := c.process(bufio.NewReader(r), out); err != nil {
			fmt.Fprintf(rc.Err, "cut: %s: %v\n", name, pathErr(err))
			status = 1
		}
		if closer != nil {
			closer.Close()
		}
	}
	if err := out.Flush(); err != nil {
		fmt.Fprintf(rc.Err, "cut: write error: %v\n", err)
		status = 1
	}
	return status
}

// parseList is a port of the GNU set-fields.c state machine: a LIST is
// ranges (N, N-, N-M, -M) separated by commas or blanks, numbered from 1.
func parseList(list string, fieldMode bool) ([]rangePair, string) {
	const unlimited = math.MaxInt
	mode := func(fieldMsg, posMsg string) string {
		if fieldMode {
			return fieldMsg
		}
		return posMsg
	}
	var pairs []rangePair
	initial, value := 1, 0
	lhs, rhs, dash := false, false, false
	numStart := -1
	i := 0
	for {
		eos := i >= len(list)
		var ch byte
		if !eos {
			ch = list[i]
		}
		switch {
		case !eos && ch == '-':
			if dash {
				return nil, mode("invalid field range", "invalid byte or character range")
			}
			dash = true
			if lhs && value == 0 {
				return nil, mode("fields are numbered from 1", "byte/character positions are numbered from 1")
			}
			if lhs {
				initial = value
			} else {
				initial = 1
			}
			value = 0
			numStart = -1
			i++
		case eos || ch == ',' || ch == ' ' || ch == '\t':
			if dash {
				dash = false
				if !lhs && !rhs {
					return nil, "invalid range with no endpoint: -"
				}
				if !rhs {
					pairs = append(pairs, rangePair{initial, unlimited})
				} else {
					if value < initial {
						return nil, "invalid decreasing range"
					}
					pairs = append(pairs, rangePair{initial, value})
				}
			} else {
				if value == 0 {
					return nil, "fields and positions are numbered from 1"
				}
				pairs = append(pairs, rangePair{value, value})
			}
			value = 0
			numStart = -1
			if eos {
				return pairs, ""
			}
			i++
			lhs, rhs = false, false
		case ch >= '0' && ch <= '9':
			if numStart < 0 {
				numStart = i
			}
			if dash {
				rhs = true
			} else {
				lhs = true
			}
			if value > (math.MaxInt-9)/10 {
				j := numStart
				for j < len(list) && list[j] >= '0' && list[j] <= '9' {
					j++
				}
				num := list[numStart:j]
				return nil, mode(
					fmt.Sprintf("field number '%s' is too large", num),
					fmt.Sprintf("byte/character offset '%s' is too large", num))
			}
			value = value*10 + int(ch-'0')
			i++
		default:
			return nil, mode("invalid field range", "invalid byte or character range")
		}
	}
}

func (c *cutter) selected(idx int) bool {
	in := false
	for _, p := range c.pairs {
		if idx >= p.lo && idx <= p.hi {
			in = true
			break
		}
	}
	return in != c.complement
}

func (c *cutter) process(in *bufio.Reader, out *bufio.Writer) error {
	for {
		line, err := in.ReadBytes('\n')
		if len(line) > 0 {
			hadNL := line[len(line)-1] == '\n'
			if hadNL {
				line = line[:len(line)-1]
			}
			c.emitLine(line, hadNL, out)
		}
		if err != nil {
			if errors.Is(err, io.EOF) {
				return nil
			}
			return err
		}
	}
}

func (c *cutter) emitLine(line []byte, hadNL bool, out *bufio.Writer) {
	if c.fieldMode {
		if bytes.IndexByte(line, c.delim) < 0 {
			// GNU: a line with no delimiter is printed whole (even with
			// --complement), unless -s suppresses it.
			if !c.onlyDelimited {
				out.Write(line)
				if hadNL {
					out.WriteByte('\n')
				}
			}
			return
		}
		fields := bytes.Split(line, []byte{c.delim})
		first := true
		for i, f := range fields {
			if c.selected(i + 1) {
				if !first {
					out.WriteByte(c.delim)
				}
				out.Write(f)
				first = false
			}
		}
		if hadNL {
			out.WriteByte('\n')
		}
		return
	}
	// byte / character mode (GNU: -c is currently the same as -b)
	for i := range line {
		if c.selected(i + 1) {
			out.WriteByte(line[i])
		}
	}
	if hadNL {
		out.WriteByte('\n')
	}
}

// pathErr unwraps *fs.PathError so diagnostics read "cut: f: no such
// file or directory" instead of repeating the operation and path.
func pathErr(err error) string {
	return tool.SysErrString(err)
}
