package grepcmd

import (
	"fmt"
	"strings"
)

// grepLocale is the locale-dependent part of POSIX regular-expression
// matching. The certification locale is the repository's already-supported
// de_DE ISO-8859-1 locale; keeping its tables in-process makes results
// independent of whichever locale archive happens to be installed on a host.
type grepLocale struct {
	ctypeGerman   bool
	collateGerman bool
}

func (l grepLocale) latin1Bytes() bool { return l.ctypeGerman || l.collateGerman }

func grepLocaleFromEnv(env []string) (grepLocale, error) {
	ctypeName := localeCategory(env, "LC_CTYPE")
	collateName := localeCategory(env, "LC_COLLATE")
	ctypeGerman, err := germanLocale(ctypeName)
	if err != nil {
		return grepLocale{}, fmt.Errorf("LC_CTYPE=%s: %w", ctypeName, err)
	}
	collateGerman, err := germanLocale(collateName)
	if err != nil {
		return grepLocale{}, fmt.Errorf("LC_COLLATE=%s: %w", collateName, err)
	}
	return grepLocale{ctypeGerman: ctypeGerman, collateGerman: collateGerman}, nil
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

func germanLocale(name string) (bool, error) {
	if name == "C" || name == "POSIX" {
		return false, nil
	}
	switch strings.ToLower(name) {
	case "de_de.iso-8859-1", "de_de.iso88591":
		return true, nil
	default:
		return false, fmt.Errorf("unsupported locale; expected C, POSIX, de_DE.ISO-8859-1, or de_DE.iso88591")
	}
}

// rewritePattern augments a bracket-expression pattern with the certification
// locale's class and equivalence members. The caller has already decoded any
// raw ISO-8859-1 high bytes to runes (see latin1ToRunes), so the ASCII class
// and equivalence tokens matched here are byte-for-byte intact.
func (l grepLocale) rewritePattern(pattern string) string {
	if l.ctypeGerman {
		// POSIX classes are embedded inside an outer bracket expression. RE2's
		// named classes are ASCII, so append every non-ASCII member of the
		// bounded de_DE ISO-8859-1 tables. The three isolated alphabetics are
		// feminine ordinal, micro sign, and masculine ordinal (AA/B5/BA).
		pattern = strings.ReplaceAll(pattern, "[:alpha:]", "[:alpha:]ªµºÀ-ÖØ-öø-ÿ")
		pattern = strings.ReplaceAll(pattern, "[:alnum:]", "[:alnum:]ªµºÀ-ÖØ-öø-ÿ")
		pattern = strings.ReplaceAll(pattern, "[:upper:]", "[:upper:]À-ÖØ-Þ")
		pattern = strings.ReplaceAll(pattern, "[:lower:]", "[:lower:]ªµºß-öø-ÿ")
		pattern = strings.ReplaceAll(pattern, "[:cntrl:]", "[:cntrl:]\u0080-\u009f")
		pattern = strings.ReplaceAll(pattern, "[:graph:]", "[:graph:]¡-ÿ")
		pattern = strings.ReplaceAll(pattern, "[:print:]", "[:print:] -ÿ")
		pattern = strings.ReplaceAll(pattern, "[:punct:]", "[:punct:]¡-©«-´¶-¹»-¿×÷")
		pattern = strings.ReplaceAll(pattern, "[:word:]", "[:word:]ªµºÀ-ÖØ-öø-ÿ")
	}
	if l.collateGerman {
		// German collation groups umlauts with their base letters and sharp-s
		// with s. Replacing the inner equivalence element leaves an ordinary
		// member list for pkg/bre's bracket translator.
		repl := strings.NewReplacer(
			"[=a=]", "aAÀÁÂÃÄÅàáâãäåª", "[=A=]", "AaÀÁÂÃÄÅàáâãäåª",
			"[=c=]", "cCÇç", "[=C=]", "CcÇç",
			"[=e=]", "eEÈÉÊËèéêë", "[=E=]", "EeÈÉÊËèéêë",
			"[=i=]", "iIÌÍÎÏìíîï", "[=I=]", "IiÌÍÎÏìíîï",
			"[=n=]", "nNÑñ", "[=N=]", "NnÑñ",
			"[=o=]", "oOÒÓÔÕÖØòóôõöøº", "[=O=]", "OoÒÓÔÕÖØòóôõöøº",
			"[=u=]", "uUÙÚÛÜùúûü", "[=U=]", "UuÙÚÛÜùúûü",
			"[=y=]", "yYÝýÿ", "[=Y=]", "YyÝýÿ",
			"[=s=]", "sSß", "[=S=]", "Ssß",
		)
		pattern = repl.Replace(pattern)
		// The bounded German collation puts the a-umlaut group between a and
		// b. Pin the command-visible range forms rather than falling back to
		// Unicode/code-point order, where ä follows b.
		alpha := "A-Za-zªµºÀ-ÖØ-öø-ÿ"
		pattern = strings.NewReplacer(
			"[[.a.]-[.b.]]", "[aAÀÁÂÃÄÅàáâãäåªbB]",
			"[[.A.]-[.B.]]", "[AaÀÁÂÃÄÅàáâãäåªBb]",
			"[a-b]", "[aAÀÁÂÃÄÅàáâãäåªbB]",
			"[A-B]", "[AaÀÁÂÃÄÅàáâãäåªBb]",
			"[a-z]", "["+alpha+"]",
			"[A-Z]", "["+alpha+"]",
		).Replace(pattern)
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

// latin1ToRunes maps each byte of s to the rune with the same code point,
// i.e. decodes an ISO-8859-1 string to its UTF-8 rune sequence. It is the
// pattern-side counterpart of decodeLatin1's subject decoding; it needs no
// offset table because pattern byte offsets are never reported.
func latin1ToRunes(s string) string {
	ascii := true
	for i := 0; i < len(s); i++ {
		if s[i] >= 0x80 {
			ascii = false
			break
		}
	}
	if ascii {
		return s
	}
	var b strings.Builder
	b.Grow(len(s) + 8)
	for i := 0; i < len(s); i++ {
		b.WriteRune(rune(s[i]))
	}
	return b.String()
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
