// Package numfmtcmd implements a compact numfmt(1) subset for common
// SI/IEC scaling conversions.
package numfmtcmd

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"math"
	"strconv"
	"strings"

	"github.com/qiangli/coreutils/tool"
)

var cmd = &tool.Tool{
	Name:     "numfmt",
	Synopsis: "Reformat numbers with optional SI/IEC unit scaling.",
	Usage:    "numfmt [OPTION]... [NUMBER]...",
}

func init() { cmd.Run = run; tool.Register(cmd) }

func run(rc *tool.RunContext, args []string) int {
	fs := tool.NewFlags(cmd.Name)
	delimiter := fs.StringP("delimiter", "d", "", "use X instead of whitespace for field delimiter")
	fieldSpec := fs.String("field", "1", "replace the numbers in these input fields")
	from := fs.String("from", "none", "auto-scale input numbers from UNIT system")
	fromUnit := fs.String("from-unit", "1", "specify the input unit size")
	to := fs.String("to", "none", "auto-scale output numbers to UNIT system")
	toUnit := fs.String("to-unit", "1", "specify the output unit size")
	format := fs.String("format", "%f", "use printf-style floating format")
	padding := fs.Int("padding", 0, "pad the output to N characters")
	suffix := fs.String("suffix", "", "accept and print SUFFIX after formatted numbers")
	unitSeparator := fs.String("unit-separator", "", "separate the number from any unit when printing")
	invalid := fs.String("invalid", "abort", "failure mode for invalid input: abort, fail, warn, ignore")
	header := fs.Int("header", 0, "print the first N header lines without conversion")
	zeroTerminated := fs.BoolP("zero-terminated", "z", false, "line delimiter is NUL, not newline")
	round := fs.String("round", "from-zero", "use METHOD for rounding: up, down, from-zero, towards-zero, nearest")
	grouping := fs.Bool("grouping", false, "use grouped digits in output")
	debug := fs.Bool("debug", false, "print warnings about invalid input")
	operands, code := tool.Parse(rc, cmd, fs, args)
	if code >= 0 {
		return code
	}
	fromScale, code := parseScale(rc, *from, "--from")
	if code >= 0 {
		return code
	}
	toScale, code := parseScale(rc, *to, "--to")
	if code >= 0 {
		return code
	}
	if toScale == scaleAuto {
		return tool.NotSupported(rc, cmd, "--to=auto (supported: none, si, iec, iec-i)")
	}
	fromUnitValue, code := parseUnitSize(rc, *fromUnit, "--from-unit")
	if code >= 0 {
		return code
	}
	toUnitValue, code := parseUnitSize(rc, *toUnit, "--to-unit")
	if code >= 0 {
		return code
	}
	effectiveFieldSpec := *fieldSpec
	if !fs.Changed("field") && len(operands) == 0 {
		effectiveFieldSpec = "-"
	}
	fields, code := parseFields(rc, effectiveFieldSpec)
	if code >= 0 {
		return code
	}
	mode, code := parseInvalidMode(rc, *invalid)
	if code >= 0 {
		return code
	}
	roundMode, code := parseRoundMode(rc, *round)
	if code >= 0 {
		return code
	}
	if *header < 0 {
		return tool.UsageError(rc, cmd, "invalid --header value: '%d'", *header)
	}
	opts := formatOptions{
		from:          fromScale,
		to:            toScale,
		fromUnit:      fromUnitValue,
		toUnit:        toUnitValue,
		format:        *format,
		padding:       *padding,
		suffix:        *suffix,
		unitSeparator: *unitSeparator,
		invalid:       mode,
		round:         roundMode,
		grouping:      *grouping,
		debug:         *debug,
	}

	if len(operands) > 0 {
		sep := "\n"
		if *zeroTerminated {
			sep = "\x00"
		}
		for _, op := range operands {
			out, ok := formatOne(rc, op, opts)
			if !ok {
				return 1
			}
			fmt.Fprint(rc.Out, out, sep)
		}
		return 0
	}
	records, err := readRecords(rc.In, *zeroTerminated)
	if err != nil {
		fmt.Fprintf(rc.Err, "numfmt: read error: %v\n", tool.SysErr(err))
		return 1
	}
	sep := "\n"
	if *zeroTerminated {
		sep = "\x00"
	}
	status := 0
	for i, rec := range records {
		if i < *header {
			fmt.Fprint(rc.Out, rec, sep)
			continue
		}
		line, ok := formatLine(rc, rec, *delimiter, fields, opts)
		if !ok {
			if mode == invalidAbort {
				return 1
			}
			if mode == invalidFail {
				status = 1
			}
		}
		fmt.Fprint(rc.Out, line, sep)
	}
	return status
}

type formatOptions struct {
	from          scale
	to            scale
	fromUnit      float64
	toUnit        float64
	format        string
	padding       int
	suffix        string
	unitSeparator string
	invalid       invalidMode
	round         roundMode
	grouping      bool
	debug         bool
}

type invalidMode int

const (
	invalidAbort invalidMode = iota
	invalidFail
	invalidWarn
	invalidIgnore
)

type roundMode int

const (
	roundUp roundMode = iota
	roundDown
	roundFromZero
	roundTowardsZero
	roundNearest
)

type scale int

const (
	scaleNone scale = iota
	scaleAuto
	scaleSI
	scaleIEC
	scaleIECI
)

func parseScale(rc *tool.RunContext, s, flag string) (scale, int) {
	switch strings.ToLower(s) {
	case "", "none":
		return scaleNone, -1
	case "auto":
		return scaleAuto, -1
	case "si":
		return scaleSI, -1
	case "iec":
		return scaleIEC, -1
	case "iec-i":
		return scaleIECI, -1
	default:
		return scaleNone, tool.NotSupported(rc, cmd, fmt.Sprintf("%s=%s (supported: none, auto, si, iec, iec-i)", flag, s))
	}
}

func parseUnitSize(rc *tool.RunContext, s, flag string) (float64, int) {
	n, err := strconv.ParseFloat(s, 64)
	if err != nil || n == 0 {
		return 0, tool.UsageError(rc, cmd, "invalid %s value: '%s'", flag, s)
	}
	return n, -1
}

func parseInvalidMode(rc *tool.RunContext, s string) (invalidMode, int) {
	switch strings.ToLower(s) {
	case "abort":
		return invalidAbort, -1
	case "fail":
		return invalidFail, -1
	case "warn":
		return invalidWarn, -1
	case "ignore":
		return invalidIgnore, -1
	default:
		return invalidAbort, tool.UsageError(rc, cmd, "invalid --invalid mode: '%s'", s)
	}
}

func parseRoundMode(rc *tool.RunContext, s string) (roundMode, int) {
	switch strings.ToLower(s) {
	case "up":
		return roundUp, -1
	case "down":
		return roundDown, -1
	case "from-zero":
		return roundFromZero, -1
	case "towards-zero":
		return roundTowardsZero, -1
	case "nearest":
		return roundNearest, -1
	default:
		return roundFromZero, tool.UsageError(rc, cmd, "invalid --round method: '%s'", s)
	}
}

func readRecords(r io.Reader, zero bool) ([]string, error) {
	if !zero {
		sc := bufio.NewScanner(r)
		var records []string
		for sc.Scan() {
			records = append(records, sc.Text())
		}
		return records, sc.Err()
	}
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}
	parts := strings.Split(string(data), "\x00")
	if len(parts) > 0 && parts[len(parts)-1] == "" {
		parts = parts[:len(parts)-1]
	}
	return parts, nil
}

type fieldRange struct {
	start int
	end   int
}

func parseFields(rc *tool.RunContext, spec string) ([]fieldRange, int) {
	if spec == "" {
		spec = "1"
	}
	if spec == "-" {
		return []fieldRange{{start: 1, end: 0}}, -1
	}
	var ranges []fieldRange
	for _, part := range strings.Split(spec, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			return nil, tool.UsageError(rc, cmd, "invalid field list: '%s'", spec)
		}
		if strings.Contains(part, "-") {
			bounds := strings.SplitN(part, "-", 2)
			start, end := 1, 0
			var err error
			if bounds[0] != "" {
				start, err = strconv.Atoi(bounds[0])
				if err != nil || start < 1 {
					return nil, tool.UsageError(rc, cmd, "invalid field list: '%s'", spec)
				}
			}
			if bounds[1] != "" {
				end, err = strconv.Atoi(bounds[1])
				if err != nil || end < 1 {
					return nil, tool.UsageError(rc, cmd, "invalid field list: '%s'", spec)
				}
			}
			if end != 0 && end < start {
				return nil, tool.UsageError(rc, cmd, "invalid field list: '%s'", spec)
			}
			ranges = append(ranges, fieldRange{start: start, end: end})
			continue
		}
		n, err := strconv.Atoi(part)
		if err != nil || n < 1 {
			return nil, tool.UsageError(rc, cmd, "invalid field list: '%s'", spec)
		}
		ranges = append(ranges, fieldRange{start: n, end: n})
	}
	return ranges, -1
}

func selectedField(n int, ranges []fieldRange) bool {
	for _, r := range ranges {
		if n >= r.start && (r.end == 0 || n <= r.end) {
			return true
		}
	}
	return false
}

func formatLine(rc *tool.RunContext, line, delimiter string, fields []fieldRange, opts formatOptions) (string, bool) {
	if delimiter == "" {
		parts := strings.Fields(line)
		ok := true
		for i, part := range parts {
			if !selectedField(i+1, fields) {
				continue
			}
			out, fieldOK := formatOne(rc, part, opts)
			if !fieldOK {
				ok = false
				if opts.invalid == invalidAbort {
					return line, false
				}
				if opts.invalid == invalidFail || opts.invalid == invalidWarn || opts.invalid == invalidIgnore {
					continue
				}
			}
			parts[i] = out
		}
		return strings.Join(parts, " "), ok
	}
	parts := strings.Split(line, delimiter)
	ok := true
	for i, part := range parts {
		if !selectedField(i+1, fields) {
			continue
		}
		out, fieldOK := formatOne(rc, part, opts)
		if !fieldOK {
			ok = false
			if opts.invalid == invalidAbort {
				return line, false
			}
			continue
		}
		parts[i] = out
	}
	return strings.Join(parts, delimiter), ok
}

func formatOne(rc *tool.RunContext, text string, opts formatOptions) (string, bool) {
	input := text
	if opts.suffix != "" && strings.HasSuffix(input, opts.suffix) {
		input = strings.TrimSuffix(input, opts.suffix)
	}
	n, err := parseNumber(input, opts.from)
	if err != nil {
		if opts.invalid != invalidIgnore {
			fmt.Fprintf(rc.Err, "numfmt: invalid number '%s'\n", text)
		}
		return "", false
	}
	n *= opts.fromUnit
	n /= opts.toUnit
	out := formatNumber(n, opts.to, opts.format, opts.unitSeparator, opts.round, opts.grouping)
	if opts.suffix != "" {
		out += opts.suffix
	}
	if opts.padding > 0 && len(out) < opts.padding {
		out = strings.Repeat(" ", opts.padding-len(out)) + out
	} else if opts.padding < 0 && len(out) < -opts.padding {
		out += strings.Repeat(" ", -opts.padding-len(out))
	}
	return out, true
}

func roundValue(n float64, mode roundMode, format string) float64 {
	scale := math.Pow10(formatPrecision(format))
	n *= scale
	switch mode {
	case roundUp:
		n = math.Ceil(n)
	case roundDown:
		n = math.Floor(n)
	case roundTowardsZero:
		n = math.Trunc(n)
	case roundNearest:
		n = math.Round(n)
	case roundFromZero:
		if n < 0 {
			n = math.Floor(n)
		} else {
			n = math.Ceil(n)
		}
	}
	return n / scale
}

func formatPrecision(format string) int {
	percent := strings.LastIndexByte(format, '%')
	if percent < 0 {
		return 6
	}
	dot := strings.LastIndexByte(format[percent:], '.')
	if dot < 0 {
		return 6
	}
	dot += percent
	end := dot + 1
	for end < len(format) && format[end] >= '0' && format[end] <= '9' {
		end++
	}
	if end == dot+1 {
		return 6
	}
	n, err := strconv.Atoi(format[dot+1 : end])
	if err != nil || n < 0 || n > 12 {
		return 6
	}
	return n
}

func parseNumber(s string, sc scale) (float64, error) {
	if sc == scaleNone {
		return strconv.ParseFloat(s, 64)
	}
	num, unit := splitNumberUnit(s)
	if num == "" {
		return 0, errors.New("missing number")
	}
	v, err := strconv.ParseFloat(num, 64)
	if err != nil {
		return 0, err
	}
	mul, ok := unitMultiplier(unit, sc)
	if !ok {
		return 0, fmt.Errorf("unsupported unit %q", unit)
	}
	return v * mul, nil
}

func splitNumberUnit(s string) (string, string) {
	i := 0
	for i < len(s) {
		c := s[i]
		if (c >= '0' && c <= '9') || c == '.' || c == '+' || c == '-' || c == 'e' || c == 'E' {
			i++
			continue
		}
		break
	}
	return s[:i], s[i:]
}

func unitBase(sc scale) float64 {
	if sc == scaleSI {
		return 1000
	}
	return 1024
}

func unitMultiplier(unit string, sc scale) (float64, bool) {
	if unit == "" {
		return 1, true
	}
	if sc == scaleAuto {
		if strings.HasSuffix(unit, "i") || strings.HasSuffix(unit, "iB") {
			return unitMultiplier(unit, scaleIECI)
		}
		return unitMultiplier(unit, scaleSI)
	}
	if strings.HasSuffix(unit, "B") {
		unit = strings.TrimSuffix(unit, "B")
	}
	if sc == scaleIECI {
		if !strings.HasSuffix(unit, "i") {
			return 0, false
		}
		unit = strings.TrimSuffix(unit, "i")
	} else if strings.HasSuffix(unit, "i") {
		if sc == scaleSI {
			return 0, false
		}
		unit = strings.TrimSuffix(unit, "i")
	}
	units := "KMGTPEZY"
	i := strings.IndexRune(units, rune(unit[0]))
	if i < 0 || len(unit) != 1 {
		return 0, false
	}
	return math.Pow(unitBase(sc), float64(i+1)), true
}

func formatNumber(n float64, sc scale, format, unitSeparator string, round roundMode, grouping bool) string {
	if sc == scaleNone {
		out := trimFloat(fmt.Sprintf(format, roundValue(n, round, format)))
		if grouping {
			out = groupNumber(out)
		}
		return out
	}
	base := unitBase(sc)
	units := []string{"", "K", "M", "G", "T", "P", "E", "Z", "Y"}
	pow := 0
	for math.Abs(n) >= base && pow < len(units)-1 {
		n /= base
		pow++
	}
	suffix := units[pow]
	if sc == scaleIECI && suffix != "" {
		suffix += "i"
	}
	out := trimFloat(fmt.Sprintf(format, roundValue(n, round, format)))
	if grouping {
		out = groupNumber(out)
	}
	if suffix != "" {
		out += unitSeparator + suffix
	}
	return out
}

func groupNumber(s string) string {
	if strings.ContainsAny(s, "eE") {
		return s
	}
	sign := ""
	if strings.HasPrefix(s, "-") || strings.HasPrefix(s, "+") {
		sign = s[:1]
		s = s[1:]
	}
	frac := ""
	if dot := strings.IndexByte(s, '.'); dot >= 0 {
		frac = s[dot:]
		s = s[:dot]
	}
	if len(s) <= 3 {
		return sign + s + frac
	}
	first := len(s) % 3
	if first == 0 {
		first = 3
	}
	var b strings.Builder
	b.WriteString(sign)
	b.WriteString(s[:first])
	for i := first; i < len(s); i += 3 {
		b.WriteByte(',')
		b.WriteString(s[i : i+3])
	}
	b.WriteString(frac)
	return b.String()
}

func trimFloat(s string) string {
	if strings.Contains(s, ".") {
		s = strings.TrimRight(s, "0")
		s = strings.TrimRight(s, ".")
	}
	if s == "-0" {
		return "0"
	}
	return s
}
