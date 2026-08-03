package grepcmd

import "strings"

// grepLocale is the locale-dependent part of POSIX regular-expression
// matching. The certification locale is the repository's already-supported
// de_DE ISO-8859-1 locale; keeping its tables in-process makes results
// independent of whichever locale archive happens to be installed on a host.
type grepLocale struct {
	ctypeGerman   bool
	collateGerman bool
}

func grepLocaleFromEnv(env []string) grepLocale {
	return grepLocale{
		ctypeGerman:   germanLocale(localeCategory(env, "LC_CTYPE")),
		collateGerman: germanLocale(localeCategory(env, "LC_COLLATE")),
	}
}

func localeCategory(env []string, category string) string {
	for _, key := range []string{"LC_ALL", category, "LANG"} {
		if value, ok := localeEnv(env, key); ok && value != "" {
			return value
		}
	}
	return "POSIX"
}

func localeEnv(env []string, key string) (string, bool) {
	prefix := key + "="
	for i := len(env) - 1; i >= 0; i-- {
		if strings.HasPrefix(env[i], prefix) {
			return env[i][len(prefix):], true
		}
	}
	return "", false
}

func germanLocale(name string) bool {
	return strings.HasPrefix(strings.ToLower(name), "de_de")
}

func (l grepLocale) rewritePattern(pattern string) string {
	if l.ctypeGerman {
		// POSIX classes are embedded inside an outer bracket expression.
		// Append the ISO-8859-1 alphabetic ranges to that same expression;
		// the subject is decoded to Unicode below before matching.
		pattern = strings.ReplaceAll(pattern, "[:alpha:]", "[:alpha:]À-ÖØ-öø-ÿ")
		pattern = strings.ReplaceAll(pattern, "[:alnum:]", "[:alnum:]À-ÖØ-öø-ÿ")
		pattern = strings.ReplaceAll(pattern, "[:upper:]", "[:upper:]À-ÖØ-Þ")
		pattern = strings.ReplaceAll(pattern, "[:lower:]", "[:lower:]ß-öø-ÿ")
	}
	if l.collateGerman {
		// German collation groups umlauts with their base letters and sharp-s
		// with s. Replacing the inner equivalence element leaves an ordinary
		// member list for pkg/bre's bracket translator.
		repl := strings.NewReplacer(
			"[=a=]", "aAÄä", "[=A=]", "AaÄä",
			"[=o=]", "oOÖö", "[=O=]", "OoÖö",
			"[=u=]", "uUÜü", "[=U=]", "UuÜü",
			"[=s=]", "sSß", "[=S=]", "Ssß",
		)
		pattern = repl.Replace(pattern)
	}
	return pattern
}

// localeMatcher decodes ISO-8859-1 input bytes before regexp matching. It
// maps match extents back to original byte offsets so -o remains byte-exact.
type localeMatcher struct{ inner grepMatcher }

func (m localeMatcher) MatchString(s string) bool {
	decoded, _ := decodeLatin1(s)
	return m.inner.MatchString(decoded)
}

func (m localeMatcher) FindStringIndex(s string) []int {
	decoded, offsets := decodeLatin1(s)
	loc := m.inner.FindStringIndex(decoded)
	if loc == nil {
		return nil
	}
	return []int{offsets[loc[0]], offsets[loc[1]]}
}

func decodeLatin1(s string) (string, map[int]int) {
	var b strings.Builder
	b.Grow(len(s))
	offsets := make(map[int]int, len(s)+1)
	for i := 0; i < len(s); i++ {
		offsets[b.Len()] = i
		b.WriteRune(rune(s[i]))
	}
	offsets[b.Len()] = len(s)
	return b.String(), offsets
}
