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
	grouping := fs.Bool("grouping", false, "group digits per locale rules (no effect in the C locale)")
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
	fields, code := parseFields(rc, *fieldSpec)
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
	cleanFormat, precision, width, leftAlign, code := validateFormat(rc, *format)
	if code >= 0 {
		return code
	}
	opts := formatOptions{
		from:          fromScale,
		to:            toScale,
		fromUnit:      fromUnitValue,
		toUnit:        toUnitValue,
		format:        cleanFormat,
		precision:     precision,
		width:         width,
		leftAlign:     leftAlign,
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
	precision     int // explicit .N from --format, -1 if none
	width         int // field width from --format, 0 if none
	leftAlign     bool
	padding       int
	suffix        string
	unitSeparator string
	invalid       invalidMode
	round         roundMode
	grouping      bool
	debug         bool
}

// validateFormat checks --format per GNU: exactly one directive of the
// form %[0]['][-][N][.N]f, with optional surrounding text. It returns
// the format with any ' flag removed (--grouping and the ' flag have no
// effect in the C locale), the explicit precision (-1 if none), and the
// field width.
func validateFormat(rc *tool.RunContext, format string) (clean string, precision, width int, leftAlign bool, code int) {
	var b strings.Builder
	found := false
	precision = -1
	i := 0
	for i < len(format) {
		c := format[i]
		if c != '%' {
			b.WriteByte(c)
			i++
			continue
		}
		if i+1 < len(format) && format[i+1] == '%' {
			b.WriteString("%%")
			i += 2
			continue
		}
		if found {
			return "", 0, 0, false, tool.UsageError(rc, cmd, "format '%s' has too many %% directives", format)
		}
		found = true
		b.WriteByte('%')
		i++
		for i < len(format) && (format[i] == '0' || format[i] == '-' || format[i] == '\'') {
			if format[i] == '-' {
				leftAlign = true
			}
			if format[i] != '\'' {
				b.WriteByte(format[i])
			}
			i++
		}
		for i < len(format) && format[i] >= '0' && format[i] <= '9' {
			width = width*10 + int(format[i]-'0')
			b.WriteByte(format[i])
			i++
		}
		if i < len(format) && format[i] == '.' {
			b.WriteByte('.')
			i++
			precision = 0
			for i < len(format) && format[i] >= '0' && format[i] <= '9' {
				precision = precision*10 + int(format[i]-'0')
				b.WriteByte(format[i])
				i++
			}
		}
		if i >= len(format) || format[i] != 'f' {
			return "", 0, 0, false, tool.UsageError(rc, cmd, "invalid format '%s', directive must be %%[0]['][-][N][.][N]f", format)
		}
		b.WriteByte('f')
		i++
	}
	if !found {
		return "", 0, 0, false, tool.UsageError(rc, cmd, "format '%s' has no %% directive", format)
	}
	return b.String(), precision, width, leftAlign, -1
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

// wsToken is one field with its preceding whitespace run; a trailing
// whitespace-only token has an empty field.
type wsToken struct {
	prefix string
	field  string
}

func tokenizeWhitespace(line string) []wsToken {
	var tokens []wsToken
	i := 0
	for i < len(line) {
		start := i
		for i < len(line) && (line[i] == ' ' || line[i] == '\t') {
			i++
		}
		prefix := line[start:i]
		start = i
		for i < len(line) && line[i] != ' ' && line[i] != '\t' {
			i++
		}
		tokens = append(tokens, wsToken{prefix: prefix, field: line[start:i]})
	}
	return tokens
}

func formatLine(rc *tool.RunContext, line, delimiter string, fields []fieldRange, opts formatOptions) (string, bool) {
	if delimiter == "" {
		// GNU preserves the shape of whitespace-delimited input: a field
		// with leading whitespace is implicitly padded to its original
		// width (prefix + field), right-aligned.
		var b strings.Builder
		ok := true
		for i, tk := range tokenizeWhitespace(line) {
			n := i + 1
			if tk.field == "" || !selectedField(n, fields) {
				b.WriteString(tk.prefix)
				b.WriteString(tk.field)
				continue
			}
			out, fieldOK := formatOne(rc, tk.field, opts)
			if !fieldOK {
				ok = false
				if opts.invalid == invalidAbort {
					return line, false
				}
				b.WriteString(tk.prefix)
				b.WriteString(tk.field)
				continue
			}
			prefix := tk.prefix
			sep := ""
			if n > 1 && len(prefix) > 0 {
				sep = " "
				prefix = prefix[1:]
			}
			if len(tk.prefix) > 0 && opts.padding == 0 {
				if w := len(prefix) + len(tk.field); len(out) < w {
					out = strings.Repeat(" ", w-len(out)) + out
				}
			}
			b.WriteString(sep)
			b.WriteString(out)
		}
		return b.String(), ok
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
	out := formatNumber(n, opts)
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

func roundValue(n float64, mode roundMode, precision int) float64 {
	scale := math.Pow10(precision)
	return roundAt(n*scale, mode) / scale
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
	if len(unit) != 1 {
		return 0, false
	}
	units := "KMGTPEZY"
	i := strings.IndexRune(units, rune(unit[0]))
	if i < 0 {
		return 0, false
	}
	return math.Pow(unitBase(sc), float64(i+1)), true
}

// humanRound implements GNU's scaled rounding: if the scaled value is
// less than 10, round to one decimal place; otherwise round to an
// integer (rounding per the selected --round method).
func humanRound(v float64, mode roundMode) float64 {
	if math.Abs(v) < 10 {
		return roundAt(v*10, mode) / 10
	}
	return roundAt(v, mode)
}

func roundAt(n float64, mode roundMode) float64 {
	switch mode {
	case roundUp:
		return math.Ceil(n)
	case roundDown:
		return math.Floor(n)
	case roundTowardsZero:
		return math.Trunc(n)
	case roundNearest:
		return math.Round(n)
	default: // roundFromZero
		if n < 0 {
			return math.Floor(n)
		}
		return math.Ceil(n)
	}
}

func formatNumber(n float64, opts formatOptions) string {
	sc := opts.to
	if sc == scaleNone {
		if opts.precision >= 0 {
			return fmt.Sprintf(opts.format, roundValue(n, opts.round, opts.precision))
		}
		return applyWidth(trimFloat(fmt.Sprintf("%f", roundValue(n, opts.round, 6))), opts)
	}
	base := unitBase(sc)
	units := []string{"", "K", "M", "G", "T", "P", "E", "Z", "Y"}
	pow := 0
	var v float64
	for {
		v = n / math.Pow(base, float64(pow))
		if opts.precision >= 0 {
			v = roundValue(v, opts.round, opts.precision)
		} else {
			v = humanRound(v, opts.round)
		}
		// Rounding can carry into the next unit (999999 -> 1000K -> 1.0M).
		if math.Abs(v) >= base && pow < len(units)-1 {
			pow++
			continue
		}
		break
	}
	suffix := units[pow]
	if sc == scaleIECI && suffix != "" {
		suffix += "i"
	}
	var out string
	if opts.precision >= 0 {
		out = fmt.Sprintf(opts.format, v)
	} else if pow > 0 && math.Abs(v) < 10 {
		// GNU prints one decimal for scaled values below 10 (1.0K).
		out = applyWidth(strconv.FormatFloat(v, 'f', 1, 64), opts)
	} else {
		out = applyWidth(strconv.FormatFloat(v, 'f', -1, 64), opts)
	}
	if suffix != "" {
		out += opts.unitSeparator + suffix
	}
	return out
}

// applyWidth honors a width given in --format (e.g. %10f) on the
// human-formatted number when no explicit precision was given.
func applyWidth(s string, opts formatOptions) string {
	if opts.width <= len(s) {
		return s
	}
	pad := strings.Repeat(" ", opts.width-len(s))
	if opts.leftAlign {
		return s + pad
	}
	return pad + s
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
