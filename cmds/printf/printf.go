// Package printfcmd implements the standalone printf(1) utility per the
// POSIX printf specification and the GNU coreutils manual: format ARGUMENTs
// according to FORMAT, reusing FORMAT as necessary to consume every
// ARGUMENT, and writing the result to standard output.
//
// Fresh pure-Go implementation — not adapted from prior art. FORMAT and
// ARGUMENT are always operands, never flags: only a sole "--help"/"-h" or
// "--version"/"-V" argument is special-cased (matching this repo's
// expr/factor-style commands, which face the same "operands can start with
// -" problem printf does), and a leading "--" ends option scanning so a
// literal FORMAT beginning with "-" can still be passed.
package printfcmd

import (
	"bytes"
	"fmt"
	"io"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/qiangli/coreutils/tool"
)

var cmd = &tool.Tool{
	Name:     "printf",
	Synopsis: "Format and print ARGUMENT(s) according to FORMAT.",
	Usage:    "printf FORMAT [ARGUMENT]...\n   or: printf OPTION",
}

func init() { cmd.Run = run; tool.Register(cmd) }

func run(rc *tool.RunContext, args []string) int {
	if len(args) == 1 && (args[0] == "--help" || args[0] == "-h") {
		printHelp(rc)
		return 0
	}
	if len(args) == 1 && (args[0] == "--version" || args[0] == "-V") {
		fmt.Fprintf(rc.Out, "%s (qiangli/coreutils) %s\n", cmd.Name, tool.Version)
		return 0
	}
	if len(args) > 0 && args[0] == "--" {
		args = args[1:]
	}
	if len(args) == 0 {
		return tool.UsageError(rc, cmd, "missing operand")
	}

	format := args[0]
	values := args[1:]

	var out bytes.Buffer
	pos := 0
	failed := false
	firstPass := true
	for firstPass || pos < len(values) {
		startPos := pos
		halt, fatal := formatOnce(rc, &out, format, values, &pos, &failed)
		if fatal || halt {
			break
		}
		firstPass = false
		if pos == startPos {
			break
		}
	}

	n, err := rc.Out.Write(out.Bytes())
	if err == nil && n != out.Len() {
		err = fmt.Errorf("short write")
	}
	if err != nil {
		fmt.Fprintf(rc.Err, "%s: write error: %v\n", cmd.Name, err)
		return 1
	}
	if failed {
		return 1
	}
	return 0
}

// formatOnce runs one pass of format against values, consuming ARGUMENTs
// through *pos and writing the result into out. halt is true when a \c
// escape was encountered (stop all further output, exit success). fatal is
// true on a malformed conversion specification (stop all further output,
// exit failure); *failed is set on any diagnostic, fatal or not.
func formatOnce(rc *tool.RunContext, out *bytes.Buffer, format string, values []string, pos *int, failed *bool) (halt, fatal bool) {
	nextArg := func() (string, bool) {
		if *pos < len(values) {
			s := values[*pos]
			*pos++
			return s, true
		}
		return "", false
	}

	i := 0
	for i < len(format) {
		c := format[i]
		switch c {
		case '\\':
			adv, stop := writeFormatEscape(out, format[i:])
			i += adv
			if stop {
				return true, false
			}
		case '%':
			specStart := i
			j := i + 1
			if j >= len(format) {
				diagInvalidConversion(rc, "%")
				*failed = true
				return false, true
			}
			if format[j] == '%' {
				out.WriteByte('%')
				i = j + 1
				continue
			}

			flagsStart := j
			for j < len(format) && strings.IndexByte("-+ 0#", format[j]) >= 0 {
				j++
			}
			flags := format[flagsStart:j]

			width := 0
			hasWidth := false
			widthFromArg := false
			if j < len(format) && format[j] == '*' {
				hasWidth, widthFromArg = true, true
				j++
			} else {
				wStart := j
				for j < len(format) && format[j] >= '0' && format[j] <= '9' {
					j++
				}
				if j > wStart {
					width, _ = strconv.Atoi(format[wStart:j])
					hasWidth = true
				}
			}

			precision := 0
			hasPrecision := false
			precFromArg := false
			if j < len(format) && format[j] == '.' {
				j++
				hasPrecision = true
				if j < len(format) && format[j] == '*' {
					precFromArg = true
					j++
				} else {
					pStart := j
					for j < len(format) && format[j] >= '0' && format[j] <= '9' {
						j++
					}
					precision, _ = strconv.Atoi(format[pStart:j])
				}
			}

			if j >= len(format) {
				diagInvalidConversion(rc, format[specStart:])
				*failed = true
				return false, true
			}
			conv := format[j]
			i = j + 1

			if widthFromArg {
				s, ok := nextArg()
				if ok {
					v, valid := parseIntArg(rc, s)
					if !valid {
						diagInvalidNumber(rc, s)
						*failed = true
						v = 0
					}
					width = int(v)
				}
			}
			if precFromArg {
				s, ok := nextArg()
				if ok {
					v, valid := parseIntArg(rc, s)
					if !valid {
						diagInvalidNumber(rc, s)
						*failed = true
						v = 0
					}
					precision = int(v)
				}
			}
			if hasWidth && width < 0 {
				if !strings.ContainsRune(flags, '-') {
					flags += "-"
				}
				width = -width
			}
			if hasPrecision && precision < 0 {
				hasPrecision = false
			}

			switch conv {
			case 's':
				s, _ := nextArg()
				writeStringField(out, s, flags, width, hasWidth, precision, hasPrecision)
			case 'b':
				if flags != "" || hasWidth || hasPrecision {
					diagInvalidConversion(rc, "%b")
					*failed = true
					return false, true
				}
				s, _ := nextArg()
				var bbuf bytes.Buffer
				stop := writeBEscapes(&bbuf, s)
				out.Write(bbuf.Bytes())
				if stop {
					return true, false
				}
			case 'c':
				s, _ := nextArg()
				if len(s) > 1 {
					s = s[:1]
				}
				writeStringField(out, s, flags, width, hasWidth, 0, false)
			case 'd', 'i':
				s, ok := nextArg()
				v := int64(0)
				if ok {
					var valid bool
					v, valid = parseIntArg(rc, s)
					if !valid {
						diagInvalidNumber(rc, s)
						*failed = true
						v = 0
					}
				}
				writeGoNumeric(out, "d", flags, width, hasWidth, precision, hasPrecision, v)
			case 'o', 'u', 'x', 'X':
				s, ok := nextArg()
				v := int64(0)
				if ok {
					var valid bool
					v, valid = parseIntArg(rc, s)
					if !valid {
						diagInvalidNumber(rc, s)
						*failed = true
						v = 0
					}
				}
				goConv := string(conv)
				if conv == 'u' {
					goConv = "d"
				}
				writeGoNumeric(out, goConv, flags, width, hasWidth, precision, hasPrecision, uint64(v))
			case 'e', 'E', 'f', 'F', 'g', 'G', 'a', 'A':
				s, ok := nextArg()
				fv := 0.0
				if ok {
					var valid bool
					fv, valid = parseFloatArg(rc, s)
					if !valid {
						diagInvalidNumber(rc, s)
						*failed = true
						fv = 0
					}
				}
				goConv := string(conv)
				switch conv {
				case 'a':
					goConv = "x"
				case 'A':
					goConv = "X"
				case 'g', 'G':
					if !hasPrecision {
						hasPrecision, precision = true, 6
					}
				}
				writeGoNumeric(out, goConv, flags, width, hasWidth, precision, hasPrecision, fv)
			default:
				diagInvalidConversion(rc, format[specStart:i])
				*failed = true
				return false, true
			}
		default:
			out.WriteByte(c)
			i++
		}
	}
	return false, false
}

func diagInvalidConversion(rc *tool.RunContext, spec string) {
	fmt.Fprintf(rc.Err, "%s: %s: invalid conversion specification\n", cmd.Name, spec)
}

func diagInvalidNumber(rc *tool.RunContext, s string) {
	fmt.Fprintf(rc.Err, "%s: invalid number: %s\n", cmd.Name, strconv.Quote(s))
}

// writeGoNumeric delegates a numeric conversion to Go's fmt package, which
// implements the same width/precision/flag semantics as C printf for
// integer and floating-point verbs (including precision as minimum digit
// count for integers, and default precision 6 for %e/%f).
func writeGoNumeric(out *bytes.Buffer, goConv, flags string, width int, hasWidth bool, precision int, hasPrecision bool, value any) {
	var verb strings.Builder
	verb.WriteByte('%')
	verb.WriteString(flags)
	if hasWidth {
		verb.WriteString(strconv.Itoa(width))
	}
	if hasPrecision {
		verb.WriteByte('.')
		verb.WriteString(strconv.Itoa(precision))
	}
	verb.WriteString(goConv)
	fmt.Fprintf(out, verb.String(), value)
}

// writeStringField applies POSIX field width/precision/left-justify to a raw
// byte string: precision truncates by byte count, width pads with spaces.
// Operating on bytes (never runes) keeps %s/%b/%c binary-safe.
func writeStringField(buf *bytes.Buffer, s string, flags string, width int, hasWidth bool, precision int, hasPrecision bool) {
	data := s
	if hasPrecision && precision < len(data) {
		data = data[:precision]
	}
	pad := 0
	if hasWidth && width > len(data) {
		pad = width - len(data)
	}
	left := strings.ContainsRune(flags, '-')
	if !left && pad > 0 {
		buf.WriteString(strings.Repeat(" ", pad))
	}
	buf.WriteString(data)
	if left && pad > 0 {
		buf.WriteString(strings.Repeat(" ", pad))
	}
}

// writeFormatEscape interprets one backslash escape at the start of s
// (s[0] == '\\') in the FORMAT string, writing its output to buf. adv is the
// number of bytes of s consumed; stop is true for \c (produce no further
// output at all).
func writeFormatEscape(buf *bytes.Buffer, s string) (adv int, stop bool) {
	if len(s) == 1 {
		buf.WriteByte('\\')
		return 1, false
	}
	switch s[1] {
	case 'a':
		buf.WriteByte('\a')
		return 2, false
	case 'b':
		buf.WriteByte('\b')
		return 2, false
	case 'c':
		return 2, true
	case 'e':
		buf.WriteByte(0x1b)
		return 2, false
	case 'f':
		buf.WriteByte('\f')
		return 2, false
	case 'n':
		buf.WriteByte('\n')
		return 2, false
	case 'r':
		buf.WriteByte('\r')
		return 2, false
	case 't':
		buf.WriteByte('\t')
		return 2, false
	case 'v':
		buf.WriteByte('\v')
		return 2, false
	case '\\':
		buf.WriteByte('\\')
		return 2, false
	case '"':
		buf.WriteByte('"')
		return 2, false
	case 'x':
		v, n := readHex(s[2:], 2)
		if n == 0 {
			buf.WriteString("\\x")
			return 2, false
		}
		buf.WriteByte(byte(v))
		return 2 + n, false
	case '0', '1', '2', '3', '4', '5', '6', '7':
		v, n := readOctal(s[1:], 3)
		buf.WriteByte(byte(v))
		return 1 + n, false
	default:
		buf.WriteByte('\\')
		buf.WriteByte(s[1])
		return 2, false
	}
}

// writeBEscapes interprets the %b escape set for one ARGUMENT: identical to
// writeFormatEscape except octal escapes require the POSIX %b form \0NNN
// (leading zero, 0 to 3 further octal digits) rather than the bare \NNN used
// in the FORMAT string. Returns true on \c (stop all further output).
func writeBEscapes(buf *bytes.Buffer, s string) bool {
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c != '\\' {
			buf.WriteByte(c)
			continue
		}
		if i == len(s)-1 {
			buf.WriteByte('\\')
			continue
		}
		i++
		switch s[i] {
		case 'a':
			buf.WriteByte('\a')
		case 'b':
			buf.WriteByte('\b')
		case 'c':
			return true
		case 'e':
			buf.WriteByte(0x1b)
		case 'f':
			buf.WriteByte('\f')
		case 'n':
			buf.WriteByte('\n')
		case 'r':
			buf.WriteByte('\r')
		case 't':
			buf.WriteByte('\t')
		case 'v':
			buf.WriteByte('\v')
		case '\\':
			buf.WriteByte('\\')
		case '"':
			buf.WriteByte('"')
		case 'x':
			v, n := readHex(s[i+1:], 2)
			if n == 0 {
				buf.WriteString("\\x")
			} else {
				buf.WriteByte(byte(v))
				i += n
			}
		case '0':
			v, n := readOctal(s[i+1:], 3)
			buf.WriteByte(byte(v))
			i += n
		default:
			buf.WriteByte('\\')
			buf.WriteByte(s[i])
		}
	}
	return false
}

func readOctal(s string, max int) (v, n int) {
	for n < max && n < len(s) && s[n] >= '0' && s[n] <= '7' {
		v = v*8 + int(s[n]-'0')
		n++
	}
	return v, n
}

func readHex(s string, max int) (v, n int) {
	for n < max && n < len(s) {
		d, ok := hexVal(s[n])
		if !ok {
			break
		}
		v = v*16 + d
		n++
	}
	return v, n
}

func hexVal(c byte) (int, bool) {
	switch {
	case c >= '0' && c <= '9':
		return int(c - '0'), true
	case c >= 'a' && c <= 'f':
		return int(c-'a') + 10, true
	case c >= 'A' && c <= 'F':
		return int(c-'A') + 10, true
	}
	return 0, false
}

// charConstant recognizes the POSIX numeric-argument character-constant
// syntax: an ARGUMENT beginning with ' or " whose value is the code point of
// the character immediately following the quote. warn is true when trailing
// characters after that one were ignored.
func charConstant(s string) (value rune, ok bool, warn bool) {
	if s == "" || (s[0] != '\'' && s[0] != '"') {
		return 0, false, false
	}
	rest := s[1:]
	if rest == "" {
		return 0, true, false
	}
	r, size := utf8.DecodeRuneInString(rest)
	return r, true, size < len(rest)
}

func parseIntArg(rc *tool.RunContext, s string) (int64, bool) {
	if r, ok, warn := charConstant(s); ok {
		if warn {
			fmt.Fprintf(rc.Err, "%s: warning: %s: character(s) following character constant have been ignored\n", cmd.Name, s)
		}
		return int64(r), true
	}
	v, err := strconv.ParseInt(strings.TrimLeft(s, " \t\n"), 0, 64)
	if err != nil {
		return 0, false
	}
	return v, true
}

func parseFloatArg(rc *tool.RunContext, s string) (float64, bool) {
	if r, ok, warn := charConstant(s); ok {
		if warn {
			fmt.Fprintf(rc.Err, "%s: warning: %s: character(s) following character constant have been ignored\n", cmd.Name, s)
		}
		return float64(r), true
	}
	v, err := strconv.ParseFloat(strings.TrimLeft(s, " \t\n"), 64)
	if err != nil {
		return 0, false
	}
	return v, true
}

func printHelp(rc *tool.RunContext) {
	fmt.Fprintf(rc.Out, "Usage: %s\n%s\n", cmd.Usage, cmd.Synopsis)
	io.WriteString(rc.Out, `
FORMAT controls the output the same way as in C printf. Interpreted
sequences in FORMAT are:

  \\        backslash            \a        alert (BEL)
  \b        backspace            \c        produce no further output
  \e        escape               \f        form feed
  \n        new line             \r        carriage return
  \t        horizontal tab       \v        vertical tab
  \"        double quote         \NNN      byte with octal value NNN (1-3 digits)
  \xHH      byte with hexadecimal value HH (1-2 digits)

  %%        a single %           %b        ARGUMENT with '\' escapes interpreted
  %c        first character of ARGUMENT    %s   ARGUMENT as a string
  %d %i %o %u %x %X               signed/unsigned/octal/hex integer conversions
  %e %E %f %F %g %G %a %A         floating-point conversions

FORMAT is reused as many times as necessary to convert all the given
ARGUMENTs; missing ARGUMENTs are treated as null strings or zero.

Options:
  -h, --help     display this help and exit
  -V, --version  output version information and exit
`)
}
