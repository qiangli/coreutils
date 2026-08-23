package localecmd

import "strconv"

// The keyword tables for the POSIX (a.k.a. C) locale, transcribed from the
// POSIX locale definition in XBD chapter 7.3. They are the ONLY locale data
// this implementation has: there is no compiled locale database to read and
// no libc to ask, so any other locale is refused by name rather than answered
// with C's values (see localeData). Returning C's decimal point for a locale
// that uses a comma is the kind of silent wrong answer this repository's
// contract exists to prevent.

// valueKind decides how a keyword's value is rendered. It is not cosmetic:
// `locale -k` quotes strings and leaves numbers bare, so a keyword classified
// wrongly produces output no consumer can parse.
type valueKind int

const (
	kindString valueKind = iota
	kindNumber
	kindStringList
)

type keyword struct {
	Name     string
	Category string
	Kind     valueKind
	// Values holds one entry for a scalar keyword and N for a list.
	Values []string
}

func str(cat, name, v string) keyword {
	return keyword{Name: name, Category: cat, Kind: kindString, Values: []string{v}}
}

func num(cat, name, v string) keyword {
	return keyword{Name: name, Category: cat, Kind: kindNumber, Values: []string{v}}
}

func list(cat, name string, v ...string) keyword {
	return keyword{Name: name, Category: cat, Kind: kindStringList, Values: v}
}

// categories is the POSIX locale-category vocabulary, in the order the
// no-operand listing writes them.
var categories = []string{
	"LC_CTYPE", "LC_NUMERIC", "LC_TIME", "LC_COLLATE", "LC_MONETARY", "LC_MESSAGES",
}

// posixKeywords is every keyword this utility can answer, with its POSIX-locale
// value.
//
// Two categories carry no scalar keywords. LC_COLLATE's locale definition is a
// set of collation RULES, not name=value settings, and LC_CTYPE's is a set of
// character CLASSES; neither has anything a `locale <keyword>` query names. The
// four LC_CTYPE entries below are the codeset facts that do have names, and
// they are derived from the locale's codeset rather than fixed — see
// localeData.
var posixKeywords = func() []keyword {
	k := []keyword{
		// LC_NUMERIC — XBD 7.3.4. The POSIX locale has no thousands separator
		// and no grouping, both expressed as empty strings.
		str("LC_NUMERIC", "decimal_point", "."),
		str("LC_NUMERIC", "thousands_sep", ""),
		str("LC_NUMERIC", "grouping", ""),

		// LC_TIME — XBD 7.3.5.
		list("LC_TIME", "abday", "Sun", "Mon", "Tue", "Wed", "Thu", "Fri", "Sat"),
		list("LC_TIME", "day", "Sunday", "Monday", "Tuesday", "Wednesday", "Thursday", "Friday", "Saturday"),
		list("LC_TIME", "abmon", "Jan", "Feb", "Mar", "Apr", "May", "Jun", "Jul", "Aug", "Sep", "Oct", "Nov", "Dec"),
		list("LC_TIME", "mon", "January", "February", "March", "April", "May", "June",
			"July", "August", "September", "October", "November", "December"),
		str("LC_TIME", "d_t_fmt", "%a %b %e %H:%M:%S %Y"),
		str("LC_TIME", "d_fmt", "%m/%d/%y"),
		str("LC_TIME", "t_fmt", "%H:%M:%S"),
		list("LC_TIME", "am_pm", "AM", "PM"),
		str("LC_TIME", "t_fmt_ampm", "%I:%M:%S %p"),
		// The era keywords are defined but empty in the POSIX locale: the
		// Gregorian calendar needs no era rules.
		str("LC_TIME", "era", ""),
		str("LC_TIME", "era_d_fmt", ""),
		str("LC_TIME", "era_t_fmt", ""),
		str("LC_TIME", "era_d_t_fmt", ""),
		str("LC_TIME", "alt_digits", ""),

		// LC_MONETARY — XBD 7.3.3. Every string is empty and every numeric
		// value is -1, which is the encoding for "unspecified in this locale";
		// zero would mean something quite different (e.g. frac_digits 0 says
		// the currency has no minor unit).
		str("LC_MONETARY", "int_curr_symbol", ""),
		str("LC_MONETARY", "currency_symbol", ""),
		str("LC_MONETARY", "mon_decimal_point", ""),
		str("LC_MONETARY", "mon_thousands_sep", ""),
		str("LC_MONETARY", "mon_grouping", ""),
		str("LC_MONETARY", "positive_sign", ""),
		str("LC_MONETARY", "negative_sign", ""),
		num("LC_MONETARY", "int_frac_digits", "-1"),
		num("LC_MONETARY", "frac_digits", "-1"),
		num("LC_MONETARY", "p_cs_precedes", "-1"),
		num("LC_MONETARY", "p_sep_by_space", "-1"),
		num("LC_MONETARY", "n_cs_precedes", "-1"),
		num("LC_MONETARY", "n_sep_by_space", "-1"),
		num("LC_MONETARY", "p_sign_posn", "-1"),
		num("LC_MONETARY", "n_sign_posn", "-1"),
		num("LC_MONETARY", "int_p_cs_precedes", "-1"),
		num("LC_MONETARY", "int_p_sep_by_space", "-1"),
		num("LC_MONETARY", "int_n_cs_precedes", "-1"),
		num("LC_MONETARY", "int_n_sep_by_space", "-1"),
		num("LC_MONETARY", "int_p_sign_posn", "-1"),
		num("LC_MONETARY", "int_n_sign_posn", "-1"),

		// LC_MESSAGES — XBD 7.3.6.
		str("LC_MESSAGES", "yesexpr", "^[yY]"),
		str("LC_MESSAGES", "noexpr", "^[nN]"),
		str("LC_MESSAGES", "yesstr", ""),
		str("LC_MESSAGES", "nostr", ""),
	}
	return k
}()

// codesetKeywords are the LC_CTYPE entries whose values depend on the locale's
// codeset rather than being fixed by the POSIX locale definition.
func codesetKeywords(charmap string, mbCurMax int) []keyword {
	return []keyword{
		str("LC_CTYPE", "charmap", charmap),
		str("LC_CTYPE", "code_set_name", charmap),
		num("LC_CTYPE", "mb_cur_max", strconv.Itoa(mbCurMax)),
		num("LC_CTYPE", "mb_cur_min", "1"),
	}
}
