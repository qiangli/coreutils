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
	months   [12]string
	weekdays [7]string
	latin1   bool
	locale   string
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
		return TimeFormatter{months: posixMonths, weekdays: posixWeekdays, locale: "POSIX"}, nil
	case "de_DE":
		f := TimeFormatter{months: germanMonths, weekdays: germanDays, locale: "de_DE"}
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
