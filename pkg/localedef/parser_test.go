package localedef

import (
	"bytes"
	"compress/gzip"
	"os"
	"strings"
	"testing"
)

func TestCharmap(t *testing.T) {
	for _, tc := range []struct {
		name, source string
		want         []byte
	}{
		{"hex", "<code_set_name> TEST\n<mb_cur_min> 1\n<mb_cur_max> 2\nCHARMAP\n<X0>...<X9> \\x30\n<wide> \\xc2\\xa3\nEND CHARMAP\nWIDTH\n<wide> 1\nEND WIDTH\n", []byte{0x39}},
		{"decimal", "<escape_char> /\n<comment_char> %\nCHARMAP\n% comment\n<X0>...<X9> /d48\nEND CHARMAP\n", []byte{0x39}},
		{"octal", "CHARMAP\n<X9> \\071\nEND CHARMAP\n", []byte{0x39}},
		{"carry", "<mb_cur_min> 2\n<mb_cur_max> 2\nCHARMAP\n<X0>...<X9> \\x01\\xff\nEND CHARMAP\n", []byte{2, 8}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m, err := ParseCharmap(strings.NewReader(tc.source))
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(m.Symbols["X9"], tc.want) {
				t.Fatalf("symbols=%v", m.Symbols)
			}
		})
	}
}

func TestCharmapErrors(t *testing.T) {
	for _, tc := range []struct{ name, source, diag string }{
		{"missing end", "CHARMAP\n<X> \\x41\n", "line 1"},
		{"bad byte", "CHARMAP\n<X> \\d999\nEND CHARMAP\n", "line 2"},
		{"duplicate", "CHARMAP\n<X> \\x41\n<X> \\x42\nEND CHARMAP\n", "line 3"},
		{"reverse", "CHARMAP\n<X9>...<X0> \\x30\nEND CHARMAP\n", "line 2"},
		{"width", "<mb_cur_max> 0\nCHARMAP\nEND CHARMAP\n", "line 1"},
		{"overflow", "CHARMAP\n<X0>...<X9> \\xff\nEND CHARMAP\n", "line 2"},
		{"long encoding", "CHARMAP\n<X> \\x41\\x42\nEND CHARMAP\n", "line 2"},
		{"outside", "<X> \\x41\n", "line 1"},
		{"bad escape", "CHARMAP\n<X> \\xZ1\nEND CHARMAP\n", "line 2"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := ParseCharmap(strings.NewReader(tc.source))
			if err == nil || !strings.Contains(err.Error(), tc.diag) {
				t.Fatalf("got %v, want %s", err, tc.diag)
			}
		})
	}
}

func TestSource(t *testing.T) {
	src := "escape_char /\ncomment_char %\n% ignored /\nLC_CTYPE\nupper <A>;/\n <B>\ntolower (<A>,<a>);(<B>,<b>)\nEND LC_CTYPE\nLC_NUMERIC\ndecimal_point \"<period>\"\nthousands_sep \"%;/\"\"\ngrouping 3;3;-1\nEND LC_NUMERIC\nLC_TIME\ncopy \"time-base\"\nEND LC_TIME\nLC_COLLATE\norder_start forward\n<A>\n<B>\norder_end\nEND LC_COLLATE\nLC_MONETARY\ncopy \"C\"\nEND LC_MONETARY\nLC_MESSAGES\nyesexpr \"^[yY]\"\nnoexpr \"^[nN]\"\nEND LC_MESSAGES\n"
	m, err := ParseSource(strings.NewReader(src))
	if err != nil {
		t.Fatal(err)
	}
	if len(m.Sections) != 6 || m.Sections["LC_TIME"].Copy != "time-base" {
		t.Fatalf("model=%+v", m)
	}
	e := m.Sections["LC_CTYPE"].Entries[0]
	if e.Line != 5 || len(e.Values) != 3 || e.Values[2].Kind != Symbol || e.Values[2].Text != "B" {
		t.Fatalf("entry=%+v", e)
	}
	v := m.Sections["LC_NUMERIC"].Entries[0].Values[0]
	if v.Kind != String || len(v.Parts) != 1 || v.Parts[0].Kind != Symbol {
		t.Fatalf("string=%+v", v)
	}
	if got := m.Sections["LC_NUMERIC"].Entries[1].Values[0].Text; got != "%;\"" {
		t.Fatalf("escaped string=%q", got)
	}
}

func TestSourceErrors(t *testing.T) {
	for _, tc := range []struct{ name, source, diag string }{
		{"unterminated section", "LC_TIME\ncopy \"C\"\n", "line 1"},
		{"mismatch", "LC_TIME\nEND LC_NUMERIC\n", "line 2"},
		{"string", "LC_NUMERIC\ndecimal_point \".\nEND LC_NUMERIC\n", "line 2"},
		{"symbol", "LC_CTYPE\nupper <A\nEND LC_CTYPE\n", "line 2"},
		{"copy mixed", "LC_TIME\ncopy \"C\"\nabday \"Sun\"\nEND LC_TIME\n", "line 3"},
		{"copy missing", "LC_TIME\ncopy\nEND LC_TIME\n", "line 2"},
		{"unknown section", "LC_WRONG\nEND LC_WRONG\n", "line 1"},
		{"unknown keyword", "LC_NUMERIC\nwrong 3\nEND LC_NUMERIC\n", "line 2"},
		{"empty", "", "line 1"},
		{"nested", "LC_TIME\nLC_CTYPE\n", "line 2"},
		{"duplicate section", "LC_TIME\nEND LC_TIME\nLC_TIME\nEND LC_TIME\n", "line 3"},
		{"continuation", "LC_TIME\nabday \\\n", "line 2"},
		{"unbalanced", "LC_CTYPE\ntoupper (<a>,<A>\nEND LC_CTYPE\n", "line 2"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := ParseSource(strings.NewReader(tc.source))
			if err == nil || !strings.Contains(err.Error(), tc.diag) {
				t.Fatalf("got %v, want %s", err, tc.diag)
			}
		})
	}
}

func TestValidation(t *testing.T) {
	for _, tc := range []struct {
		cat, key string
		warning  bool
	}{{"LC_CTYPE", "upper", true}, {"LC_COLLATE", "<missing>", true}, {"LC_NUMERIC", "decimal_point", false}} {
		t.Run(tc.cat, func(t *testing.T) {
			line := tc.key + " <absent>"
			if tc.cat == "LC_NUMERIC" {
				line = tc.key + " \"<absent>\""
			}
			m, err := ParseSource(strings.NewReader(tc.cat + "\n" + line + "\nEND " + tc.cat + "\n"))
			if err != nil {
				t.Fatal(err)
			}
			ds := Validate(m, &Charmap{Symbols: map[string][]byte{}})
			if len(ds) == 0 || ds[0].Warning != tc.warning || ds[0].Line != 2 {
				t.Fatalf("diagnostics=%+v", ds)
			}
		})
	}
}

func TestSystemSource(t *testing.T) {
	f, err := os.Open("/usr/share/i18n/locales/en_US")
	if err != nil {
		t.Skip("system en_US unavailable")
	}
	defer f.Close()
	c, err := os.Open("/usr/share/i18n/charmaps/UTF-8.gz")
	if err != nil {
		t.Skip("system UTF-8 charmap unavailable")
	}
	defer c.Close()
	z, err := gzip.NewReader(c)
	if err != nil {
		t.Fatal(err)
	}
	defer z.Close()
	cm, err := ParseCharmap(z)
	if err != nil {
		t.Fatal(err)
	}
	if cm.CodeSet != "UTF-8" || len(cm.Symbols) < 128 {
		t.Fatalf("unexpected charmap: %s, %d symbols", cm.CodeSet, len(cm.Symbols))
	}
	m, err := ParseSource(f)
	if err != nil {
		t.Fatal(err)
	}
	if m.Sections["LC_TIME"] == nil {
		t.Fatal("missing LC_TIME")
	}
}

func TestCopiedCTypeAcceptsGlibcTransliterationExtension(t *testing.T) {
	source := "LC_CTYPE\ncopy \"i18n\"\ntranslit_start\ninclude \"translit_combining\";\"\"\ntranslit_end\nEND LC_CTYPE\n"
	m, err := ParseSource(strings.NewReader(source))
	if err != nil {
		t.Fatal(err)
	}
	if got := m.Sections["LC_CTYPE"].Copy; got != "i18n" {
		t.Fatalf("LC_CTYPE copy = %q, want i18n", got)
	}
	if _, err := ParseSource(strings.NewReader("LC_CTYPE\ncopy \"i18n\"\ntranslit_start\nEND LC_CTYPE\n")); err == nil {
		t.Fatal("accepted unterminated transliteration block")
	}
}

func TestSyntaxEdges(t *testing.T) {
	for _, tc := range []struct {
		name, input    string
		charmap, valid bool
	}{
		{"explicit default escape", "escape_char \\\nLC_TIME\ncopy \"C\"\nEND LC_TIME\n", false, true},
		{"charmap whitespace", "CHARMAP # begin\n<X> \\x41\nEND\tCHARMAP # end\n", true, true},
		{"empty continuation", "\\\n", false, false},
		{"bare decimal", "LC_NUMERIC\ndecimal_point\nEND LC_NUMERIC\n", false, false},
		{"numeric string", "LC_NUMERIC\ndecimal_point 4\nEND LC_NUMERIC\n", false, false},
		{"bad pair", "LC_CTYPE\ntolower (<A>;<a>)\nEND LC_CTYPE\n", false, false},
		{"collating declaration", "LC_COLLATE\ncollating-element <ch> from \"<c><h>\"\norder_start forward\n<ch>\norder_end\nEND LC_COLLATE\n", false, true},
		{"custom class", "LC_CTYPE\ncharclass \"vowel\"\nvowel <a>;<e>\nEND LC_CTYPE\n", false, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var err error
			if tc.charmap {
				_, err = ParseCharmap(strings.NewReader(tc.input))
			} else {
				_, err = ParseSource(strings.NewReader(tc.input))
			}
			if (err == nil) != tc.valid {
				t.Fatalf("valid=%v err=%v", tc.valid, err)
			}
		})
	}
}
