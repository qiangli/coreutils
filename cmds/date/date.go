// Package datecmd implements date(1) per the GNU coreutils manual
// (C locale): print the current (or specified) time in the default
// format or per a +FORMAT operand built from strftime directives.
//
// Supported directives: %Y %m %d %H %M %S %y %j %a %A %b %h %B %e %T
// %D %F %R %s %N %z %Z %p %I %u %w %n %t %%. Unknown %X sequences pass
// through literally, as GNU date does.
//
// -d STRING parses a documented subset (RFC 3339, @EPOCH,
// "YYYY-MM-DD [HH:MM[:SS]]"); anything else is a clear error. Setting
// the system date is documented-but-unsupported.
//
// Portions adapted from https://github.com/u-root/u-root cmds/core/date/date.go (BSD-3-Clause).
// Changes: rewired to the tool framework; strftime rewritten as a
// single-pass interpreter (the prior art's string-replace loop
// re-expands directives produced by earlier substitutions); GNU C
// locale default output; -d STRING subset parser; set-date mode
// refused per repo rules.
package datecmd

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/qiangli/coreutils/tool"
)

var cmd = &tool.Tool{
	Name:     "date",
	Synopsis: "Display date and time in the given FORMAT (C locale).",
	Usage:    "date [OPTION]... [+FORMAT]",
}

func init() { cmd.Run = run; tool.Register(cmd) }

func run(rc *tool.RunContext, args []string) int {
	fs := tool.NewFlags(cmd.Name)
	utc := fs.BoolP("utc", "u", false, "print in Coordinated Universal Time (UTC)")
	universal := fs.Bool("universal", false, "same as --utc")
	dstr := fs.StringP("date", "d", "", "display time described by STRING, not 'now'")
	ref := fs.StringP("reference", "r", "", "display the last modification time of FILE")
	dateFile := fs.StringP("file", "f", "", "like --date once for each line of FILE")
	debug := fs.Bool("debug", false, "annotate parsed dates on stderr")
	iso8601 := fs.StringP("iso-8601", "I", "", "output date/time in ISO 8601 format")
	fs.Lookup("iso-8601").NoOptDefVal = "date"
	rfc3339 := fs.String("rfc-3339", "", "output date/time in RFC 3339 format")
	rfcEmail := fs.BoolP("rfc-email", "R", false, "output date and time in RFC 5322 email format")
	rfc822 := fs.Bool("rfc-822", false, "output date and time in RFC 5322 email format")
	fs.Lookup("rfc-822").Hidden = true
	rfc2822 := fs.Bool("rfc-2822", false, "output date and time in RFC 5322 email format")
	fs.Lookup("rfc-2822").Hidden = true
	uct := fs.Bool("uct", false, "print in Coordinated Universal Time (UTC)")
	fs.Lookup("uct").Hidden = true
	resolution := fs.Bool("resolution", false, "output the available timestamp resolution")
	setDate := fs.StringP("set", "s", "", "set time described by STRING")
	operands, code := tool.Parse(rc, cmd, fs, args)
	if code >= 0 {
		return code
	}
	if *setDate != "" {
		return tool.NotSupported(rc, cmd, "setting the system date")
	}

	loc := time.Local
	if *utc || *universal || *uct {
		loc = time.UTC
	}

	sources := 0
	for _, set := range []bool{*dstr != "", *ref != "", *dateFile != ""} {
		if set {
			sources++
		}
	}
	if sources > 1 {
		return tool.UsageError(rc, cmd, "the options to specify dates for printing are mutually exclusive")
	}
	if *resolution {
		if sources > 0 || len(operands) > 0 {
			return tool.UsageError(rc, cmd, "--resolution is mutually exclusive with date formatting")
		}
		fmt.Fprintln(rc.Out, "0.000000001")
		return 0
	}
	format, code := selectFormat(rc, operands, *iso8601, fs.Changed("iso-8601"), *rfc3339, *rfcEmail || *rfc822 || *rfc2822)
	if code >= 0 {
		return code
	}
	if *dateFile != "" {
		data, err := os.ReadFile(rc.Path(*dateFile))
		if err != nil {
			fmt.Fprintf(rc.Err, "date: %s: %v\n", *dateFile, tool.SysErr(err))
			return 1
		}
		status := 0
		for _, line := range strings.Split(strings.TrimRight(string(data), "\n"), "\n") {
			t, err := parseDateString(line, loc)
			if err != nil {
				fmt.Fprintf(rc.Err, "date: invalid date %q (supported: RFC 3339, @EPOCH, \"YYYY-MM-DD [HH:MM[:SS]]\")\n", line)
				status = 1
				continue
			}
			if *debug {
				fmt.Fprintf(rc.Err, "date: parsed date %q -> %s\n", line, t.In(loc).Format(time.RFC3339Nano))
			}
			fmt.Fprintf(rc.Out, "%s\n", strftime(t.In(loc), format))
		}
		return status
	}

	t := time.Now()
	switch {
	case *ref != "":
		fi, err := os.Stat(rc.Path(*ref))
		if err != nil {
			fmt.Fprintf(rc.Err, "date: %s: %v\n", *ref, err)
			return 1
		}
		t = fi.ModTime()
	case *dstr != "":
		var err error
		t, err = parseDateString(*dstr, loc)
		if err != nil {
			fmt.Fprintf(rc.Err, "date: invalid date %q (supported: RFC 3339, @EPOCH, \"YYYY-MM-DD [HH:MM[:SS]]\")\n", *dstr)
			return 1
		}
	}
	t = t.In(loc)
	if *debug && *dstr != "" {
		fmt.Fprintf(rc.Err, "date: parsed date %q -> %s\n", *dstr, t.Format(time.RFC3339Nano))
	}

	fmt.Fprintf(rc.Out, "%s\n", strftime(t, format))
	return 0
}

func selectFormat(rc *tool.RunContext, operands []string, iso string, isoSet bool, rfc3339 string, rfcEmail bool) (string, int) {
	format := "%a %b %e %H:%M:%S %Z %Y"
	formatCount := 0
	if len(operands) > 0 {
		if len(operands) > 1 {
			return "", tool.UsageError(rc, cmd, "extra operand %q", operands[1])
		}
		if !strings.HasPrefix(operands[0], "+") {
			return "", tool.NotSupported(rc, cmd, "setting the system date")
		}
		format = operands[0][1:]
		formatCount++
	}
	if isoSet {
		f, code := isoFormat(rc, iso)
		if code >= 0 {
			return "", code
		}
		format = f
		formatCount++
	}
	if rfc3339 != "" {
		f, code := rfc3339Format(rc, rfc3339)
		if code >= 0 {
			return "", code
		}
		format = f
		formatCount++
	}
	if rfcEmail {
		format = "%a, %d %b %Y %H:%M:%S %z"
		formatCount++
	}
	if formatCount > 1 {
		return "", tool.UsageError(rc, cmd, "multiple output formats specified")
	}
	return format, -1
}

func isoFormat(rc *tool.RunContext, spec string) (string, int) {
	switch spec {
	case "", "date":
		return "%Y-%m-%d", -1
	case "hours":
		return "%Y-%m-%dT%H%z", -1
	case "minutes":
		return "%Y-%m-%dT%H:%M%z", -1
	case "seconds":
		return "%Y-%m-%dT%H:%M:%S%z", -1
	case "ns", "nanoseconds":
		return "%Y-%m-%dT%H:%M:%S,%N%z", -1
	default:
		return "", tool.UsageError(rc, cmd, "invalid --iso-8601 timespec: %q", spec)
	}
}

func rfc3339Format(rc *tool.RunContext, spec string) (string, int) {
	switch spec {
	case "date":
		return "%Y-%m-%d", -1
	case "seconds":
		return "%Y-%m-%d %H:%M:%S%:z", -1
	case "ns", "nanoseconds":
		return "%Y-%m-%d %H:%M:%S.%N%:z", -1
	default:
		return "", tool.UsageError(rc, cmd, "invalid --rfc-3339 timespec: %q", spec)
	}
}

// parseDateString accepts the documented -d subset: @EPOCH (integer or
// fractional seconds), RFC 3339, and zone-less calendar forms
// interpreted in loc.
func parseDateString(s string, loc *time.Location) (time.Time, error) {
	s = strings.TrimSpace(s)
	if rest, ok := strings.CutPrefix(s, "@"); ok {
		secs, frac, _ := strings.Cut(rest, ".")
		sec, err := strconv.ParseInt(secs, 10, 64)
		if err != nil {
			return time.Time{}, err
		}
		var nsec int64
		if frac != "" {
			f, err := strconv.ParseFloat("0."+frac, 64)
			if err != nil {
				return time.Time{}, err
			}
			nsec = int64(f * 1e9)
			if sec < 0 {
				nsec = -nsec
			}
		}
		return time.Unix(sec, nsec), nil
	}
	// Zoned forms first (the string's own zone wins over loc).
	for _, layout := range []string{time.RFC3339Nano, "2006-01-02T15:04:05Z07:00"} {
		if t, err := time.Parse(layout, s); err == nil {
			return t, nil
		}
	}
	// Zone-less calendar forms, interpreted in loc.
	for _, layout := range []string{
		"2006-01-02T15:04:05.999999999",
		"2006-01-02T15:04:05",
		"2006-01-02 15:04:05.999999999",
		"2006-01-02 15:04:05",
		"2006-01-02 15:04",
		"2006-01-02",
	} {
		if t, err := time.ParseInLocation(layout, s, loc); err == nil {
			return t, nil
		}
	}
	return time.Time{}, fmt.Errorf("unparsed date %q", s)
}

// strftime renders the supported GNU/strftime directive subset in the
// C locale. Unknown %X sequences pass through literally.
func strftime(t time.Time, f string) string {
	var b strings.Builder
	for i := 0; i < len(f); i++ {
		c := f[i]
		if c != '%' || i == len(f)-1 {
			b.WriteByte(c)
			continue
		}
		i++
		switch f[i] {
		case 'Y':
			fmt.Fprintf(&b, "%d", t.Year())
		case 'y':
			fmt.Fprintf(&b, "%02d", t.Year()%100)
		case 'm':
			fmt.Fprintf(&b, "%02d", int(t.Month()))
		case 'd':
			fmt.Fprintf(&b, "%02d", t.Day())
		case 'e':
			fmt.Fprintf(&b, "%2d", t.Day())
		case 'j':
			fmt.Fprintf(&b, "%03d", t.YearDay())
		case 'H':
			fmt.Fprintf(&b, "%02d", t.Hour())
		case 'I':
			h := t.Hour() % 12
			if h == 0 {
				h = 12
			}
			fmt.Fprintf(&b, "%02d", h)
		case 'M':
			fmt.Fprintf(&b, "%02d", t.Minute())
		case 'S':
			fmt.Fprintf(&b, "%02d", t.Second())
		case 'N':
			fmt.Fprintf(&b, "%09d", t.Nanosecond())
		case 's':
			fmt.Fprintf(&b, "%d", t.Unix())
		case 'a':
			b.WriteString(t.Format("Mon"))
		case 'A':
			b.WriteString(t.Format("Monday"))
		case 'b', 'h':
			b.WriteString(t.Format("Jan"))
		case 'B':
			b.WriteString(t.Format("January"))
		case 'c':
			b.WriteString(t.Format("Mon Jan _2 15:04:05 2006"))
		case 'C':
			fmt.Fprintf(&b, "%02d", t.Year()/100)
		case 'g':
			y, _ := t.ISOWeek()
			fmt.Fprintf(&b, "%02d", y%100)
		case 'G':
			y, _ := t.ISOWeek()
			fmt.Fprintf(&b, "%04d", y)
		case 'r':
			b.WriteString(t.Format("03:04:05 PM"))
		case 'p':
			if t.Hour() < 12 {
				b.WriteString("AM")
			} else {
				b.WriteString("PM")
			}
		case 'T':
			fmt.Fprintf(&b, "%02d:%02d:%02d", t.Hour(), t.Minute(), t.Second())
		case 'R':
			fmt.Fprintf(&b, "%02d:%02d", t.Hour(), t.Minute())
		case 'D':
			fmt.Fprintf(&b, "%02d/%02d/%02d", int(t.Month()), t.Day(), t.Year()%100)
		case 'F':
			fmt.Fprintf(&b, "%d-%02d-%02d", t.Year(), int(t.Month()), t.Day())
		case 'u':
			// ISO weekday, Monday=1 .. Sunday=7.
			wd := int(t.Weekday())
			if wd == 0 {
				wd = 7
			}
			fmt.Fprintf(&b, "%d", wd)
		case 'U':
			fmt.Fprintf(&b, "%02d", weekNumber(t, time.Sunday))
		case 'V':
			_, w := t.ISOWeek()
			fmt.Fprintf(&b, "%02d", w)
		case 'w':
			// Weekday, Sunday=0.
			fmt.Fprintf(&b, "%d", int(t.Weekday()))
		case 'W':
			fmt.Fprintf(&b, "%02d", weekNumber(t, time.Monday))
		case 'x':
			b.WriteString(t.Format("01/02/06"))
		case 'X':
			b.WriteString(t.Format("15:04:05"))
		case 'z':
			b.WriteString(t.Format("-0700"))
		case ':':
			if i+1 < len(f) && f[i+1] == 'z' {
				i++
				b.WriteString(t.Format("-07:00"))
			} else {
				b.WriteByte('%')
				b.WriteByte(':')
			}
		case 'Z':
			b.WriteString(t.Format("MST"))
		case 'n':
			b.WriteByte('\n')
		case 't':
			b.WriteByte('\t')
		case '%':
			b.WriteByte('%')
		default:
			b.WriteByte('%')
			b.WriteByte(f[i])
		}
	}
	return b.String()
}

// weekNumber computes the week number (00-53) for the given time.
// firstDay is time.Sunday for %U, time.Monday for %W.
func weekNumber(t time.Time, firstDay time.Weekday) int {
	yd := t.YearDay() - 1
	jan1 := time.Date(t.Year(), 1, 1, 0, 0, 0, 0, t.Location())
	jan1_wd := jan1.Weekday()

	// Days until the first target day of the week
	offset := (int(firstDay) - int(jan1_wd) + 7) % 7

	if yd < offset {
		return 0
	}
	return 1 + (yd-offset)/7
}
