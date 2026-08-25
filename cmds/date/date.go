// Package datecmd implements date(1) per POSIX Issue 7 plus documented GNU
// extensions: print the current (or specified) time in the default
// format or per a +FORMAT operand built from strftime directives.
//
// Supported directives: %Y %m %d %H %M %S %y %j %a %A %b %h %B %e %T
// %D %F %R %s %N %z %Z %p %I %u %w %n %t %%. The POSIX alternative
// modifiers (%Ec %EC %Ex %EX %Ey %EY and %Od %Oe %OH %OI %Om %OM %OS
// %Ou %OU %OV %Ow %OW %Oy) render as the unmodified conversion, as
// POSIX requires in the C/POSIX locale. Unknown %X sequences pass
// through literally, as GNU date does.
//
// The TZ environment variable (from the RunContext environment, per
// the tool contract) selects the output zone unless -u is given —
// both IANA names and POSIX "std offset dst[,rule,rule]" expansions
// are honored; see cmds/internal/tzenv.
//
// -d STRING parses a documented subset (RFC 3339, @EPOCH,
// "YYYY-MM-DD [HH:MM[:SS]]"); anything else is a clear error. Setting
// the system date through the XSI mmddhhmm[[cc]yy] operand is supported
// through a platform clock setter; the GNU -s extension remains separate.
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

	"github.com/qiangli/coreutils/cmds/internal/tzenv"
	sharedlocale "github.com/qiangli/coreutils/pkg/locale"
	"github.com/qiangli/coreutils/tool"
)

var cmd = &tool.Tool{
	Name:     "date",
	Synopsis: "Display or set date and time using the selected locale.",
	Usage:    "date [-u] [+FORMAT]\n       date [-u] mmddhhmm[[cc]yy]",
}

func init() { cmd.Run = run; tool.Register(cmd) }

func run(rc *tool.RunContext, args []string) int {
	return runWithClock(rc, args, time.Now, setSystemClock)
}

type clockSetter func(time.Time) error

func runWithClock(rc *tool.RunContext, args []string, now func() time.Time, setClock clockSetter) int {
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

	loc := dateLocation(rc.Env)
	names, err := selectDateLocale(rc.Env)
	if err != nil {
		fmt.Fprintf(rc.Err, "date: %v\n", err)
		return 1
	}
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
		return writeOutput(rc, "0.000000001\n")
	}
	if len(operands) > 0 && !strings.HasPrefix(operands[0], "+") {
		if len(operands) > 1 {
			return tool.UsageError(rc, cmd, "extra operand %q", operands[1])
		}
		formatOption := fs.Changed("iso-8601") || *rfc3339 != "" || *rfcEmail || *rfc822 || *rfc2822
		if sources != 0 || formatOption || *debug {
			return tool.UsageError(rc, cmd, "the XSI set-date operand cannot be combined with a date source or output-format option")
		}
		target, err := parseXSISetDate(operands[0], now().In(loc), loc)
		if err != nil {
			return tool.UsageError(rc, cmd, "invalid XSI set-date operand %q: %v", operands[0], err)
		}
		if err := setClock(target); err != nil {
			fmt.Fprintf(rc.Err, "date: cannot set date: %v\n", err)
			return 1
		}
		return writeOutput(rc, strftime(target.In(loc), "%a %b %e %H:%M:%S %Z %Y", names)+"\n")
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
			if writeOutput(rc, strftime(t.In(loc), format, names)+"\n") != 0 {
				return 1
			}
		}
		return status
	}

	t := now()
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

	return writeOutput(rc, strftime(t, format, names)+"\n")
}

// writeOutput makes a failed standard-output write observable. POSIX general
// assertion 39 requires a diagnostic and non-zero status for write failures;
// ignoring fmt's error made a closed pipe look like a successful date run.
func writeOutput(rc *tool.RunContext, s string) int {
	if _, err := fmt.Fprint(rc.Out, s); err != nil {
		fmt.Fprintf(rc.Err, "date: write error: %v\n", err)
		return 1
	}
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
			return "", tool.UsageError(rc, cmd, "invalid date operand %q", operands[0])
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

// isSetDateOperand reports whether s has the XSI set-date operand grammar
// mmddhhmm[[cc]yy]: the eight mandatory digits mmddhhmm, optionally followed
// by a two-digit yy or a four-digit ccyy. It is a shape check for diagnostics,
// not a validity check on the individual fields.
func isSetDateOperand(s string) bool {
	switch len(s) {
	case 8, 10, 12: // mmddhhmm, mmddhhmm+yy, mmddhhmm+ccyy
	default:
		return false
	}
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
}

// parseXSISetDate parses mmddhhmm[[cc]yy]. When the year is omitted it uses
// the current year in the selected timezone. A two-digit year follows the
// Issue 7 mapping: 69..99 => 1969..1999 and 00..68 => 2000..2068.
func parseXSISetDate(s string, now time.Time, loc *time.Location) (time.Time, error) {
	if !isSetDateOperand(s) {
		return time.Time{}, fmt.Errorf("expected mmddhhmm, mmddhhmmyy, or mmddhhmmccyy")
	}
	field := func(start int) int {
		n, _ := strconv.Atoi(s[start : start+2])
		return n
	}
	month, day, hour, minute := field(0), field(2), field(4), field(6)
	year := now.Year()
	switch len(s) {
	case 10:
		yy := field(8)
		if yy >= 69 {
			year = 1900 + yy
		} else {
			year = 2000 + yy
		}
	case 12:
		cc, yy := field(8), field(10)
		year = cc*100 + yy
	}
	if month < 1 || month > 12 || day < 1 || day > 31 || hour > 23 || minute > 59 {
		return time.Time{}, fmt.Errorf("field outside its permitted range")
	}
	t := time.Date(year, time.Month(month), day, hour, minute, 0, 0, loc)
	if t.Year() != year || int(t.Month()) != month || t.Day() != day || t.Hour() != hour || t.Minute() != minute {
		return time.Time{}, fmt.Errorf("date does not exist in the selected timezone")
	}
	return t, nil
}

// dateLocation applies date's stricter Issue 7 TZ rule: an unset or null TZ
// selects the system default. Other values use the shared pure-Go POSIX/IANA
// resolver. The generic resolver intentionally gives null TZ UTC semantics for
// glibc-shaped callers, so date must preserve this utility-specific rule here.
func dateLocation(env []string) *time.Location {
	for i := len(env) - 1; i >= 0; i-- {
		if value, ok := strings.CutPrefix(env[i], "TZ="); ok {
			if value == "" {
				return time.Local
			}
			return tzenv.FromValue(value)
		}
	}
	return time.Local
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

// Valid conversions for the POSIX %E and %O alternative modifiers
// (XBD strftime). In the C/POSIX locale there are no alternative era
// or digit representations, so a valid modified conversion renders
// exactly as the unmodified one; an invalid combination is left to
// pass through literally like any other unknown sequence.
const (
	eModified = "cCxXyY"
	oModified = "deHImMSuUVwWy"
)

type dateLocale struct {
	weekdays      [7]string
	weekdaysShort [7]string
	months        [12]string
	monthsShort   [12]string
	latin1        bool
	german        bool
}

var germanDateLocale = dateLocale{
	weekdays:      [7]string{"Sonntag", "Montag", "Dienstag", "Mittwoch", "Donnerstag", "Freitag", "Samstag"},
	weekdaysShort: [7]string{"So", "Mo", "Di", "Mi", "Do", "Fr", "Sa"},
	months:        [12]string{"Januar", "Februar", "März", "April", "Mai", "Juni", "Juli", "August", "September", "Oktober", "November", "Dezember"},
	monthsShort:   [12]string{"Jan", "Feb", "Mär", "Apr", "Mai", "Jun", "Jul", "Aug", "Sep", "Okt", "Nov", "Dez"},
	german:        true,
}

// selectDateLocale applies POSIX locale precedence for the LC_TIME category:
// a non-empty LC_ALL, then LC_TIME, then LANG. C/POSIX and an absent locale
// use Go's C-locale names; de_DE is embedded so standalone binaries do not
// depend on host locale archives, which are routinely absent in containers.
func selectDateLocale(env []string) (dateLocale, error) {
	name := sharedlocale.Resolve(env, sharedlocale.Time)
	name, _, _ = strings.Cut(name, "@")
	base, codeset, _ := strings.Cut(name, ".")
	normalized := strings.ToUpper(strings.NewReplacer("-", "", "_", "").Replace(codeset))
	if base == "C" || base == "POSIX" {
		if normalized == "" || normalized == "UTF8" {
			return dateLocale{}, nil
		}
		return dateLocale{}, fmt.Errorf("LC_TIME locale %q is unavailable", name)
	}
	if base != "de_DE" {
		return dateLocale{}, fmt.Errorf("LC_TIME locale %q is unavailable: embedded date data is limited to C/POSIX and de_DE", name)
	}
	if normalized != "" && normalized != "UTF8" && normalized != "ISO88591" {
		return dateLocale{}, fmt.Errorf("LC_TIME locale %q is unavailable", name)
	}
	loc := germanDateLocale
	// The unqualified certification locale is the ISO-8859-1 corpus.
	loc.latin1 = normalized == "" || normalized == "ISO88591"
	return loc, nil
}

func localeText(loc dateLocale, s string) string {
	if !loc.latin1 {
		return s
	}
	var b strings.Builder
	for _, r := range s {
		if r <= 0xff {
			b.WriteByte(byte(r))
		} else {
			b.WriteByte('?')
		}
	}
	return b.String()
}

// strftime renders the supported GNU/strftime directive subset. Unknown %X
// sequences pass through literally.
func strftime(t time.Time, f string, loc dateLocale) string {
	var b strings.Builder
	for i := 0; i < len(f); i++ {
		c := f[i]
		if c != '%' || i == len(f)-1 {
			b.WriteByte(c)
			continue
		}
		i++
		if i+1 < len(f) &&
			((f[i] == 'E' && strings.IndexByte(eModified, f[i+1]) >= 0) ||
				(f[i] == 'O' && strings.IndexByte(oModified, f[i+1]) >= 0)) {
			i++ // C/POSIX locale: fall back to the unmodified conversion
		}
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
			if loc.weekdaysShort[0] == "" {
				b.WriteString(t.Format("Mon"))
			} else {
				b.WriteString(localeText(loc, loc.weekdaysShort[t.Weekday()]))
			}
		case 'A':
			if loc.weekdays[0] == "" {
				b.WriteString(t.Format("Monday"))
			} else {
				b.WriteString(localeText(loc, loc.weekdays[t.Weekday()]))
			}
		case 'b', 'h':
			if loc.monthsShort[0] == "" {
				b.WriteString(t.Format("Jan"))
			} else {
				b.WriteString(localeText(loc, loc.monthsShort[int(t.Month())-1]))
			}
		case 'B':
			if loc.months[0] == "" {
				b.WriteString(t.Format("January"))
			} else {
				b.WriteString(localeText(loc, loc.months[int(t.Month())-1]))
			}
		case 'c':
			if !loc.german {
				b.WriteString(t.Format("Mon Jan _2 15:04:05 2006"))
			} else {
				// glibc de_DE d_t_fmt: "%a %d %b %Y %T %Z".
				fmt.Fprintf(&b, "%s %02d %s %d %02d:%02d:%02d %s",
					localeText(loc, loc.weekdaysShort[t.Weekday()]),
					t.Day(), localeText(loc, loc.monthsShort[int(t.Month())-1]),
					t.Year(), t.Hour(), t.Minute(), t.Second(), t.Format("MST"))
			}
		case 'C':
			fmt.Fprintf(&b, "%02d", t.Year()/100)
		case 'g':
			y, _ := t.ISOWeek()
			fmt.Fprintf(&b, "%02d", y%100)
		case 'G':
			y, _ := t.ISOWeek()
			fmt.Fprintf(&b, "%04d", y)
		case 'r':
			if loc.german {
				// de_DE has an empty am_pm value; glibc retains its separator.
				b.WriteString(t.Format("03:04:05 "))
			} else {
				b.WriteString(t.Format("03:04:05 PM"))
			}
		case 'p':
			if loc.german {
				break
			} else if t.Hour() < 12 {
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
			if loc.german {
				b.WriteString(t.Format("02.01.2006"))
			} else {
				b.WriteString(t.Format("01/02/06"))
			}
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
