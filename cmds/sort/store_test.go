package sortcmd

import (
	"strings"
	"testing"

	"github.com/qiangli/coreutils/pkg/locale"
)

// A locale compiled by our own localedef(1) feeds sort -n. The radix is a
// character no real locale uses, so the assertion cannot pass by accident
// against host data or the carried de_DE tables.
func TestNumericSortUsesCompiledLocale(t *testing.T) {
	store := t.TempDir()
	c := &locale.Compiled{Name: "xx_XX", Charmap: "ASCII", MbCurMin: 1, MbCurMax: 1}
	c.Set("LC_NUMERIC", "decimal_point", locale.Keyword{Values: []string{"!"}})
	c.Set("LC_NUMERIC", "thousands_sep", locale.Keyword{Values: []string{"_"}})
	if err := locale.Save(store, c); err != nil {
		t.Fatal(err)
	}
	env := []string{"LOCPATH=" + store, "LC_NUMERIC=xx_XX", "LC_COLLATE=C"}
	for _, tc := range []struct {
		name, in, want string
	}{
		{"radix", "1!20\n-1!2\n", "-1!2\n1!20\n"},
		{"thousands separator", "1_000!5\n900\n", "900\n1_000!5\n"},
		// A period is NOT the radix in this locale, so "1.3" has no fractional
		// part at all and sorts as 1.
		{"period is not the radix", "1!9\n1.3\n", "1.3\n1!9\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, errb, code := runToolEnv(t, t.TempDir(), env, tc.in, "-n")
			if code != 0 || out != tc.want {
				t.Fatalf("sort -n = %q (%d) %q; want %q", out, code, errb, tc.want)
			}
		})
	}
}

// A locale in the store that carries no LC_NUMERIC must not silently become C:
// sort keeps refusing the name, exactly as it did before the store existed.
func TestNumericSortRefusesLocaleWithoutNumeric(t *testing.T) {
	store := t.TempDir()
	c := &locale.Compiled{Name: "yy_YY", Charmap: "ASCII", MbCurMin: 1, MbCurMax: 1}
	c.Set("LC_TIME", "d_fmt", locale.Keyword{Values: []string{"%d.%m.%Y"}})
	if err := locale.Save(store, c); err != nil {
		t.Fatal(err)
	}
	env := []string{"LOCPATH=" + store, "LC_NUMERIC=yy_YY", "LC_COLLATE=C"}
	_, errb, code := runToolEnv(t, t.TempDir(), env, "1,2\n", "-n")
	if code != 2 || !strings.Contains(errb, "yy_YY") {
		t.Fatalf("sort accepted a locale with no LC_NUMERIC: %d %q", code, errb)
	}
}

func TestNumericSortCompiledUTF8OverridesCarriedDefault(t *testing.T) {
	store := t.TempDir()
	c := &locale.Compiled{Name: "xx_XX.UTF-8", Charmap: "UTF-8", MbCurMin: 1, MbCurMax: 4}
	c.Set("LC_NUMERIC", "decimal_point", locale.Keyword{Values: []string{"!"}})
	if err := locale.Save(store, c); err != nil {
		t.Fatal(err)
	}
	env := []string{"LOCPATH=" + store, "LC_NUMERIC=xx_XX.UTF-8", "LC_COLLATE=C"}
	out, errb, code := runToolEnv(t, t.TempDir(), env, "1!9\n1!20\n", "-n")
	if code != 0 || out != "1!9\n1!20\n" {
		t.Fatalf("sort -n = %q (%d) %q; want compiled radix ordering", out, code, errb)
	}
}
