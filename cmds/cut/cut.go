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
	"sort"

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
	pairs               []rangePair
	complement          bool
	fieldMode           bool
	delim               byte
	onlyDelimited       bool
	scratch             []byte
	buf                 []byte
	outDelim            []byte
	lineTerm            byte
	whitespaceDelimited bool
	hasOutDelim         bool
}

func run(rc *tool.RunContext, args []string) int {
	fs := tool.NewFlags(cmd.Name)
	bytesList := fs.StringP("bytes", "b", "", "select only these bytes")
	charsList := fs.StringP("characters", "c", "", "select only these characters")
	delim := fs.StringP("delimiter", "d", "", "use DELIM instead of TAB for field delimiter")
	fieldsList := fs.StringP("fields", "f", "", "select only these fields; also print any line that contains no delimiter character, unless the -s option is specified")
	complement := fs.Bool("complement", false, "complement the set of selected bytes, characters or fields")
	onlyDelimited := fs.BoolP("only-delimited", "s", false, "do not print lines not containing delimiters")
	outputDelimiter := fs.StringP("output-delimiter", "O", "", "use STRING as the output delimiter")
	zeroTerminated := fs.BoolP("zero-terminated", "z", false, "line delimiter is NUL, not newline")
	fs.BoolP("ignored-n", "n", false, "do not split multi-byte characters (ignored)")
	whitespaceDelimited := fs.BoolP("whitespace-delimited", "w", false, "use any consecutive spaces and/or tabs as the field delimiter")

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

	if *whitespaceDelimited && fs.Changed("delimiter") {
		return tool.UsageError(rc, cmd, "only one delimiter may be specified")
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
	if *whitespaceDelimited && !fieldMode {
		return tool.UsageError(rc, cmd, "whitespace-delimited mode makes sense only when operating on fields")
	}

	pairs, errMsg := parseList(list, fieldMode)
	if errMsg != "" {
		return tool.UsageError(rc, cmd, "%s", errMsg)
	}

	pairs = mergeRanges(pairs)
	if *complement {
		pairs = complementRanges(pairs)
	}

	lineTerm := byte('\n')
	if *zeroTerminated {
		lineTerm = 0
	}
	var outDelim []byte
	if fs.Changed("output-delimiter") {
		outDelim = []byte(*outputDelimiter)
	} else if *whitespaceDelimited {
		outDelim = []byte(" ")
	} else {
		outDelim = []byte{delimByte}
	}

	c := &cutter{
		pairs:               pairs,
		complement:          *complement,
		fieldMode:           fieldMode,
		delim:               delimByte,
		onlyDelimited:       *onlyDelimited,
		scratch:             make([]byte, 0, 1024),
		buf:                 make([]byte, 4*1024),
		outDelim:            outDelim,
		lineTerm:            lineTerm,
		whitespaceDelimited: *whitespaceDelimited,
		hasOutDelim:         fs.Changed("output-delimiter"),
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
		if err := c.process(r, out); err != nil {
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

func mergeRanges(pairs []rangePair) []rangePair {
	if len(pairs) == 0 {
		return nil
	}
	sort.Slice(pairs, func(i, j int) bool {
		if pairs[i].lo == pairs[j].lo {
			return pairs[i].hi < pairs[j].hi
		}
		return pairs[i].lo < pairs[j].lo
	})

	merged := make([]rangePair, 0, len(pairs))
	current := pairs[0]
	for i := 1; i < len(pairs); i++ {
		next := pairs[i]
		if next.lo <= current.hi+1 {
			if next.hi > current.hi {
				current.hi = next.hi
			}
		} else {
			merged = append(merged, current)
			current = next
		}
	}
	merged = append(merged, current)
	return merged
}

func complementRanges(pairs []rangePair) []rangePair {
	var comp []rangePair
	lastHi := 0
	for _, p := range pairs {
		if p.lo > lastHi+1 {
			comp = append(comp, rangePair{lastHi + 1, p.lo - 1})
		}
		lastHi = p.hi
		if lastHi == math.MaxInt {
			break
		}
	}
	if lastHi < math.MaxInt {
		comp = append(comp, rangePair{lastHi + 1, math.MaxInt})
	}
	return comp
}

func (c *cutter) process(r io.Reader, out *bufio.Writer) error {
	buf := c.buf
	head := 0
	tail := 0

	for {
		if tail == len(buf) {
			if head > 0 {
				copy(buf, buf[head:tail])
				tail -= head
				head = 0
			} else {
				newBuf := make([]byte, len(buf)*2)
				copy(newBuf, buf)
				buf = newBuf
				c.buf = buf
			}
		}

		n, err := r.Read(buf[tail:])
		tail += n

		for {
			idx := bytes.IndexByte(buf[head:tail], c.lineTerm)
			if idx < 0 {
				break
			}
			lineEnd := head + idx
			line := buf[head:lineEnd]
			c.emitLine(line, true, out)
			head = lineEnd + 1
		}

		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return err
		}
	}

	if head < tail {
		c.emitLine(buf[head:tail], false, out)
	}
	return nil
}

func (c *cutter) emitLine(line []byte, hadNL bool, out *bufio.Writer) {
	if c.fieldMode {
		if c.whitespaceDelimited {
			hasDelim := bytes.IndexByte(line, ' ') >= 0 || bytes.IndexByte(line, '\t') >= 0
			if !hasDelim {
				if !c.onlyDelimited {
					c.scratch = c.scratch[:0]
					c.scratch = append(c.scratch, line...)
					if hadNL {
						c.scratch = append(c.scratch, c.lineTerm)
					}
					if len(c.scratch) > 0 {
						out.Write(c.scratch)
					}
				}
				return
			}
			fields := splitWhitespace(line)
			first := true
			c.scratch = c.scratch[:0]
			for _, p := range c.pairs {
				lo := p.lo
				hi := p.hi
				if lo > len(fields) {
					break
				}
				if hi > len(fields) {
					hi = len(fields)
				}
				for fIdx := lo; fIdx <= hi; fIdx++ {
					if !first {
						c.scratch = append(c.scratch, c.outDelim...)
					}
					c.scratch = append(c.scratch, fields[fIdx-1]...)
					first = false
				}
			}
			if hadNL {
				c.scratch = append(c.scratch, c.lineTerm)
			}
			if len(c.scratch) > 0 {
				out.Write(c.scratch)
			}
			return
		}

		if bytes.IndexByte(line, c.delim) < 0 {
			if !c.onlyDelimited {
				c.scratch = c.scratch[:0]
				c.scratch = append(c.scratch, line...)
				if hadNL {
					c.scratch = append(c.scratch, c.lineTerm)
				}
				if len(c.scratch) > 0 {
					out.Write(c.scratch)
				}
			}
			return
		}
		first := true
		field := 1
		start := 0
		pairIdx := 0

		c.scratch = c.scratch[:0]

		for {
			for pairIdx < len(c.pairs) && field > c.pairs[pairIdx].hi {
				pairIdx++
			}
			if pairIdx >= len(c.pairs) {
				break
			}

			if field < c.pairs[pairIdx].lo {
				for field < c.pairs[pairIdx].lo {
					rel := bytes.IndexByte(line[start:], c.delim)
					if rel < 0 {
						goto done
					}
					start = start + rel + 1
					field++
				}
			}

			rel := bytes.IndexByte(line[start:], c.delim)
			end := len(line)
			if rel >= 0 {
				end = start + rel
			}

			if !first {
				c.scratch = append(c.scratch, c.outDelim...)
			}
			c.scratch = append(c.scratch, line[start:end]...)
			first = false

			if rel < 0 {
				break
			}
			field++
			start = end + 1
		}
	done:
		if hadNL {
			c.scratch = append(c.scratch, c.lineTerm)
		}
		if len(c.scratch) > 0 {
			out.Write(c.scratch)
		}
		return
	}

	c.scratch = c.scratch[:0]
	printDelim := false
	for _, p := range c.pairs {
		if p.lo > len(line) {
			break
		}
		end := p.hi
		if end > len(line) {
			end = len(line)
		}
		if printDelim {
			c.scratch = append(c.scratch, c.outDelim...)
		} else if c.hasOutDelim {
			printDelim = true
		}
		c.scratch = append(c.scratch, line[p.lo-1:end]...)
	}
	if hadNL {
		c.scratch = append(c.scratch, c.lineTerm)
	}
	if len(c.scratch) > 0 {
		out.Write(c.scratch)
	}
}

func splitWhitespace(line []byte) [][]byte {
	var fields [][]byte
	inField := false
	start := 0
	for i, b := range line {
		if b == ' ' || b == '\t' {
			if inField {
				fields = append(fields, line[start:i])
				inField = false
			}
		} else {
			if !inField {
				start = i
				inField = true
			}
		}
	}
	if inField {
		fields = append(fields, line[start:])
	}
	return fields
}

// pathErr unwraps *fs.PathError so diagnostics read "cut: f: no such
// file or directory" instead of repeating the operation and path.
func pathErr(err error) string {
	return tool.SysErrString(err)
}
