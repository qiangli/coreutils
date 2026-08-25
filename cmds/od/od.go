// Package odcmd implements a practical od(1) subset for agents:
// octal-word default output plus common byte-oriented -t formats and
// offset/limit controls.
package odcmd

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"strconv"
	"strings"

	"github.com/qiangli/coreutils/tool"
)

var cmd = &tool.Tool{
	Name:     "od",
	Synopsis: "Dump files in octal and other simple formats. Type aliases: -D -F -H -I -L -O -X -e -f -i -l -s.",
	Usage:    "od [OPTION]... [FILE]...",
}

// GNU od's exact (lowercase) diagnostic for -j past the end of input.
var errSkipPastEOF = errors.New("cannot skip past end of combined input")

func init() { cmd.Run = run; tool.Register(cmd) }

type options struct {
	addrRadix string
	formats   []dumpFormat
	endian    binary.ByteOrder
	strings   int
	limit     int64
	skip      int64
	width     int
	showAll   bool
}

type dumpFormat struct {
	kind string
	size int
}

func run(rc *tool.RunContext, args []string) int {
	args = expandFormatArgs(args)
	fs := tool.NewFlags(cmd.Name)
	addrRadix := fs.StringP("address-radix", "A", "o", "select offset radix: d, o, x, or n")
	formats := fs.StringArrayP("format", "t", nil, "select output format; repeat for multiple formats")
	limitText := fs.StringP("read-bytes", "N", "", "limit dump to BYTES input bytes")
	skipText := fs.StringP("skip-bytes", "j", "0", "skip BYTES input bytes first")
	width := fs.IntP("width", "w", 16, "output BYTES bytes per line")
	endianText := fs.String("endian", "little", "select byte order for multi-byte formats: little or big")
	stringsText := fs.StringP("strings", "S", "", "output printable strings at least BYTES long")
	namedChars := fs.BoolP("named-chars", "a", false, "same as -t a")
	octalBytes := fs.BoolP("octal-bytes", "b", false, "same as -t o1")
	chars := fs.BoolP("characters", "c", false, "same as -t c")
	unsignedDecimal := fs.BoolP("unsigned-decimal-2", "d", false, "same as -t u2")
	octalWords := fs.BoolP("octal-2", "o", false, "same as -t o2")
	hexWords := fs.BoolP("hex-2", "x", false, "same as -t x2")
	showAll := fs.BoolP("output-duplicates", "v", false, "do not use * to mark line suppression")
	traditional := fs.Bool("traditional", false, "accept arguments in third traditional form")
	unsignedInt := fs.BoolP("unsigned-int", "D", false, "same as -t u4")
	floatDouble := fs.BoolP("float-double", "F", false, "same as -t f8")
	hexInt := fs.BoolP("hex-int", "H", false, "same as -t x4")
	decimalInt := fs.BoolP("decimal-int", "I", false, "same as -t d4")
	decimalLong := fs.BoolP("decimal-long", "L", false, "same as -t d8")
	octalInt := fs.BoolP("octal-int", "O", false, "same as -t o4")
	hexCap := fs.BoolP("hex-cap", "X", false, "same as -t x4")
	floatDoubleShort := fs.BoolP("float-double-short", "e", false, "same as -t f8")
	floatSingle := fs.BoolP("float-single", "f", false, "same as -t f4")
	signedInt := fs.BoolP("signed-int", "i", false, "same as -t d4")
	signedLong := fs.BoolP("signed-long", "l", false, "same as -t d8")
	signedShort := fs.BoolP("signed-short", "s", false, "same as -t d2")
	operands, code := tool.Parse(rc, cmd, fs, args)
	if code >= 0 {
		return code
	}
	// Alias-derived -t values carry aliasMark; a value without it proves
	// a literal -t/--format on the command line (the offset gate below
	// distinguishes the two: XSI -bcdosx do not close the gate, -t does).
	selectedFormats := make([]string, 0, len(*formats))
	explicitFormat := false
	for _, v := range *formats {
		if stripped, ok := strings.CutPrefix(v, aliasMark); ok {
			selectedFormats = append(selectedFormats, stripped)
		} else {
			explicitFormat = true
			selectedFormats = append(selectedFormats, v)
		}
	}
	// XSI offset operand [+]offset[.][b] (POSIX.1-2016 od OPERANDS):
	// recognized only when there are no more than two operands, none of
	// -A, -j, -N, -t, or -v is specified, and the last operand begins
	// with '+' — or is the second of exactly two operands and begins
	// with a digit. --traditional additionally accepts a trailing or
	// leading +offset regardless of those conditions (GNU third form).
	if len(operands) > 0 {
		last := operands[len(operands)-1]
		plusForm := strings.HasPrefix(last, "+")
		digitForm := len(operands) == 2 && len(last) > 0 && last[0] >= '0' && last[0] <= '9'
		gateOpen := len(operands) <= 2 && !explicitFormat &&
			!fs.Lookup("address-radix").Changed &&
			!fs.Lookup("skip-bytes").Changed &&
			!fs.Lookup("read-bytes").Changed &&
			!fs.Lookup("output-duplicates").Changed
		if (gateOpen && (plusForm || digitForm)) || (*traditional && plusForm) {
			off, err := parseTraditionalOffset(strings.TrimPrefix(last, "+"))
			if err != nil || off < 0 {
				return tool.UsageError(rc, cmd, "invalid offset: %q", last)
			}
			*skipText = strconv.FormatInt(off, 10)
			operands = operands[:len(operands)-1]
		}
	}
	if *traditional && len(operands) > 0 && strings.HasPrefix(operands[0], "+") {
		traditionalSkip, err := parseTraditionalOffset(strings.TrimPrefix(operands[0], "+"))
		if err != nil || traditionalSkip < 0 {
			return tool.UsageError(rc, cmd, "invalid offset: %q", operands[0])
		}
		*skipText = strconv.FormatInt(traditionalSkip, 10)
		operands = operands[1:]
	}
	for _, choice := range []struct {
		on     bool
		format string
	}{
		{*namedChars, "a"},
		{*octalBytes, "o1"},
		{*chars, "c"},
		{*unsignedDecimal, "u2"},
		{*octalWords, "o2"},
		{*hexWords, "x2"},
		{*unsignedInt, "u4"},
		{*floatDouble, "f8"},
		{*hexInt, "x4"},
		{*decimalInt, "d4"},
		{*decimalLong, "d8"},
		{*octalInt, "o4"},
		{*hexCap, "x4"},
		{*floatDoubleShort, "f8"},
		{*floatSingle, "f4"},
		{*signedInt, "d4"},
		{*signedLong, "d8"},
		{*signedShort, "d2"},
	} {
		if choice.on {
			selectedFormats = append(selectedFormats, choice.format)
		}
	}
	if len(selectedFormats) == 0 {
		selectedFormats = []string{"o2"}
	}
	parsedFormats, err := parseFormats(selectedFormats)
	if err != nil {
		return tool.UsageError(rc, cmd, "%v", err)
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
	var byteOrder binary.ByteOrder = binary.LittleEndian
	switch *endianText {
	case "little":
	case "big":
		byteOrder = binary.BigEndian
	default:
		return tool.UsageError(rc, cmd, "invalid endian: %q", *endianText)
	}
	minString := 0
	if fs.Lookup("strings").Changed {
		minString = 3
		if *stringsText != "" {
			n, err := strconv.Atoi(*stringsText)
			if err != nil || n <= 0 {
				return tool.UsageError(rc, cmd, "invalid string length: %q", *stringsText)
			}
			minString = n
		}
	}
	o := options{addrRadix: *addrRadix, formats: parsedFormats, endian: byteOrder, strings: minString, limit: limit, skip: skip, width: *width, showAll: *showAll}
	if o.addrRadix != "d" && o.addrRadix != "o" && o.addrRadix != "x" && o.addrRadix != "n" {
		return tool.UsageError(rc, cmd, "invalid address radix: %q", o.addrRadix)
	}
	if o.width <= 0 {
		return tool.UsageError(rc, cmd, "invalid output width: %d", o.width)
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
		if errors.Is(err, errSkipPastEOF) {
			fmt.Fprintf(rc.Err, "od: %v\n", err)
		} else {
			fmt.Fprintf(rc.Err, "od: %v\n", tool.SysErr(err))
		}
		exit = 1
	}
	if err := w.Flush(); err != nil {
		fmt.Fprintf(rc.Err, "od: write error: %v\n", err)
		return 1
	}
	return exit
}

// aliasMark prefixes -t values synthesized from format alias options so
// run can tell them apart from a literal -t/--format (which closes the
// XSI offset-operand gate). NUL cannot appear in a real argv string.
const aliasMark = "\x00"

// formatAliasFlags maps od's single-letter format options — the XSI
// -bcdosx set plus -a and GNU's traditional type aliases — to their -t
// equivalents. POSIX requires output lines in the order the type
// options appear, so expandFormatArgs rewrites each occurrence to a -t
// token in place instead of collecting the flags after parsing.
var formatAliasFlags = map[byte]string{
	'a': "a", 'b': "o1", 'c': "c", 'd': "u2", 'o': "o2", 's': "d2", 'x': "x2",
	'D': "u4", 'F': "f8", 'H': "x4", 'I': "d4", 'L': "d8", 'O': "o4", 'X': "x4",
	'e': "f8", 'f': "f4", 'i': "d4", 'l': "d8",
}

// longFormatAliasFlags are the exact long spellings of the same format
// options (extensions; POSIX defines only the short forms).
var longFormatAliasFlags = map[string]string{
	"--named-chars": "a", "--octal-bytes": "o1", "--characters": "c",
	"--unsigned-decimal-2": "u2", "--octal-2": "o2", "--hex-2": "x2",
	"--unsigned-int": "u4", "--float-double": "f8", "--hex-int": "x4",
	"--decimal-int": "d4", "--decimal-long": "d8", "--octal-int": "o4",
	"--hex-cap": "x4", "--float-double-short": "f8", "--float-single": "f4",
	"--signed-int": "d4", "--signed-long": "d8", "--signed-short": "d2",
}

// valueShorthands are od's short options that take an option-argument;
// inside a bundle the rest of the bundle (or the next arg) is theirs.
var valueShorthands = map[byte]bool{'A': true, 'j': true, 'N': true, 't': true, 'S': true, 'w': true}

func expandFormatArgs(args []string) []string {
	out := make([]string, 0, len(args))
	for idx, arg := range args {
		if arg == "--" {
			out = append(out, args[idx:]...)
			break
		}
		if format, ok := longFormatAliasFlags[arg]; ok {
			out = append(out, "-t", aliasMark+format)
			continue
		}
		if len(arg) < 2 || arg[0] != '-' || arg[1] == '-' {
			out = append(out, arg)
			continue
		}
		expanded, ok := expandShortBundle(arg)
		if !ok {
			out = append(out, arg)
			continue
		}
		out = append(out, expanded...)
	}
	return out
}

// expandShortBundle splits a short-option bundle like -bcx or -vtx1 so
// each format alias becomes its own ordered -t token. Bundles holding
// an unknown shorthand are left untouched for pflag to diagnose.
func expandShortBundle(arg string) ([]string, bool) {
	var out []string
	body := arg[1:]
	for i := 0; i < len(body); i++ {
		ch := body[i]
		if format, ok := formatAliasFlags[ch]; ok {
			out = append(out, "-t", aliasMark+format)
			continue
		}
		if ch == 'v' {
			out = append(out, "-v")
			continue
		}
		if valueShorthands[ch] {
			out = append(out, "-"+body[i:])
			return out, true
		}
		return nil, false
	}
	return out, true
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
			return errSkipPastEOF
		}
	}
	if o.limit >= 0 {
		r = io.LimitReader(r, o.limit)
	}
	if o.strings > 0 {
		return dumpStrings(r, w, o)
	}
	block := make([]byte, o.width)
	prev := make([]byte, 0, o.width)
	offset := o.skip
	suppressing := false
	for {
		n, err := io.ReadFull(r, block)
		if n > 0 {
			// GNU default: consecutive identical lines are elided and
			// marked with a single "*"; -v outputs them all.
			if !o.showAll && n == o.width && len(prev) == o.width && bytes.Equal(block[:n], prev) {
				if !suppressing {
					if _, werr := w.WriteString("*\n"); werr != nil {
						return werr
					}
					suppressing = true
				}
			} else {
				suppressing = false
				prev = append(prev[:0], block[:n]...)
				if werr := writeLine(w, offset, block[:n], o); werr != nil {
					return werr
				}
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
	for i, format := range o.formats {
		if o.addrRadix != "n" {
			prefix := formatOffset(offset, o.addrRadix)
			if i > 0 {
				prefix = strings.Repeat(" ", len(prefix))
			}
			if _, err := fmt.Fprintf(w, "%s", prefix); err != nil {
				return err
			}
		}
		if err := writeFormat(w, b, format, o.endian); err != nil {
			return err
		}
		if _, err := w.WriteString("\n"); err != nil {
			return err
		}
	}
	return nil
}

func writeFormat(w *bufio.Writer, b []byte, format dumpFormat, order binary.ByteOrder) error {
	switch format.kind {
	case "x1":
		for _, c := range b {
			fmt.Fprintf(w, " %02x", c)
		}
	case "x2":
		writeInts(w, b, 2, false, 16, order)
	case "x4":
		writeInts(w, b, 4, false, 16, order)
	case "x8":
		writeInts(w, b, 8, false, 16, order)
	case "o1":
		for _, c := range b {
			fmt.Fprintf(w, " %03o", c)
		}
	case "o2":
		writeInts(w, b, 2, false, 8, order)
	case "o4":
		writeInts(w, b, 4, false, 8, order)
	case "u1":
		for _, c := range b {
			fmt.Fprintf(w, " %3d", c)
		}
	case "u2":
		writeInts(w, b, 2, false, 10, order)
	case "u4":
		writeInts(w, b, 4, false, 10, order)
	case "d1":
		for _, c := range b {
			fmt.Fprintf(w, " %4d", int8(c))
		}
	case "d2":
		writeInts(w, b, 2, true, 10, order)
	case "d4":
		writeInts(w, b, 4, true, 10, order)
	case "d8":
		writeInts(w, b, 8, true, 10, order)
	case "o8":
		writeInts(w, b, 8, false, 8, order)
	case "u8":
		writeInts(w, b, 8, false, 10, order)
	case "f4":
		for _, v := range words(b, 4, order) {
			fmt.Fprintf(w, " %.7g", math.Float32frombits(uint32(v)))
		}
	case "f8":
		for _, v := range words(b, 8, order) {
			fmt.Fprintf(w, " %.14g", math.Float64frombits(v))
		}
	case "c":
		for _, c := range b {
			fmt.Fprintf(w, " %3s", cChar(c))
		}
	case "a":
		for _, c := range b {
			fmt.Fprintf(w, " %3s", namedChar(c))
		}
	}
	return nil
}

func writeInts(w *bufio.Writer, b []byte, size int, signed bool, base int, order binary.ByteOrder) {
	for _, v := range words(b, size, order) {
		if signed {
			shift := uint(64 - size*8)
			sv := int64(v<<shift) >> shift
			fmt.Fprintf(w, intFormat(base, size, true), sv)
			continue
		}
		fmt.Fprintf(w, intFormat(base, size, false), v)
	}
}

func intFormat(base, size int, signed bool) string {
	if signed {
		return " %d"
	}
	switch base {
	case 8:
		return " %0" + strconv.Itoa(size*3) + "o"
	case 16:
		return " %0" + strconv.Itoa(size*2) + "x"
	default:
		return " %d"
	}
}

func words(b []byte, size int, order binary.ByteOrder) []uint64 {
	var out []uint64
	for i := 0; i < len(b); i += size {
		var buf [8]byte
		chunk := b[i:min(i+size, len(b))]
		n := len(chunk)
		if order == binary.BigEndian {
			copy(buf[size-n:size], chunk)
		} else {
			copy(buf[:], chunk)
		}
		switch size {
		case 1:
			out = append(out, uint64(buf[0]))
		case 2:
			out = append(out, uint64(order.Uint16(buf[:2])))
		case 4:
			out = append(out, uint64(order.Uint32(buf[:4])))
		case 8:
			out = append(out, order.Uint64(buf[:8]))
		}
	}
	return out
}

// dumpStrings implements -S per GNU: a string constant is at least
// o.strings printable characters followed by a NUL byte. Runs cut off
// by EOF (no NUL) are not printed, and no trailing offset line is
// emitted.
func dumpStrings(r io.Reader, w *bufio.Writer, o options) error {
	data, err := io.ReadAll(r)
	if err != nil {
		return err
	}
	start := -1
	for i, c := range data {
		if isStringByte(c) {
			if start < 0 {
				start = i
			}
			continue
		}
		if c == 0 && start >= 0 && i-start >= o.strings {
			if o.addrRadix != "n" {
				fmt.Fprintf(w, "%s ", formatOffset(o.skip+int64(start), o.addrRadix))
			}
			fmt.Fprintf(w, "%s\n", data[start:i])
		}
		start = -1
	}
	return nil
}

func isStringByte(c byte) bool {
	return c >= 32 && c <= 126
}

func parseFormats(values []string) ([]dumpFormat, error) {
	var formats []dumpFormat
	for _, value := range values {
		// First split by spaces/commas/tabs (explicit separators)
		for _, token := range strings.FieldsFunc(value, func(r rune) bool { return r == ',' || r == ' ' || r == '\t' }) {
			// Check if the entire token is a format alias first
			if alias, ok := formatAliases[token]; ok {
				// It's a format alias like "char" -> "c"
				fmts, err := parseConcatenatedFormats(alias)
				if err != nil {
					return nil, err
				}
				formats = append(formats, fmts...)
				continue
			}
			// Then parse concatenated format strings within each token
			fmts, err := parseConcatenatedFormats(token)
			if err != nil {
				return nil, err
			}
			formats = append(formats, fmts...)
		}
	}
	return formats, nil
}

// parseConcatenatedFormats handles multiple format specs concatenated together,
// e.g., "x1x2" → [x1, x2], or "x1o1c" → [x1, o1, c], or "octal1hex1" → [o1, x1]
func parseConcatenatedFormats(s string) ([]dumpFormat, error) {
	var formats []dumpFormat
	i := 0
	for i < len(s) {
		// Check for named format aliases first (e.g., "octal", "hex", "signed", "unsigned")
		matched := false
		for name, alias := range formatAliases {
			if len(name) > 1 && strings.HasPrefix(s[i:], name) {
				typeChar := alias[0] // e.g. 'o' from "octal"
				i += len(name)
				start := i
				for i < len(s) && s[i] >= '0' && s[i] <= '9' {
					i++
				}
				if i == start && i < len(s) && s[i] >= 'A' && s[i] <= 'Z' {
					i++
				}
				sizeText := s[start:i]
				if sizeText == "" {
					formats = append(formats, parseFormatWithDefault(string(typeChar)))
				} else {
					f, err := formatForType(typeChar, sizeText, s)
					if err != nil {
						return nil, err
					}
					formats = append(formats, f)
				}
				matched = true
				break
			}
		}

		if matched {
			continue
		}

		// Handle single-character type letters
		if i >= len(s) || !isFormatTypeChar(s[i]) {
			return nil, fmt.Errorf("unsupported output format: %q", s)
		}

		typeChar := s[i]
		i++

		// a and c never take a size (POSIX)
		if typeChar == 'a' || typeChar == 'c' {
			formats = append(formats, dumpFormat{kind: string(typeChar), size: 1})
			continue
		}

		// Named word sizes (extension): xchar, dshort, uint, olong
		namedSize := ""
		for _, name := range []string{"char", "short", "int", "long"} {
			if strings.HasPrefix(s[i:], name) {
				namedSize = name
				break
			}
		}
		if namedSize != "" {
			i += len(namedSize)
			f, err := formatForType(typeChar, namedSize, s)
			if err != nil {
				return nil, err
			}
			formats = append(formats, f)
			continue
		}

		// A decimal byte count, or one POSIX size letter
		start := i
		for i < len(s) && s[i] >= '0' && s[i] <= '9' {
			i++
		}
		if i == start && i < len(s) && s[i] >= 'A' && s[i] <= 'Z' {
			i++
		}
		sizeText := s[start:i]
		if sizeText == "" {
			formats = append(formats, parseFormatWithDefault(string(typeChar)))
			continue
		}
		f, err := formatForType(typeChar, sizeText, s)
		if err != nil {
			return nil, err
		}
		formats = append(formats, f)
	}

	return formats, nil
}

// formatForType builds the dump format for one type letter with an
// explicit size modifier; spec is the whole type_string, for errors.
func formatForType(typeChar byte, sizeText, spec string) (dumpFormat, error) {
	if typeChar != 'x' && typeChar != 'o' && typeChar != 'u' && typeChar != 'd' && typeChar != 'f' {
		return dumpFormat{}, fmt.Errorf("unsupported output format: %q", spec)
	}
	if n, ok := sizeFromLetter(typeChar, sizeText); ok {
		sizeText = strconv.Itoa(n)
	}
	size, err := strconv.Atoi(sizeText)
	if err != nil || (size != 1 && size != 2 && size != 4 && size != 8) {
		return dumpFormat{}, fmt.Errorf("unsupported output format: %q", spec)
	}
	if typeChar == 'f' && size != 4 && size != 8 {
		return dumpFormat{}, fmt.Errorf("unsupported output format: %q", spec)
	}
	return dumpFormat{kind: string(typeChar) + strconv.Itoa(size), size: size}, nil
}

// sizeFromLetter resolves the POSIX size letters: after f only F, D,
// or L (float, double, long double); after d/o/u/x only C, S, I, or L
// (char, short, int, long), plus their word spellings (extension).
// long double maps to 8 bytes: Go has no wider float type and
// sizeof(long double) is 8 on this implementation's primary targets.
func sizeFromLetter(typeChar byte, mod string) (int, bool) {
	if typeChar == 'f' {
		switch mod {
		case "F":
			return 4, true
		case "D", "L":
			return 8, true
		}
		return 0, false
	}
	switch mod {
	case "C", "char":
		return 1, true
	case "S", "short":
		return 2, true
	case "I", "int":
		return 4, true
	case "L", "long":
		return 8, true
	}
	return 0, false
}

func parseFormatWithDefault(prefix string) dumpFormat {
	switch prefix {
	case "f":
		return dumpFormat{kind: "f8", size: 8} // double
	case "a", "c":
		return dumpFormat{kind: prefix, size: 1}
	default:
		return dumpFormat{kind: prefix + "4", size: 4} // int
	}
}

func isFormatTypeChar(c byte) bool {
	return c == 'a' || c == 'c' || c == 'd' || c == 'f' || c == 'o' || c == 'u' || c == 'x'
}

var formatAliases = map[string]string{
	"char":     "c",
	"ascii":    "a",
	"named":    "a",
	"float":    "f8",
	"double":   "f8",
	"octal":    "o",
	"hex":      "x",
	"signed":   "d",
	"unsigned": "u",
}

func formatOffset(n int64, radix string) string {
	switch radix {
	case "d":
		return fmt.Sprintf("%07d", n)
	case "x":
		// GNU prints hexadecimal addresses 6 digits wide.
		return fmt.Sprintf("%06x", n)
	default:
		return fmt.Sprintf("%07o", n)
	}
}

// parseTraditionalOffset parses the pre-POSIX trailing [+]offset
// operand: octal by default, decimal with a trailing '.', hex with a
// leading 0x/0X, and a trailing 'b' multiplies by 512.
func parseTraditionalOffset(s string) (int64, error) {
	if strings.HasPrefix(s, "0x") || strings.HasPrefix(s, "0X") {
		return strconv.ParseInt(s[2:], 16, 64)
	}
	mult := int64(1)
	if strings.HasSuffix(s, "b") {
		mult = 512
		s = strings.TrimSuffix(s, "b")
	}
	base := 8
	if strings.HasSuffix(s, ".") {
		base = 10
		s = strings.TrimSuffix(s, ".")
	}
	n, err := strconv.ParseInt(s, base, 64)
	if err != nil {
		return 0, err
	}
	return n * mult, nil
}

// cChar renders one byte for -t c: printable bytes as themselves
// (space stays a blank), the C backslash escapes, 3-digit octal for
// the rest.
func cChar(c byte) string {
	switch c {
	case 0:
		return "\\0"
	case '\a':
		return "\\a"
	case '\b':
		return "\\b"
	case '\t':
		return "\\t"
	case '\n':
		return "\\n"
	case '\v':
		return "\\v"
	case '\f':
		return "\\f"
	case '\r':
		return "\\r"
	}
	if c >= 32 && c <= 126 {
		return string(c)
	}
	return fmt.Sprintf("%03o", c)
}

// asciiNames are the -t a named characters for bytes 0-32 (POSIX od).
var asciiNames = [...]string{
	"nul", "soh", "stx", "etx", "eot", "enq", "ack", "bel",
	"bs", "ht", "nl", "vt", "ff", "cr", "so", "si",
	"dle", "dc1", "dc2", "dc3", "dc4", "nak", "syn", "etb",
	"can", "em", "sub", "esc", "fs", "gs", "rs", "us", "sp",
}

// namedChar renders one byte for -t a: named characters, ignoring the
// high-order bit (GNU od manual).
func namedChar(c byte) string {
	c &= 0x7f
	if int(c) < len(asciiNames) {
		return asciiNames[c]
	}
	if c == 0x7f {
		return "del"
	}
	return string(c)
}

var multipliers = map[string]int64{
	"":    1,
	"b":   512,         // POSIX
	"k":   1024,        // POSIX
	"m":   1024 * 1024, // POSIX
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
	base := 10
	digitsStart := 0
	if strings.HasPrefix(s, "0x") || strings.HasPrefix(s, "0X") {
		base = 16
		digitsStart = 2
	} else if len(s) > 0 && s[0] == '0' {
		base = 8
	}

	i := digitsStart
	for i < len(s) && digitInBase(s[i], base) {
		i++
	}
	if i == digitsStart {
		return 0, strconv.ErrSyntax
	}
	n, err := strconv.ParseInt(s[digitsStart:i], base, 64)
	if err != nil {
		return 0, err
	}
	m, ok := multipliers[s[i:]]
	if !ok {
		return 0, strconv.ErrSyntax
	}
	if m != 0 && n > math.MaxInt64/m {
		return 0, strconv.ErrRange
	}
	return n * m, nil
}

func digitInBase(c byte, base int) bool {
	if c >= '0' && c <= '9' {
		return int(c-'0') < base
	}
	if base == 16 {
		return c >= 'a' && c <= 'f' || c >= 'A' && c <= 'F'
	}
	return false
}
