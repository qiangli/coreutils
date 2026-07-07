// Package seqcmd implements seq(1) per the GNU coreutils manual:
// print numbers from FIRST to LAST, in steps of INCREMENT.
//
// Portions adapted from https://github.com/u-root/u-root cmds/core/seq/seq.go (BSD-3-Clause)
// and https://github.com/guonaihong/coreutils seq/seq.go (Apache-2.0).
// Changes: rewired to the tool framework; GNU default-format rules
// (integer fast path, %.PRECf with precision from FIRST/INCREMENT,
// %g for scientific operands); GNU -w width derivation from operand
// widths; removed u-root's automatic increment sign-flip (GNU prints
// nothing for "seq 5 1"); -f/-w conflict and format validation per
// the manual.
package seqcmd

import (
	"bufio"
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/qiangli/coreutils/tool"
)

var cmd = &tool.Tool{
	Name:     "seq",
	Synopsis: "Print numbers from FIRST to LAST, in steps of INCREMENT.",
	Usage:    "seq [OPTION]... LAST\n   or: seq [OPTION]... FIRST LAST\n   or: seq [OPTION]... FIRST INCREMENT LAST",
}

func init() { cmd.Run = run; tool.Register(cmd) }

func run(rc *tool.RunContext, args []string) int {
	// Pre-scan: negative numbers ("-5", "-.3") are operands, not
	// flags — GNU seq stops option parsing for them. Keep flag-ish
	// tokens (and the value following -s/-f) for the flag parser.
	flagArgs, operands := splitArgs(args)

	fs := tool.NewFlags(cmd.Name)
	format := fs.StringP("format", "f", "", "use printf style floating-point FORMAT")
	sep := fs.StringP("separator", "s", "\n", "use STRING to separate numbers")
	equalWidth := fs.BoolP("equal-width", "w", false, "equalize width by padding with leading zeroes")
	terminator := fs.StringP("terminator", "t", "\n", "use STRING to terminate each line")
	parsed, code := tool.Parse(rc, cmd, fs, flagArgs)
	if code >= 0 {
		return code
	}
	operands = append(operands, parsed...)

	firstStr, incrStr := "1", "1"
	var lastStr string
	switch len(operands) {
	case 1:
		lastStr = operands[0]
	case 2:
		firstStr, lastStr = operands[0], operands[1]
	case 3:
		firstStr, incrStr, lastStr = operands[0], operands[1], operands[2]
	case 0:
		return tool.UsageError(rc, cmd, "missing operand")
	default:
		return tool.UsageError(rc, cmd, "extra operand %q", operands[3])
	}

	first, err := strconv.ParseFloat(firstStr, 64)
	if err != nil {
		return tool.UsageError(rc, cmd, "invalid floating point argument: %q", firstStr)
	}
	incr, err := strconv.ParseFloat(incrStr, 64)
	if err != nil {
		return tool.UsageError(rc, cmd, "invalid floating point argument: %q", incrStr)
	}
	last, err := strconv.ParseFloat(lastStr, 64)
	if err != nil {
		return tool.UsageError(rc, cmd, "invalid floating point argument: %q", lastStr)
	}
	if incr == 0 {
		return tool.UsageError(rc, cmd, "invalid Zero increment value: %q", incrStr)
	}

	if fs.Changed("format") && *equalWidth {
		return tool.UsageError(rc, cmd, "format string may not be specified when printing equal width strings")
	}

	out := bufio.NewWriter(rc.Out)
	defer out.Flush()

	if fs.Changed("format") {
		f, errMsg := normalizeFormat(*format)
		if errMsg != "" {
			return tool.UsageError(rc, cmd, "%s", errMsg)
		}
		printFloats(out, f, *sep, *terminator, first, incr, last)
		return 0
	}

	pFirst, pIncr, pLast := precisionOf(firstStr), precisionOf(incrStr), precisionOf(lastStr)
	scientific := pFirst < 0 || pIncr < 0 || pLast < 0
	prec := max(pFirst, pIncr)

	// Integer fast path: all operands written as plain integers.
	if !scientific && pFirst == 0 && pIncr == 0 && pLast == 0 {
		fi, err1 := strconv.ParseInt(firstStr, 10, 64)
		ii, err2 := strconv.ParseInt(incrStr, 10, 64)
		li, err3 := strconv.ParseInt(lastStr, 10, 64)
		if err1 == nil && err2 == nil && err3 == nil {
			width := 0
			if *equalWidth {
				width = max(len(firstStr), len(lastStr))
			}
			printInts(out, *sep, *terminator, width, fi, ii, li)
			return 0
		}
	}

	var f string
	switch {
	case scientific:
		// GNU uses %Lg (default precision 6) for exponent operands;
		// -w is not applied in this case.
		f = "%.6g"
	case *equalWidth:
		adjust := func(w, p int) int {
			fw := w + (prec - p)
			if p > 0 && prec == 0 {
				fw-- // no room needed for '.'
			}
			if p == 0 && prec > 0 {
				fw++ // room for '.'
			}
			return fw
		}
		width := max(adjust(len(firstStr), pFirst), adjust(len(lastStr), pLast))
		f = fmt.Sprintf("%%0%d.%df", width, prec)
	default:
		if prec == 0 {
			// GNU uses %Lg when neither FIRST nor INCREMENT has a
			// fractional part (e.g. "seq 1. 2.5").
			f = "%.6g"
		} else {
			f = fmt.Sprintf("%%.%df", prec)
		}
	}
	printFloats(out, f, *sep, *terminator, first, incr, last)
	return 0
}

// splitArgs separates number-like tokens (operands) from flag tokens
// before pflag sees them, so "seq -5 5" and "seq 5 -1 1" parse.
func splitArgs(args []string) (flagArgs, operands []string) {
	expectValue := false
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case expectValue:
			flagArgs = append(flagArgs, a)
			expectValue = false
		case a == "--":
			operands = append(operands, args[i+1:]...)
			return
		case len(a) > 1 && a[0] == '-' && (a[1] == '.' || (a[1] >= '0' && a[1] <= '9')):
			operands = append(operands, a)
		case len(a) > 1 && a[0] == '-':
			flagArgs = append(flagArgs, a)
			if a == "-s" || a == "-f" || a == "-t" || a == "--separator" || a == "--format" || a == "--terminator" {
				expectValue = true
			}
		default:
			operands = append(operands, a)
		}
	}
	return
}

// precisionOf returns the count of digits after the decimal point in
// the operand as written, or -1 for scientific notation (GNU's
// INT_MAX marker).
func precisionOf(s string) int {
	if strings.ContainsAny(s, "eE") {
		return -1
	}
	if i := strings.IndexByte(s, '.'); i >= 0 {
		return len(s) - i - 1
	}
	return 0
}

// normalizeFormat validates a -f FORMAT per the GNU rules (exactly
// one %[flags][width][.precision]{e,f,g,E,F,G} directive; %% is
// literal) and injects the C default precision 6 when none is given
// (Go's fmt would otherwise print the shortest representation).
// Returns ("", message) on invalid formats. The C-only %a/%A hex
// conversions are rejected — Go cannot reproduce their exact shape.
func normalizeFormat(f string) (string, string) {
	directives := 0
	var b strings.Builder
	for i := 0; i < len(f); i++ {
		if f[i] != '%' {
			b.WriteByte(f[i])
			continue
		}
		if i+1 < len(f) && f[i+1] == '%' {
			b.WriteString("%%")
			i++
			continue
		}
		j := i + 1
		for j < len(f) && strings.IndexByte("+-# 0'", f[j]) >= 0 {
			if f[j] == '\'' {
				return "", fmt.Sprintf("invalid format string: %q (the ' grouping flag is not supported)", f)
			}
			j++
		}
		for j < len(f) && f[j] >= '0' && f[j] <= '9' {
			j++
		}
		hasPrec := false
		if j < len(f) && f[j] == '.' {
			hasPrec = true
			j++
			for j < len(f) && f[j] >= '0' && f[j] <= '9' {
				j++
			}
		}
		if j >= len(f) {
			return "", fmt.Sprintf("format %q ends in %%", f)
		}
		switch f[j] {
		case 'e', 'f', 'g', 'E', 'F', 'G':
			directives++
			b.WriteString(f[i:j])
			if !hasPrec {
				b.WriteString(".6")
			}
			b.WriteByte(f[j])
		case 'a', 'A':
			return "", fmt.Sprintf("format %q uses the %%%c directive, which is not supported by pure-Go coreutils", f, f[j])
		default:
			return "", fmt.Sprintf("format %q has unknown %%%c directive", f, f[j])
		}
		i = j
	}
	if directives == 0 {
		return "", fmt.Sprintf("format %q has no %% directive", f)
	}
	if directives > 1 {
		return "", fmt.Sprintf("format %q has too many %% directives", f)
	}
	return b.String(), ""
}

func printInts(out *bufio.Writer, sep, term string, width int, first, incr, last int64) {
	printed := false
	for v := first; (incr > 0 && v <= last) || (incr < 0 && v >= last); {
		if printed {
			out.WriteString(sep)
		}
		if width > 0 {
			fmt.Fprintf(out, "%0*d", width, v)
		} else {
			out.WriteString(strconv.FormatInt(v, 10))
		}
		printed = true
		next := v + incr
		if (incr > 0 && next < v) || (incr < 0 && next > v) {
			break // int64 overflow
		}
		v = next
	}
	if printed {
		out.WriteString(term)
	}
}

// printFloats mirrors GNU's loop: x = first + i*incr, terminating
// when x passes last (no epsilon — the multiply, not repeated
// addition, is what keeps "seq 0 0.1 1" landing exactly on 1).
func printFloats(out *bufio.Writer, format, sep, term string, first, incr, last float64) {
	printed := false
	for i := 0; ; i++ {
		x := first + float64(i)*incr
		if incr > 0 && x > last || incr < 0 && x < last || math.IsInf(x, 0) {
			break
		}
		if printed {
			out.WriteString(sep)
		}
		fmt.Fprintf(out, format, x)
		printed = true
	}
	if printed {
		out.WriteString(term)
	}
}
