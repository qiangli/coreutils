package locale

import (
	"fmt"
	"strings"
	"time"
)

// TimeFormatter is the bounded, invocation-local LC_TIME provider used by
// commands that need abbreviated month names. It deliberately carries only
// locale data that is embedded in this repository: the normative C/POSIX
// locale and the de_DE data used by the certification fixture. Callers must
// fail closed when ResolveTime cannot provide the requested locale.
type TimeFormatter struct {
	months                                 [12]string
	weekdays                               [7]string
	dateTime, date, clock, clock12, am, pm string
	latin1                                 bool
	locale                                 string
}

var (
	posixMonths   = [12]string{"Jan", "Feb", "Mar", "Apr", "May", "Jun", "Jul", "Aug", "Sep", "Oct", "Nov", "Dec"}
	posixWeekdays = [7]string{"Sun", "Mon", "Tue", "Wed", "Thu", "Fri", "Sat"}
	germanMonths  = [12]string{"Jan", "Feb", "Mär", "Apr", "Mai", "Jun", "Jul", "Aug", "Sep", "Okt", "Nov", "Dez"}
	germanDays    = [7]string{"So", "Mo", "Di", "Mi", "Do", "Fr", "Sa"}
)

// ResolveTime applies POSIX LC_TIME precedence and returns an embedded time
// formatter. C and POSIX accept an optional codeset. de_DE is available in
// UTF-8 and ISO-8859-1 because those exact abbreviated names are already part
// of coreutils' bounded locale corpus. No host locale database is consulted.
func ResolveTime(env []string) (TimeFormatter, error) {
	name := Resolve(env, Time)
	base, codeset := splitName(name)
	switch base {
	case "C", "POSIX":
		return TimeFormatter{
			months: posixMonths, weekdays: posixWeekdays, locale: "POSIX",
			dateTime: "%a %b %e %H:%M:%S %Y", date: "%m/%d/%y",
			clock: "%H:%M:%S", clock12: "%I:%M:%S %p", am: "AM", pm: "PM",
		}, nil
	case "de_DE":
		f := TimeFormatter{
			months: germanMonths, weekdays: germanDays, locale: "de_DE",
			dateTime: "%a %d %b %Y %T %Z", date: "%d.%m.%Y",
			clock: "%T", clock12: "",
		}
		switch {
		case isUTF8Name(codeset):
			return f, nil
		case codeset == "", isISO88591Name(codeset):
			// The unqualified de_DE locale in the embedded certification
			// corpus uses ISO-8859-1, matching its localedef definition.
			f.latin1 = true
			return f, nil
		}
	}
	return TimeFormatter{}, fmt.Errorf(
		"LC_TIME locale %q is unavailable: embedded time data is limited to C/POSIX and de_DE",
		name)
}

var englishMonthNames = [12]string{"January", "February", "March", "April", "May", "June", "July", "August", "September", "October", "November", "December"}
var germanMonthNames = [12]string{"Januar", "Februar", "März", "April", "Mai", "Juni", "Juli", "August", "September", "Oktober", "November", "Dezember"}
var englishWeekdayNames = [7]string{"Sunday", "Monday", "Tuesday", "Wednesday", "Thursday", "Friday", "Saturday"}
var germanWeekdayNames = [7]string{"Sonntag", "Montag", "Dienstag", "Mittwoch", "Donnerstag", "Freitag", "Samstag"}

// ParseMonth and ParseWeekday recognize only names supplied by this bounded
// locale provider. They intentionally do not consult the host locale database.
func (f TimeFormatter) ParseMonth(word string) (time.Month, bool) {
	word = strings.TrimSuffix(word, ",")
	full := englishMonthNames
	if f.locale == "de_DE" {
		full = germanMonthNames
	}
	for i := range f.months {
		short, long := f.months[i], full[i]
		if f.latin1 {
			short, long = toLatin1(short), toLatin1(long)
		}
		if strings.EqualFold(word, short) || strings.EqualFold(word, long) {
			return time.Month(i + 1), true
		}
	}
	return 0, false
}

func (f TimeFormatter) ParseWeekday(word string) (time.Weekday, bool) {
	full := englishWeekdayNames
	if f.locale == "de_DE" {
		full = germanWeekdayNames
	}
	for i := range f.weekdays {
		short, long := f.weekdays[i], full[i]
		if f.latin1 {
			short, long = toLatin1(short), toLatin1(long)
		}
		if strings.EqualFold(word, short) || strings.EqualFold(word, long) {
			return time.Weekday(i), true
		}
	}
	return 0, false
}

// FormatMonthDayTime renders the who(1) POSIX-locale time field shape,
// localizing its abbreviated month and output encoding through this provider.
func (f TimeFormatter) FormatMonthDayTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	month := f.months[int(t.Month())-1]
	if f.latin1 {
		month = toLatin1(month)
	}
	return fmt.Sprintf("%s %2d %02d:%02d", month, t.Day(), t.Hour(), t.Minute())
}

// FormatAtJobTime renders the POSIX at/batch confirmation and listing date
// shape: date +"%a %b %e %T %Y".
func (f TimeFormatter) FormatAtJobTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	weekday := f.weekdays[int(t.Weekday())]
	month := f.months[int(t.Month())-1]
	if f.latin1 {
		weekday = toLatin1(weekday)
		month = toLatin1(month)
	}
	return fmt.Sprintf("%s %s %2d %02d:%02d:%02d %04d",
		weekday, month, t.Day(), t.Hour(), t.Minute(), t.Second(), t.Year())
}

// Format renders the complete POSIX Issue 7 date operand conversion set with
// this formatter's invocation-selected names and output encoding. Unsupported
// conversions fail closed so callers cannot silently emit a non-conforming
// timestamp.
func (f TimeFormatter) Format(t time.Time, format string) (string, error) {
	var out strings.Builder
	for i := 0; i < len(format); i++ {
		if format[i] != '%' {
			out.WriteByte(format[i])
			continue
		}
		i++
		if i >= len(format) {
			return "", fmt.Errorf("incomplete date conversion")
		}
		if format[i] == 'E' || format[i] == 'O' {
			modifier := format[i]
			if i+1 >= len(format) {
				return "", fmt.Errorf("incomplete date conversion")
			}
			i++
			valid := modifier == 'E' && strings.ContainsRune("cCxXyY", rune(format[i])) ||
				modifier == 'O' && strings.ContainsRune("deHImMSuUVwWy", rune(format[i]))
			if !valid {
				return "", fmt.Errorf("unsupported date conversion %%%c%c", modifier, format[i])
			}
			// Carried locales have no alternative eras or digits, so valid
			// modifiers render through the corresponding base conversion.
		}
		switch format[i] {
		case '%':
			out.WriteByte('%')
		case 'Y':
			fmt.Fprintf(&out, "%04d", t.Year())
		case 'y':
			fmt.Fprintf(&out, "%02d", t.Year()%100)
		case 'C':
			fmt.Fprintf(&out, "%02d", t.Year()/100)
		case 'm':
			fmt.Fprintf(&out, "%02d", t.Month())
		case 'd':
			fmt.Fprintf(&out, "%02d", t.Day())
		case 'e':
			fmt.Fprintf(&out, "%2d", t.Day())
		case 'j':
			fmt.Fprintf(&out, "%03d", t.YearDay())
		case 'H':
			fmt.Fprintf(&out, "%02d", t.Hour())
		case 'I':
			hour := t.Hour() % 12
			if hour == 0 {
				hour = 12
			}
			fmt.Fprintf(&out, "%02d", hour)
		case 'M':
			fmt.Fprintf(&out, "%02d", t.Minute())
		case 'S':
			fmt.Fprintf(&out, "%02d", t.Second())
		case 'a':
			out.WriteString(f.weekdayName(t.Weekday(), false))
		case 'A':
			out.WriteString(f.weekdayName(t.Weekday(), true))
		case 'b', 'h':
			out.WriteString(f.monthName(t.Month(), false))
		case 'B':
			out.WriteString(f.monthName(t.Month(), true))
		case 'c':
			text, err := f.Format(t, f.dateTime)
			if err != nil {
				return "", err
			}
			out.WriteString(text)
		case 'D':
			fmt.Fprintf(&out, "%02d/%02d/%02d", t.Month(), t.Day(), t.Year()%100)
		case 'F':
			fmt.Fprintf(&out, "%04d-%02d-%02d", t.Year(), t.Month(), t.Day())
		case 'g':
			year, _ := t.ISOWeek()
			fmt.Fprintf(&out, "%02d", year%100)
		case 'G':
			year, _ := t.ISOWeek()
			fmt.Fprintf(&out, "%04d", year)
		case 'p':
			if t.Hour() < 12 {
				out.WriteString(f.am)
			} else {
				out.WriteString(f.pm)
			}
		case 'r':
			text, err := f.Format(t, f.clock12)
			if err != nil {
				return "", err
			}
			out.WriteString(text)
		case 'R':
			fmt.Fprintf(&out, "%02d:%02d", t.Hour(), t.Minute())
		case 'T':
			fmt.Fprintf(&out, "%02d:%02d:%02d", t.Hour(), t.Minute(), t.Second())
		case 'X':
			text, err := f.Format(t, f.clock)
			if err != nil {
				return "", err
			}
			out.WriteString(text)
		case 'u':
			weekday := int(t.Weekday())
			if weekday == 0 {
				weekday = 7
			}
			fmt.Fprintf(&out, "%d", weekday)
		case 'U':
			fmt.Fprintf(&out, "%02d", timeWeekNumber(t, time.Sunday))
		case 'V':
			_, week := t.ISOWeek()
			fmt.Fprintf(&out, "%02d", week)
		case 'w':
			fmt.Fprintf(&out, "%d", t.Weekday())
		case 'W':
			fmt.Fprintf(&out, "%02d", timeWeekNumber(t, time.Monday))
		case 'x':
			text, err := f.Format(t, f.date)
			if err != nil {
				return "", err
			}
			out.WriteString(text)
		case 'z':
			out.WriteString(t.Format("-0700"))
		case 'Z':
			out.WriteString(t.Format("MST"))
		case 'n':
			out.WriteByte('\n')
		case 't':
			out.WriteByte('\t')
		default:
			return "", fmt.Errorf("unsupported date conversion %%%c", format[i])
		}
	}
	return out.String(), nil
}

func (f TimeFormatter) monthName(month time.Month, full bool) string {
	name := f.months[int(month)-1]
	if full {
		if f.locale == "de_DE" {
			name = germanMonthNames[int(month)-1]
		} else {
			name = englishMonthNames[int(month)-1]
		}
	}
	if f.latin1 {
		name = toLatin1(name)
	}
	return name
}

func (f TimeFormatter) weekdayName(day time.Weekday, full bool) string {
	name := f.weekdays[int(day)]
	if full {
		if f.locale == "de_DE" {
			name = germanWeekdayNames[int(day)]
		} else {
			name = englishWeekdayNames[int(day)]
		}
	}
	if f.latin1 {
		name = toLatin1(name)
	}
	return name
}

func timeWeekNumber(t time.Time, firstDay time.Weekday) int {
	yearDay := t.YearDay() - 1
	jan1 := time.Date(t.Year(), 1, 1, 0, 0, 0, 0, t.Location()).Weekday()
	offset := (int(firstDay) - int(jan1) + 7) % 7
	if yearDay < offset {
		return 0
	}
	return 1 + (yearDay-offset)/7
}

func splitName(name string) (base, codeset string) {
	name, _, _ = strings.Cut(name, "@")
	base, codeset, _ = strings.Cut(name, ".")
	return base, codeset
}

func isUTF8Name(codeset string) bool {
	c := strings.ToUpper(strings.NewReplacer("-", "", "_", "").Replace(codeset))
	return c == "UTF8"
}

func isISO88591Name(codeset string) bool {
	c := strings.ToUpper(strings.NewReplacer("-", "", "_", "").Replace(codeset))
	return c == "ISO88591"
}

func toLatin1(s string) string {
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
