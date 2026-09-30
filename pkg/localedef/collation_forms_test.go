package localedef

import (
	"reflect"
	"strings"
	"testing"
)

func TestCollationRequiredForms(t *testing.T) {
	cm := &Charmap{CodeSet: "UTF-8", MinBytes: 1, MaxBytes: 1, Symbols: map[string][]byte{"nul": {0}, "a": {'a'}, "b": {'b'}, "bee": {'b'}, "c": {'c'}, "d": {'d'}, "max": {127}}}
	for _, tc := range []struct {
		name, body, order string
		warning           bool
		weights           [][]int
	}{
		{"range", "<a>\n...\n<d>\nUNDEFINED IGNORE", "abcd\x00\x7f", false, nil},
		{"initial", "...\n<c>\nUNDEFINED IGNORE", "abc\x00d\x7f", false, nil},
		{"trailing", "<b>\n...\nUNDEFINED IGNORE", "", false, nil}, // UNDEFINED is not a range endpoint.
		{"last", "UNDEFINED IGNORE\n<b>\n...", "\x00a\x7fbcd", false, nil},
		{"omitted", "<c>\n<a>", "ca\x00bd\x7f", true, nil},
		{"empty", "<a> ;\n<b> <a>;\nUNDEFINED IGNORE;IGNORE", "ab\x00cd\x7f", false, [][]int{{1}, {1}}},
		{"range-weights", "<a>\n... <a>;...\n<d>\nUNDEFINED IGNORE;IGNORE", "abcd\x00\x7f", false, [][]int{{1}, {1}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			src, err := ParseSource(strings.NewReader("LC_COLLATE\norder_start forward,position;backward,position\n" + tc.body + "\norder_end\nEND LC_COLLATE\n"))
			if err != nil {
				t.Fatal(err)
			}
			c, err := Compile("forms", src, cm)
			if tc.order == "" {
				if err == nil {
					t.Fatal("accepted non-character endpoint")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			var order strings.Builder
			for _, e := range c.Collation.Elements {
				order.WriteString(e.Text)
			}
			if order.String() != tc.order {
				t.Fatalf("order=%q want=%q", order.String(), tc.order)
			}
			if !reflect.DeepEqual(c.Collation.Position, []bool{true, true}) {
				t.Fatal(c.Collation.Position)
			}
			warnings := Validate(src, cm)
			if (len(warnings) > 0) != tc.warning {
				t.Fatalf("warnings=%v", warnings)
			}
			if tc.warning && !warnings[0].Warning {
				t.Fatal(warnings)
			}
			if tc.weights != nil && !reflect.DeepEqual(c.Collation.Elements[0].Weights, tc.weights) {
				t.Fatal(c.Collation.Elements[0].Weights)
			}
			if tc.name == "range-weights" && !reflect.DeepEqual(c.Collation.Elements[1].Weights, [][]int{{1}, {2}}) {
				t.Fatal(c.Collation.Elements[1])
			}
		})
	}
}

func TestCollationInvalidForms(t *testing.T) {
	for _, body := range []string{
		"order_start forward,backward\n<a>",
		"order_start forward;\n<a>",
		"order_start forward,\n<a>",
		"order_start forward position\n<a>",
		"order_start forward\n<z>\n...\n<a>",
		"order_start forward\n<a>\n...\n...\n<z>",
		"collating-symbol <s>\norder_start forward\n<s>\n...\n<z>",
		"collating-element <ch> from \"ch\"\norder_start forward\n<a>\n...\n<ch>",
		"order_start forward\n<a> ...",
		"order_start forward\n<a> ;",
	} {
		src, err := ParseSource(strings.NewReader("LC_COLLATE\n" + body + "\norder_end\nEND LC_COLLATE\n"))
		if err != nil {
			continue
		}
		if _, err = Compile("bad", src, nil); err == nil {
			t.Errorf("accepted %s", body)
		}
	}
}

func TestCollationUndefinedDefaults(t *testing.T) {
	for _, weights := range []string{"", " ;", " ...;...", " IGNORE;...", " <a>;<a>"} {
		src, err := ParseSource(strings.NewReader("LC_COLLATE\norder_start forward;forward\n<a>\nUNDEFINED" + weights + "\norder_end\nEND LC_COLLATE\n"))
		if err != nil {
			t.Fatal(err)
		}
		cm := &Charmap{Symbols: map[string][]byte{"a": {'a'}, "b": {'b'}, "c": {'c'}}}
		c, err := Compile("undef", src, cm)
		if err != nil {
			t.Fatal(err)
		}
		b, d := c.Collation.Elements[1], c.Collation.Elements[2]
		if weights == "" || weights == " ;" {
			if !reflect.DeepEqual(b.Weights, [][]int{{2}, {2}}) || !reflect.DeepEqual(d.Weights, [][]int{{2}, {3}}) {
				t.Fatalf("%q: %v %v", weights, b, d)
			}
		}
		if len(Validate(src, cm)) != 0 {
			t.Fatal("explicit UNDEFINED warned")
		}
	}
}

func TestCollationEmptyMiddleAndRangeAlias(t *testing.T) {
	src, err := ParseSource(strings.NewReader("LC_COLLATE\norder_start forward;position;backward,position\n<a>\n... <a>;;...\n<d> <bee>;;\nUNDEFINED IGNORE;IGNORE;IGNORE\norder_end\nEND LC_COLLATE\n"))
	if err != nil {
		t.Fatal(err)
	}
	cm := &Charmap{Symbols: map[string][]byte{"a": {'a'}, "b": {'b'}, "bee": {'b'}, "c": {'c'}, "d": {'d'}}}
	c, err := Compile("alias", src, cm)
	if err != nil {
		t.Fatal(err)
	}
	for i, want := range [][][]int{{{1}, {1}, {1}}, {{1}, {2}, {2}}, {{1}, {3}, {3}}, {{2}, {4}, {4}}} {
		if !reflect.DeepEqual(c.Collation.Elements[i].Weights, want) {
			t.Fatalf("%d: %v want %v", i, c.Collation.Elements[i].Weights, want)
		}
	}
}
