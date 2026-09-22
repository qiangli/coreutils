package localecmd

import (
	"reflect"
	"testing"
)

func TestParseHostLocaleValueGitBashTimeValues(t *testing.T) {
	for _, tc := range []struct {
		name  string
		input string
		kind  valueKind
		want  []string
	}{
		{name: "empty era", input: "", kind: kindString, want: []string{""}},
		{name: "quoted semicolons", input: `"Sun;Mon;Tue";"Wed;Thu;Fri;Sat"`, kind: kindStringList, want: []string{"Sun;Mon;Tue", "Wed;Thu;Fri;Sat"}},
		{name: "hex escape", input: `"Sun\x3bMon"`, kind: kindString, want: []string{"Sun;Mon"}},
		{name: "number", input: "7", kind: kindNumber, want: []string{"7"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, kind, err := parseHostLocaleValue(tc.input)
			if err != nil {
				t.Fatalf("parseHostLocaleValue(%q): %v", tc.input, err)
			}
			if kind != tc.kind || !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("parseHostLocaleValue(%q) = %#v, %v; want %#v, %v", tc.input, got, kind, tc.want, tc.kind)
			}
		})
	}
}

func TestParseHostLocaleKeywordsGitBashTimeCategory(t *testing.T) {
	out := "abday=\"Sun;Mon;Tue\";\"Wed;Thu;Fri;Sat\"\n" +
		"era=\n" +
		"alt_digits=\n"
	got, err := parseHostLocaleKeywords("LC_TIME", out)
	if err != nil {
		t.Fatalf("parse Git Bash LC_TIME output: %v", err)
	}
	want := []keyword{
		{Name: "abday", Category: "LC_TIME", Kind: kindStringList, Values: []string{"Sun;Mon;Tue", "Wed;Thu;Fri;Sat"}},
		{Name: "era", Category: "LC_TIME", Kind: kindString, Values: []string{""}},
		{Name: "alt_digits", Category: "LC_TIME", Kind: kindString, Values: []string{""}},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("parsed Git Bash LC_TIME output = %#v, want %#v", got, want)
	}
}

func TestParseHostLocaleValueRejectsMalformedOutput(t *testing.T) {
	for _, input := range []string{`"unterminated`, `"ok"tail`, `"bad\q"`, `"bad\x0"`, `"ok";`} {
		if _, _, err := parseHostLocaleValue(input); err == nil {
			t.Errorf("parseHostLocaleValue(%q) succeeded; want malformed output error", input)
		}
	}
}
