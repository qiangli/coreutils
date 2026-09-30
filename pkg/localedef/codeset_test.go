package localedef

import (
	"errors"
	"strings"
	"testing"
)

func TestTargetCharmapRanges(t *testing.T) {
	for _, tc := range []struct {
		target, encoding, want string
		limited                bool
	}{
		{"UTF-8", "<U007F>", "\u007f\u0080", false},
		{"UTF-8", "U00FF", "ÿĀ", false},
		{"ASCII", "<U007F>", "", true},
		{"UTF-8", "<UD7FF>", "", true},
		{"UTF-8", "<U0010FFFF>", "", true},
	} {
		t.Run(tc.target+tc.encoding, func(t *testing.T) {
			cm, err := ParseCharmapTarget(strings.NewReader("CHARMAP\n<ch01>...<ch02> "+tc.encoding+"\nEND CHARMAP\n"), tc.target)
			if tc.limited {
				if !errors.Is(err, ErrCodeset) {
					t.Fatalf("want codeset limit, got %v", err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if got := string(cm.Symbols["ch01"]) + string(cm.Symbols["ch02"]); got != tc.want {
				t.Errorf("range=%q want %q", got, tc.want)
			}
		})
	}
}

func TestTargetAliasesAndMetadata(t *testing.T) {
	for _, alias := range []string{"UTF-8", "utf8", "ascii", "US-ASCII", "ANSI_X3.4-1968", "iso646-us"} {
		cm, err := ParseCharmapTarget(strings.NewReader("<code_set_name> UCS\n<mb_cur_min> 2\n<mb_cur_max> 2\nCHARMAP\n<A> <U0041>\nEND CHARMAP\n"), alias)
		if err != nil {
			t.Fatal(err)
		}
		want, max := "ASCII", 1
		if strings.HasPrefix(strings.ToUpper(alias), "UTF") {
			want, max = "UTF-8", 4
		}
		if cm.CodeSet != want || cm.MinBytes != 1 || cm.MaxBytes != max || string(cm.Symbols["A"]) != "A" {
			t.Errorf("%s: %+v", alias, cm)
		}
	}
}

func TestMixedPositionAndNumericEncodings(t *testing.T) {
	cm, err := ParseCharmapTarget(strings.NewReader("CHARMAP\n<A> <U0041>\n<B> \\d066\n<eacute> <U00E9>\n<euro> \\xe2\\x82\\xac\nEND CHARMAP\n"), "UTF-8")
	if err != nil {
		t.Fatal(err)
	}
	for symbol, want := range map[string]string{"A": "A", "B": "B", "eacute": "é", "euro": "€"} {
		if got := string(cm.Symbols[symbol]); got != want {
			t.Errorf("%s=%q want %q", symbol, got, want)
		}
	}
}

func TestUTF8TargetNumericEncodingIsOneCharacter(t *testing.T) {
	for _, tc := range []struct {
		name, encoding, want string
		reject               bool
	}{
		{"two ASCII characters", `\x41\x42`, "", true},
		{"base and combining character", `\x65\xcc\x81`, "", true},
		{"ASCII", `\x41`, "A", false},
		{"NUL", `\x00`, "\x00", false},
		{"two-byte scalar", `\xc3\xa9`, "é", false},
		{"three-byte scalar", `\xe2\x82\xac`, "€", false},
		{"replacement scalar", `\xef\xbf\xbd`, "\ufffd", false},
		{"four-byte scalar", `\xf0\x9f\x98\x80`, "\U0001f600", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cm, err := ParseCharmapTarget(strings.NewReader("CHARMAP\n<custom> "+tc.encoding+"\nEND CHARMAP\n"), "UTF-8")
			if tc.reject {
				if !errors.Is(err, ErrCodeset) || cm != nil {
					t.Fatalf("want no charmap and codeset error, got charmap=%+v error=%v", cm, err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if got := string(cm.Symbols["custom"]); got != tc.want {
				t.Errorf("encoding=%q want %q", got, tc.want)
			}
		})
	}
}
