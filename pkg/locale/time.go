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
	months [12]string
	latin1 bool
}

var (
	posixMonths  = [12]string{"Jan", "Feb", "Mar", "Apr", "May", "Jun", "Jul", "Aug", "Sep", "Oct", "Nov", "Dec"}
	germanMonths = [12]string{"Jan", "Feb", "Mär", "Apr", "Mai", "Jun", "Jul", "Aug", "Sep", "Okt", "Nov", "Dez"}
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
		return TimeFormatter{months: posixMonths}, nil
	case "de_DE":
		f := TimeFormatter{months: germanMonths}
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
