// Package localecmd implements locale(1): write information about the current
// locale environment, or about all available locales.
//
// # Two very different jobs under one name
//
// `locale` with no operands reports the ENVIRONMENT — which variable supplied
// each category's setting. That needs no locale database at all, and this
// implementation answers it exactly.
//
// `locale <keyword>` reports locale DATA — the decimal point, the month names,
// the currency symbol. That needs a compiled locale database, which a pure-Go
// program running on a host with no libc bridge does not have. What it does
// have is a small built-in database: the normative POSIX locale definition and
// the required scalar categories for the certification fixture's
// de_DE.ISO-8859-1 locale. Every category not actually carried is refused by
// name.
//
// The refusal is the important part. Answering `LC_NUMERIC=de_DE locale
// decimal_point` with "." because that is what C says would be a silent wrong
// answer of exactly the kind this repository's contract forbids — and it would
// be wrong in the direction that breaks number parsing downstream, where
// nothing reports it.
//
// # The quoting rule in the no-operand listing
//
// LANG and LC_ALL are written as raw environment values. Each category is
// written UNQUOTED when its own variable supplied the value, and QUOTED when
// the value was derived — from LC_ALL, from LANG, or from the implementation
// default. The quotes are how a reader tells "someone set this" from "this is
// what fell out", which is the only question the listing exists to answer.
package localecmd

import (
	"fmt"
	"strings"

	"github.com/qiangli/coreutils/pkg/locale"
	"github.com/qiangli/coreutils/tool"
)

var cmd = &tool.Tool{
	Name:     "locale",
	Synopsis: "Get locale-specific information.",
	Usage: `locale [-a|-m]
  locale [-ck] name...`,
}

func init() { cmd.Run = run; tool.Register(cmd) }

func run(rc *tool.RunContext, args []string) int {
	// NOT tool.AliasHelpVersion: it rewrites any short cluster containing an
	// 'h' into --help. locale's clusters are short, but tool.Parse registers
	// the -h/-V aliases itself, so the pre-pass buys nothing and can only
	// misfire.
	fs := tool.NewFlags(cmd.Name)
	all := fs.BoolP("all-locales", "a", false, "write names of available locales")
	charmaps := fs.BoolP("charmaps", "m", false, "write names of available charmaps")
	withCategory := fs.BoolP("category-name", "c", false, "write the category name before its keywords")
	withKeyword := fs.BoolP("keyword-name", "k", false, "write keyword=value pairs")
	operands, code := tool.Parse(rc, cmd, fs, args)
	if code >= 0 {
		return code
	}

	switch {
	case *all && *charmaps:
		return tool.UsageError(rc, cmd, "-a and -m are mutually exclusive")
	case (*all || *charmaps) && (*withCategory || *withKeyword):
		return tool.UsageError(rc, cmd, "-a and -m cannot be combined with -c or -k")
	case (*all || *charmaps) && len(operands) > 0:
		return tool.UsageError(rc, cmd, "-a and -m take no operands")
	}

	switch {
	case *all:
		return writeLines(rc, availableLocales())
	case *charmaps:
		return writeLines(rc, availableCharmaps())
	case len(operands) == 0:
		if *withCategory || *withKeyword {
			return tool.UsageError(rc, cmd, "-c and -k require a name operand")
		}
		return writeEnvironment(rc)
	default:
		return writeNames(rc, operands, *withCategory, *withKeyword)
	}
}

func writeLines(rc *tool.RunContext, lines []string) int {
	for _, l := range lines {
		if _, err := fmt.Fprintln(rc.Out, l); err != nil {
			fmt.Fprintf(rc.Err, "locale: %v\n", err)
			return 1
		}
	}
	return 0
}

// writeEnvironment implements the no-operand form.
func writeEnvironment(rc *tool.RunContext) int {
	lcAll := rc.Getenv("LC_ALL")
	lang := rc.Getenv("LANG")

	if _, err := fmt.Fprintf(rc.Out, "LANG=%s\n", lang); err != nil {
		fmt.Fprintf(rc.Err, "locale: %v\n", err)
		return 1
	}
	for _, cat := range categories {
		var err error
		if v := rc.Getenv(cat); v != "" && lcAll == "" {
			// The category's own variable supplied it: written bare.
			_, err = fmt.Fprintf(rc.Out, "%s=%s\n", cat, v)
		} else {
			// Derived — from LC_ALL, else LANG, else the default. LC_ALL wins even
			// over a category variable that is also set, so a set LC_ALL makes
			// EVERY line quoted; that is the visible signal that the per-category
			// settings are being overridden and are not in effect.
			derived := lcAll
			if derived == "" {
				derived = lang
			}
			if derived == "" {
				derived = locale.Default
			}
			_, err = fmt.Fprintf(rc.Out, "%s=%q\n", cat, derived)
		}
		if err != nil {
			fmt.Fprintf(rc.Err, "locale: %v\n", err)
			return 1
		}
	}
	if _, err := fmt.Fprintf(rc.Out, "LC_ALL=%s\n", lcAll); err != nil {
		fmt.Fprintf(rc.Err, "locale: %v\n", err)
		return 1
	}
	return 0
}

// writeNames implements the operand form: a category name, a keyword name, or
// the special name "charmap".
func writeNames(rc *tool.RunContext, names []string, withCategory, withKeyword bool) int {
	status := 0
	for _, name := range names {
		if err := writeName(rc, name, withCategory, withKeyword); err != nil {
			fmt.Fprintf(rc.Err, "locale: %v\n", err)
			status = 1
		}
	}
	return status
}

func writeName(rc *tool.RunContext, name string, withCategory, withKeyword bool) error {
	if isCategory(name) {
		data, err := localeData(rc, name)
		if err != nil {
			return err
		}
		if withCategory {
			if _, err := fmt.Fprintln(rc.Out, name); err != nil {
				return err
			}
		}
		for _, k := range data.keywordsIn(name) {
			if _, err := fmt.Fprintln(rc.Out, render(k, withKeyword)); err != nil {
				return err
			}
		}
		return nil
	}

	// "charmap" is a keyword of LC_CTYPE, and also the documented spelling for
	// asking which charmap is in effect. Both reach the same entry.
	cat, ok := categoryOfKeyword(name)
	if !ok {
		return fmt.Errorf("unknown name %q", name)
	}
	data, err := localeData(rc, cat)
	if err != nil {
		return err
	}
	k, ok := data.keyword(name)
	if !ok {
		return fmt.Errorf("unknown name %q", name)
	}
	if withCategory {
		if _, err := fmt.Fprintln(rc.Out, cat); err != nil {
			return err
		}
	}
	if _, err := fmt.Fprintln(rc.Out, render(k, withKeyword)); err != nil {
		return err
	}
	return nil
}

// render formats one keyword. With -k the name is included and strings are
// quoted; without it only the value is written, list elements separated by
// semicolons.
func render(k keyword, withKeyword bool) string {
	if !withKeyword {
		return strings.Join(k.Values, ";")
	}
	switch k.Kind {
	case kindNumber:
		return k.Name + "=" + k.Values[0]
	case kindStringList:
		quoted := make([]string, len(k.Values))
		for i, v := range k.Values {
			quoted[i] = quoteLocaleString(v)
		}
		return k.Name + "=" + strings.Join(quoted, ";")
	default:
		return k.Name + "=" + quoteLocaleString(k.Values[0])
	}
}

func quoteLocaleString(value string) string {
	value = strings.ReplaceAll(value, `\`, `\\`)
	value = strings.ReplaceAll(value, `"`, `\"`)
	return `"` + value + `"`
}

func isCategory(name string) bool {
	for _, c := range categories {
		if c == name {
			return true
		}
	}
	return false
}

func categoryOfKeyword(name string) (string, bool) {
	for _, k := range posixKeywords {
		if k.Name == name {
			return k.Category, true
		}
	}
	for _, k := range codesetKeywords("", 1) {
		if k.Name == name {
			return k.Category, true
		}
	}
	return "", false
}

// data is the keyword set for one resolved locale.
type data struct {
	name     string
	keywords []keyword
}

func (d data) keywordsIn(cat string) []keyword {
	var out []keyword
	for _, k := range d.keywords {
		if k.Category == cat {
			out = append(out, k)
		}
	}
	return out
}

func (d data) keyword(name string) (keyword, bool) {
	for _, k := range d.keywords {
		if k.Name == name {
			return k, true
		}
	}
	return keyword{}, false
}

// localeData resolves the locale in effect for cat and returns its keyword set,
// or an error naming the locale this build cannot describe.
func localeData(rc *tool.RunContext, cat string) (data, error) {
	name := locale.Resolve(rc.Env, locale.Category(cat))
	if cat == "LC_MESSAGES" {
		if messages, ok := locale.LookupMessages(name); ok {
			return data{name: name, keywords: messagesKeywords(messages)}, nil
		}
	}
	base, codeset := splitLocaleName(name)
	if base == "de_DE" && isISO88591(codeset) {
		var keywords []keyword
		switch cat {
		case "LC_COLLATE":
			// LC_COLLATE has rules rather than scalar locale(1) keywords.
			keywords = []keyword{}
		case "LC_CTYPE":
			keywords = codesetKeywords(iso88591Charmap, 1)
		case "LC_NUMERIC":
			keywords = germanISO88591NumericKeywords
		case "LC_MONETARY":
			keywords = germanISO88591MonetaryKeywords
		case "LC_TIME":
			keywords = germanISO88591TimeKeywords
		}
		if keywords != nil {
			return data{name: name, keywords: keywords}, nil
		}
	}
	if base != "C" && base != "POSIX" {
		return data{}, fmt.Errorf(
			"locale %q is not available: pure-Go coreutils does not carry %s data for it",
			name, cat)
	}

	charmap, mbCurMax := posixCharmap, 1
	if codeset != "" {
		charmap = codeset
		if isUTF8(codeset) {
			mbCurMax = 4
		}
	}
	kws := make([]keyword, 0, len(posixKeywords)+4)
	kws = append(kws, codesetKeywords(charmap, mbCurMax)...)
	kws = append(kws, posixKeywords...)
	return data{name: name, keywords: kws}, nil
}

// posixCharmap is the charmap name of the bare POSIX locale: 7-bit US-ASCII
// under its IANA-registered name, which is what a C-locale host reports.
const posixCharmap = "ANSI_X3.4-1968"

// Locale names admit several spellings of the codeset. The charmap keyword is
// the canonical public name, not a copy of whichever alias selected it.
const iso88591Charmap = "ISO-8859-1"

// splitLocaleName separates "C.UTF-8@modifier" into its base name and codeset.
// The modifier is discarded: no modifier is defined for the POSIX locale, and
// carrying one into the charmap would report a codeset nobody named.
func splitLocaleName(name string) (base, codeset string) {
	name, _, _ = strings.Cut(name, "@")
	base, codeset, _ = strings.Cut(name, ".")
	return base, codeset
}

func isUTF8(codeset string) bool {
	c := strings.ToUpper(strings.ReplaceAll(codeset, "-", ""))
	return c == "UTF8"
}

func isISO88591(codeset string) bool {
	c := strings.ToUpper(codeset)
	c = strings.NewReplacer("-", "", "_", "").Replace(c)
	return c == "ISO88591"
}
