package localecmd

import (
	"strings"
	"testing"

	"github.com/qiangli/coreutils/pkg/locale"
)

// compileFixture writes a locale of our own into a private store and returns
// the environment that selects it. The decimal point is deliberately a
// character no real locale uses, so an assertion cannot pass by accident
// against host or built-in data.
func compileFixture(t *testing.T) []string {
	t.Helper()
	dir := t.TempDir()
	c := &locale.Compiled{Name: "xx_XX", Charmap: "ASCII", MbCurMin: 1, MbCurMax: 1}
	c.Set("LC_NUMERIC", "decimal_point", locale.Keyword{Values: []string{"!"}})
	c.Set("LC_NUMERIC", "thousands_sep", locale.Keyword{Values: []string{"_"}})
	c.Set("LC_NUMERIC", "grouping", locale.Keyword{Values: []string{"3"}, Numeric: true})
	c.Set("LC_MESSAGES", "yesexpr", locale.Keyword{Values: []string{"^[oO]"}})
	c.Set("LC_MESSAGES", "noexpr", locale.Keyword{Values: []string{"^[nN]"}})
	c.Set("LC_MESSAGES", "yesstr", locale.Keyword{Values: []string{"oui"}})
	c.Set("LC_MESSAGES", "nostr", locale.Keyword{Values: []string{"non"}})
	c.Set("LC_TIME", "d_fmt", locale.Keyword{Values: []string{"%d.%m.%Y"}})
	c.Set("LC_MONETARY", "currency_symbol", locale.Keyword{Values: []string{"#"}})
	c.Set("LC_MONETARY", "frac_digits", locale.Keyword{Values: []string{"2"}, Numeric: true})
	// A definition that had an LC_CTYPE section carries the category even
	// though its classes are not locale(1) keywords; that is what the
	// compiler records, so the fixture records it the same way.
	c.EnsureCategory("LC_CTYPE")
	c.Classes = map[string]string{"upper": "AB", "lower": "ab"}
	if err := locale.Save(dir, c); err != nil {
		t.Fatal(err)
	}
	return []string{"LOCPATH=" + dir, "LC_ALL=xx_XX"}
}

func TestCompiledLocaleKeywords(t *testing.T) {
	env := compileFixture(t)
	for _, tc := range []struct{ arg, want string }{
		{"decimal_point", `decimal_point="!"`},
		{"thousands_sep", `thousands_sep="_"`},
		{"grouping", "grouping=3"},
		{"yesexpr", `yesexpr="^[oO]"`},
		{"noexpr", `noexpr="^[nN]"`},
		{"d_fmt", `d_fmt="%d.%m.%Y"`},
		{"currency_symbol", `currency_symbol="#"`},
		{"frac_digits", "frac_digits=2"},
		{"charmap", `charmap="ASCII"`},
	} {
		t.Run(tc.arg, func(t *testing.T) {
			out, errb, code := runCmd(t, env, "-k", tc.arg)
			if code != 0 || strings.TrimSpace(out) != tc.want {
				t.Fatalf("locale -k %s = %q (%d) %q; want %q", tc.arg, out, code, errb, tc.want)
			}
		})
	}
}

// The whole category reads from the compiled locale too, not just single
// keyword queries.
func TestCompiledLocaleCategory(t *testing.T) {
	env := compileFixture(t)
	out, errb, code := runCmd(t, env, "-k", "LC_NUMERIC")
	if code != 0 {
		t.Fatalf("locale -k LC_NUMERIC failed: %d %q", code, errb)
	}
	for _, want := range []string{`decimal_point="!"`, `thousands_sep="_"`, "grouping=3"} {
		if !strings.Contains(out, want) {
			t.Fatalf("locale -k LC_NUMERIC = %q; missing %q", out, want)
		}
	}
}

// A compiled locale is listed by -a, since it is genuinely available here.
func TestCompiledLocaleListed(t *testing.T) {
	env := compileFixture(t)
	out, _, code := runCmd(t, env, "-a")
	if code != 0 || !strings.Contains(out, "xx_XX\n") {
		t.Fatalf("locale -a = %q (%d); want it to list xx_XX", out, code)
	}
}

// A category the compiled locale does not carry must not be answered from it:
// falling through to the existing behaviour is the documented contract.
func TestUncompiledCategoryKeepsHostBehaviour(t *testing.T) {
	dir := t.TempDir()
	c := &locale.Compiled{Name: "yy_YY", Charmap: "ASCII", MbCurMin: 1, MbCurMax: 1}
	c.Set("LC_NUMERIC", "decimal_point", locale.Keyword{Values: []string{"!"}})
	if err := locale.Save(dir, c); err != nil {
		t.Fatal(err)
	}
	env := []string{"LOCPATH=" + dir, "LC_ALL=yy_YY"}
	if out, _, code := runCmd(t, env, "-k", "decimal_point"); code != 0 || strings.TrimSpace(out) != `decimal_point="!"` {
		t.Fatalf("compiled LC_NUMERIC not used: %q (%d)", out, code)
	}
	// LC_TIME was never compiled for yy_YY, so it is refused by name rather
	// than answered with C's month names under a locale that did not define them.
	if _, errb, code := runCmd(t, env, "-k", "d_fmt"); code == 0 || !strings.Contains(errb, "yy_YY") {
		t.Fatalf("uncompiled LC_TIME was answered anyway: %d %q", code, errb)
	}
}

// Locales we did not compile are untouched by the store.
func TestUncompiledLocaleUnchanged(t *testing.T) {
	env := []string{"LOCPATH=" + t.TempDir(), "LC_ALL=POSIX"}
	out, _, code := runCmd(t, env, "-k", "decimal_point")
	if code != 0 || strings.TrimSpace(out) != `decimal_point="."` {
		t.Fatalf("POSIX decimal_point = %q (%d); want \".\"", out, code)
	}
}
