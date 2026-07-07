// Package odcmd implements a practical od(1) subset for agents:
// octal-word default output plus common byte-oriented -t formats and
// offset/limit controls.
package odcmd

import (
	"bufio"
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"github.com/qiangli/coreutils/tool"
)

var cmd = &tool.Tool{
	Name:     "od",
	Synopsis: "Dump files in octal and other simple formats.",
	Usage:    "od [OPTION]... [FILE]...",
}

func init() { cmd.Run = run; tool.Register(cmd) }

type options struct {
	addrRadix string
	format    string
	limit     int64
	skip      int64
}

func run(rc *tool.RunContext, args []string) int {
	fs := tool.NewFlags(cmd.Name)
	addrRadix := fs.StringP("address-radix", "A", "o", "select offset radix: d, o, x, or n")
	format := fs.StringP("format", "t", "o2", "select output format: x1, o1, o2, c, d1")
	limitText := fs.StringP("read-bytes", "N", "", "limit dump to BYTES input bytes")
	skipText := fs.StringP("skip-bytes", "j", "0", "skip BYTES input bytes first")
	operands, code := tool.Parse(rc, cmd, fs, args)
	if code >= 0 {
		return code
	}

	limit := int64(-1)
	if *limitText != "" {
		n, err := parseBytes(*limitText)
		if err != nil || n < 0 {
			return tool.UsageError(rc, cmd, "invalid byte count: %q", *limitText)
		}
		limit = n
	}
	skip, err := parseBytes(*skipText)
	if err != nil || skip < 0 {
		return tool.UsageError(rc, cmd, "invalid skip count: %q", *skipText)
	}
	o := options{addrRadix: *addrRadix, format: *format, limit: limit, skip: skip}
	if o.addrRadix != "d" && o.addrRadix != "o" && o.addrRadix != "x" && o.addrRadix != "n" {
		return tool.UsageError(rc, cmd, "invalid address radix: %q", o.addrRadix)
	}
	if o.format != "x1" && o.format != "o1" && o.format != "o2" && o.format != "c" && o.format != "d1" {
		return tool.UsageError(rc, cmd, "unsupported output format: %q", o.format)
	}

	r, closers, exit := openInputs(rc, operands)
	defer func() {
		for _, c := range closers {
			c.Close()
		}
	}()
	if exit != 0 && r == nil {
		return exit
	}

	w := bufio.NewWriter(rc.Out)
	if err := dump(r, w, o); err != nil {
		fmt.Fprintf(rc.Err, "od: %v\n", tool.SysErr(err))
		exit = 1
	}
	if err := w.Flush(); err != nil {
		fmt.Fprintf(rc.Err, "od: write error: %v\n", err)
		return 1
	}
	return exit
}

func openInputs(rc *tool.RunContext, operands []string) (io.Reader, []io.Closer, int) {
	if len(operands) == 0 {
		if rc.In == nil {
			return strings.NewReader(""), nil, 0
		}
		return rc.In, nil, 0
	}
	var readers []io.Reader
	var closers []io.Closer
	exit := 0
	for _, name := range operands {
		if name == "-" {
			if rc.In == nil {
				readers = append(readers, strings.NewReader(""))
			} else {
				readers = append(readers, rc.In)
			}
			continue
		}
		f, err := os.Open(rc.Path(name))
		if err != nil {
			fmt.Fprintf(rc.Err, "od: %s: %v\n", name, tool.SysErr(err))
			exit = 1
			continue
		}
		readers = append(readers, f)
		closers = append(closers, f)
	}
	if len(readers) == 0 {
		return nil, closers, exit
	}
	return io.MultiReader(readers...), closers, exit
}

func dump(r io.Reader, w *bufio.Writer, o options) error {
	if o.skip > 0 {
		n, err := io.CopyN(io.Discard, r, o.skip)
		if err != nil && err != io.EOF {
			return err
		}
		if n < o.skip {
			if o.addrRadix != "n" {
				_, err = fmt.Fprintln(w, formatOffset(o.skip, o.addrRadix))
			}
			return err
		}
	}
	if o.limit >= 0 {
		r = io.LimitReader(r, o.limit)
	}
	block := make([]byte, 16)
	offset := o.skip
	for {
		n, err := io.ReadFull(r, block)
		if n > 0 {
			if werr := writeLine(w, offset, block[:n], o); werr != nil {
				return werr
			}
			offset += int64(n)
		}
		if err == io.EOF || err == io.ErrUnexpectedEOF {
			break
		}
		if err != nil {
			return err
		}
	}
	if o.addrRadix != "n" {
		_, err := fmt.Fprintln(w, formatOffset(offset, o.addrRadix))
		return err
	}
	return nil
}

func writeLine(w *bufio.Writer, offset int64, b []byte, o options) error {
	if o.addrRadix != "n" {
		if _, err := fmt.Fprintf(w, "%s", formatOffset(offset, o.addrRadix)); err != nil {
			return err
		}
	}
	switch o.format {
	case "x1":
		for _, c := range b {
			fmt.Fprintf(w, " %02x", c)
		}
	case "o1":
		for _, c := range b {
			fmt.Fprintf(w, " %03o", c)
		}
	case "d1":
		for _, c := range b {
			fmt.Fprintf(w, " %4d", int8(c))
		}
	case "c":
		for _, c := range b {
			fmt.Fprintf(w, " %3s", charName(c))
		}
	default:
		for i := 0; i < len(b); i += 2 {
			var word uint16
			if i+1 < len(b) {
				word = binary.LittleEndian.Uint16(b[i : i+2])
			} else {
				word = uint16(b[i])
			}
			fmt.Fprintf(w, " %06o", word)
		}
	}
	_, err := w.WriteString("\n")
	return err
}

func formatOffset(n int64, radix string) string {
	switch radix {
	case "d":
		return fmt.Sprintf("%07d", n)
	case "x":
		return fmt.Sprintf("%07x", n)
	default:
		return fmt.Sprintf("%07o", n)
	}
}

func charName(c byte) string {
	switch c {
	case 0:
		return "\\0"
	case '\n':
		return "\\n"
	case '\t':
		return "\\t"
	case '\r':
		return "\\r"
	case '\b':
		return "\\b"
	case '\f':
		return "\\f"
	case ' ':
		return "sp"
	}
	if c >= 32 && c <= 126 {
		return string(c)
	}
	return fmt.Sprintf("%03o", c)
}

var multipliers = map[string]int64{
	"":    1,
	"b":   512,
	"kB":  1000,
	"K":   1024,
	"KB":  1000,
	"M":   1024 * 1024,
	"MB":  1000 * 1000,
	"G":   1024 * 1024 * 1024,
	"GB":  1000 * 1000 * 1000,
	"KiB": 1024,
	"MiB": 1024 * 1024,
	"GiB": 1024 * 1024 * 1024,
}

func parseBytes(s string) (int64, error) {
	i := 0
	for i < len(s) && s[i] >= '0' && s[i] <= '9' {
		i++
	}
	if i == 0 {
		return 0, strconv.ErrSyntax
	}
	n, err := strconv.ParseInt(s[:i], 10, 64)
	if err != nil {
		return 0, err
	}
	m, ok := multipliers[s[i:]]
	if !ok {
		return 0, strconv.ErrSyntax
	}
	return n * m, nil
}
