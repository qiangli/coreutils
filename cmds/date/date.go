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
	operands, code := tool.Parse(rc, cmd, fs, args)
	if code >= 0 {
		return code
	}

	loc := time.Local
	if *utc || *universal {
		loc = time.UTC
	}

	if *dstr != "" && *ref != "" {
		return tool.UsageError(rc, cmd, "the options to specify dates for printing are mutually exclusive")
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

	switch len(operands) {
	case 0:
		// GNU C-locale default: "%a %b %e %H:%M:%S %Z %Y".
		fmt.Fprintf(rc.Out, "%s\n", t.Format("Mon Jan _2 15:04:05 MST 2006"))
		return 0
	case 1:
		if !strings.HasPrefix(operands[0], "+") {
			return tool.NotSupported(rc, cmd, "setting the system date")
		}
		fmt.Fprintf(rc.Out, "%s\n", strftime(t, operands[0][1:]))
		return 0
	default:
		return tool.UsageError(rc, cmd, "extra operand %q", operands[1])
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
		case 'w':
			// Weekday, Sunday=0.
			fmt.Fprintf(&b, "%d", int(t.Weekday()))
		case 'z':
			b.WriteString(t.Format("-0700"))
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
