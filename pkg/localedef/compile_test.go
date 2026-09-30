package localedef

import (
	"strings"
	"testing"

	"github.com/qiangli/coreutils/pkg/locale"
)

func compileSource(t *testing.T, source string) *locale.Compiled {
	t.Helper()
	src, err := ParseSource(strings.NewReader(source))
	if err != nil {
		t.Fatalf("ParseSource: %v", err)
	}
	c, err := Compile("xx_XX", src, DefaultCharmap())
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	return c
}

func TestCompileNumericAndMessages(t *testing.T) {
	c := compileSource(t, "LC_NUMERIC\ndecimal_point \"<exclamation-mark>\"\nthousands_sep \"<underscore>\"\ngrouping 3;3\nEND LC_NUMERIC\n"+
		"LC_MESSAGES\nyesexpr \"^[oO]\"\nnoexpr \"^[nN]\"\nyesstr \"oui\"\nnostr \"non\"\nEND LC_MESSAGES\n")
	if v, _ := c.KeywordString("LC_NUMERIC", "decimal_point"); v != "!" {
		t.Fatalf("decimal_point=%q want !", v)
	}
	if v, _ := c.KeywordString("LC_NUMERIC", "thousands_sep"); v != "_" {
		t.Fatalf("thousands_sep=%q want _", v)
	}
	if v, _ := c.KeywordValues("LC_NUMERIC", "grouping"); len(v) != 2 || v[0] != "3" || v[1] != "3" {
		t.Fatalf("grouping=%v", v)
	}
	if k, _ := c.Keyword("LC_NUMERIC", "grouping"); !k.Numeric {
		t.Fatal("grouping is not marked numeric")
	}
	d, ok := c.MessagesData()
	if !ok || d.YesExpr != "^[oO]" || d.NoExpr != "^[nN]" || d.YesStr != "oui" || d.NoStr != "non" {
		t.Fatalf("MessagesData=%+v ok=%v", d, ok)
	}
}

func TestCompileTimeListsAndMonetary(t *testing.T) {
	c := compileSource(t, "LC_TIME\nabday \"Su\";\"Mo\";\"Tu\";\"We\";\"Th\";\"Fr\";\"Sa\"\nd_fmt \"%d.%m.%Y\"\nEND LC_TIME\n"+
		"LC_MONETARY\ncurrency_symbol \"<dollar-sign>\"\nfrac_digits 2\nEND LC_MONETARY\n")
	day, ok := c.KeywordValues("LC_TIME", "abday")
	if !ok || len(day) != 7 || day[0] != "Su" || day[6] != "Sa" {
		t.Fatalf("abday=%v", day)
	}
	if v, _ := c.KeywordString("LC_TIME", "d_fmt"); v != "%d.%m.%Y" {
		t.Fatalf("d_fmt=%q", v)
	}
	if v, _ := c.KeywordString("LC_MONETARY", "currency_symbol"); v != "$" {
		t.Fatalf("currency_symbol=%q", v)
	}
	if k, _ := c.Keyword("LC_MONETARY", "frac_digits"); !k.Numeric || k.Values[0] != "2" {
		t.Fatalf("frac_digits=%+v", k)
	}
}

func TestCompileCtypeClassesAndCaseMaps(t *testing.T) {
	c := compileSource(t, "LC_CTYPE\nupper <A>;<B>\nlower <a>;<b>\ntoupper (<a>,<A>);(<b>,<B>)\nEND LC_CTYPE\n")
	if s, ok := c.Class("upper"); !ok || s != "AB" {
		t.Fatalf("upper=%q ok=%v", s, ok)
	}
	if s, _ := c.Class("lower"); s != "ab" {
		t.Fatalf("lower=%q", s)
	}
	if c.ToUpperFrom != "ab" || c.ToUpperTo != "AB" {
		t.Fatalf("toupper %q->%q", c.ToUpperFrom, c.ToUpperTo)
	}
	if c.Charmap != "ASCII" || c.MbCurMax != 1 {
		t.Fatalf("charmap=%q mb_cur_max=%d", c.Charmap, c.MbCurMax)
	}
}

func TestCompileRejectsUnresolvedCopy(t *testing.T) {
	src, err := ParseSource(strings.NewReader("LC_TIME\ncopy \"other\"\nEND LC_TIME\n"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Compile("xx_XX", src, DefaultCharmap()); err == nil {
		t.Fatal("Compile silently ignored an unresolved copy")
	}
}

func TestCompileRejectsUndefinedSymbol(t *testing.T) {
	src, err := ParseSource(strings.NewReader("LC_NUMERIC\ndecimal_point \"<absent>\"\nEND LC_NUMERIC\n"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Compile("xx_XX", src, DefaultCharmap()); err == nil {
		t.Fatal("Compile accepted an undefined symbolic name")
	}
}
