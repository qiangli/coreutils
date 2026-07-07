// Package numfmtcmd implements a compact numfmt(1) subset for common
// SI/IEC scaling conversions.
package numfmtcmd

import (
	"bufio"
	"errors"
	"fmt"
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
	from := fs.String("from", "none", "auto-scale input numbers from UNIT system")
	to := fs.String("to", "none", "auto-scale output numbers to UNIT system")
	format := fs.String("format", "%f", "use printf-style floating format")
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

	if len(operands) > 0 {
		for _, op := range operands {
			out, ok := formatOne(rc, op, fromScale, toScale, *format)
			if !ok {
				return 1
			}
			fmt.Fprintln(rc.Out, out)
		}
		return 0
	}
	sc := bufio.NewScanner(rc.In)
	for sc.Scan() {
		fields := strings.Fields(sc.Text())
		for i, field := range fields {
			if i > 0 {
				fmt.Fprint(rc.Out, " ")
			}
			out, ok := formatOne(rc, field, fromScale, toScale, *format)
			if !ok {
				return 1
			}
			fmt.Fprint(rc.Out, out)
		}
		fmt.Fprintln(rc.Out)
	}
	if err := sc.Err(); err != nil {
		fmt.Fprintf(rc.Err, "numfmt: read error: %v\n", tool.SysErr(err))
		return 1
	}
	return 0
}

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

func formatOne(rc *tool.RunContext, text string, from, to scale, format string) (string, bool) {
	n, err := parseNumber(text, from)
	if err != nil {
		fmt.Fprintf(rc.Err, "numfmt: invalid number '%s'\n", text)
		return "", false
	}
	out := formatNumber(n, to, format)
	return out, true
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

func formatNumber(n float64, sc scale, format string) string {
	if sc == scaleNone {
		return trimFloat(fmt.Sprintf(format, n))
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
	return trimFloat(fmt.Sprintf(format, n)) + suffix
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
