package localecmd

// Locales compiled by this repository's localedef(1) are read from the Go
// locale store before anything else. That order is the whole point: a locale
// the operator compiled here is data we own and can describe exactly, so it
// outranks both the host locale service and the built-in fixture tables.
//
// A category the compiled locale does not carry is NOT answered from it. The
// resolution falls through to the existing behaviour instead, which for an
// unknown locale name is still a refusal by name rather than C's values under
// someone else's locale.

import (
	"github.com/qiangli/coreutils/pkg/locale"
)

// compiledData converts one category of a compiled locale into the keyword
// set locale(1) renders. It reports false when the locale carries no data for
// cat, so the caller keeps its existing behaviour.
func compiledData(c *locale.Compiled, cat string) ([]keyword, bool) {
	if !c.Has(cat) {
		return nil, false
	}
	var kws []keyword
	if cat == "LC_CTYPE" {
		// The codeset facts are the only named LC_CTYPE values; the compiled
		// classes are consumed through pkg/locale, not written by locale(1).
		charmap := c.Charmap
		if charmap == "" {
			charmap = posixCharmap
		}
		mbCurMax := c.MbCurMax
		if mbCurMax < 1 {
			mbCurMax = 1
		}
		kws = append(kws, codesetKeywords(charmap, mbCurMax)...)
	}
	for _, name := range sortedKeywordNames(c, cat) {
		k, _ := c.Keyword(cat, name)
		kws = append(kws, keyword{
			Name:     name,
			Category: cat,
			Kind:     compiledKind(k),
			Values:   k.Values,
		})
	}
	return kws, true
}

// compiledKind decides how the value is written. Getting this wrong is not
// cosmetic: `locale -k` quotes strings and leaves numbers bare, and a consumer
// parses accordingly.
func compiledKind(k locale.Keyword) valueKind {
	switch {
	case len(k.Values) > 1 && k.Numeric:
		return kindNumberList
	case len(k.Values) > 1:
		return kindStringList
	case k.Numeric:
		return kindNumber
	default:
		return kindString
	}
}

// sortedKeywordNames orders a category's keywords so the listing is
// deterministic across runs and hosts, as the agent contract requires.
func sortedKeywordNames(c *locale.Compiled, cat string) []string {
	names := make(map[string]bool, len(c.Categories[cat]))
	for name := range c.Categories[cat] {
		names[name] = true
	}
	return sortedKeys(names)
}
