package printfcmd

import (
	"strings"
	"testing"
)

// TestPrintfVSCNumericResiduals pins the exact operands and formats from the
// certification failures. The implementation is clean-room from the POSIX
// printf rule that floating operands are converted as by strtod: an optional
// sign applies to NaN, hexadecimal constants are accepted, and LC_NUMERIC
// selects both the input and output radix character.
func TestPrintfVSCNumericResiduals(t *testing.T) {
	assert := func(t *testing.T, env []string, format, arg, want string) {
		t.Helper()
		out, errb, code := runToolEnv(t, env, format, arg)
		if code != 0 || errb != "" || out != want {
			t.Fatalf("printf %q %q env=%q = (%q, %q, %d), want (%q, \"\", 0)",
				format, arg, env, out, errb, code, want)
		}
	}

	// TP37/TP53/TP77: signed NaN, field width, left justification, and
	// the rule that zero padding is ignored for a non-finite value.
	for _, conv := range "aefgAEFG" {
		want := "  -nan"
		left := "-nan  "
		if conv >= 'A' && conv <= 'Z' {
			want, left = "  -NAN", "-NAN  "
		}
		assert(t, nil, "%6"+string(conv), "-NAN", want)
		assert(t, nil, "%-6"+string(conv), "-NAN", left)
		assert(t, nil, "%-06"+string(conv), "-NAN", left)
		assert(t, nil, "%06"+string(conv), "-NAN", want)
	}

	// TP62/TP63/TP66: the exact locale and decimal-comma operand selected
	// by the certification image, through each POSIX precedence input.
	for _, env := range [][]string{
		{"LANG=POSIX", "LC_NUMERIC=de_DE.iso88591"},
		{"LANG=de_DE.iso88591"},
		{"LANG=POSIX", "LC_NUMERIC=POSIX", "LC_ALL=de_DE.iso88591"},
	} {
		assert(t, env, "%.1f\n", "12345678,9", "12345678,9\n")
	}

	// TP74 (suite assertion 75): a hexadecimal floating operand without
	// an explicit p exponent, in both signs and all eight conversions.
	positive := map[string]string{
		"%.6a": "0x1.348000p+10", "%.6A": "0X1.348000P+10",
		"%e": "1.234000e+03", "%E": "1.234000E+03",
		"%f": "1234.000000", "%F": "1234.000000",
		"%g": "1234", "%G": "1234",
	}
	for format, want := range positive {
		assert(t, nil, format, "0x4d2", want)
		assert(t, nil, format, "-0x4d2", "-"+want)
	}

	// TP76: an explicit plus sign on NaN remains a valid strtod subject
	// sequence and does not appear in the default rendering.
	for _, conv := range "aefgAEFG" {
		want := "nan"
		if conv >= 'A' && conv <= 'Z' {
			want = "NAN"
		}
		assert(t, nil, "%"+string(conv), "+NAN", want)
	}
}

func TestPrintfLCNumericRadix(t *testing.T) {
	cases := []struct {
		name string
		env  []string
		arg  string
		fmt  string
		want string
	}{
		{"lc-numeric", []string{"LANG=POSIX", "LC_NUMERIC=de_DE.iso88591"}, "1234,5", "%.1f", "1234,5"},
		{"lang", []string{"LANG=de_DE.iso88591"}, "1234,5", "%.1f", "1234,5"},
		{"lc-all", []string{"LANG=POSIX", "LC_NUMERIC=POSIX", "LC_ALL=de_DE.iso88591"}, "1234,5", "%.1f", "1234,5"},
		{"lc-all-overrides", []string{"LANG=de_DE.iso88591", "LC_NUMERIC=de_DE.iso88591", "LC_ALL=POSIX"}, "1234.5", "%.1f", "1234.5"},
		{"empty-lc-all", []string{"LANG=POSIX", "LC_NUMERIC=de_DE.iso88591", "LC_ALL="}, "1234,5", "%.1f", "1234,5"},
		{"exponent", []string{"LC_NUMERIC=de_DE.UTF-8"}, "1234,5", "%.1e", "1,2e+03"},
		{"hex", []string{"LC_NUMERIC=de_DE.UTF-8"}, "0x1,4p+0", "%.1a", "0x1,4p+0"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			out, errb, code := runToolEnv(t, tc.env, tc.fmt, tc.arg)
			if code != 0 || errb != "" || out != tc.want {
				t.Fatalf("printf locale env=%q = (%q, %q, %d), want %q", tc.env, out, errb, code, tc.want)
			}
		})
	}
}

// These cases pin the POSIX strtod subject sequence that printf's
// floating-point ARGUMENTs are converted with. Two forms Go's
// strconv.ParseFloat rejects outright have to be recognized here, and both were
// live conformance failures (VSC-PCTS printf assertions 37, 53, 75, 77 and 78):
// a *signed* NaN, and a hexadecimal constant with no binary-exponent part.

// TestPrintfSignedNaN: C strtod applies the optional sign to NAN and printf
// renders the result as "-nan"/"-NAN"; the spelling is matched
// case-insensitively, and a leading '+' leaves the sign bit clear.
func TestPrintfSignedNaN(t *testing.T) {
	lower := []string{"a", "e", "f", "g"}
	upper := []string{"A", "E", "F", "G"}

	for _, arg := range []string{"NAN", "+NAN", "nan", "+nan", "Nan", "+nAn"} {
		for _, conv := range lower {
			if out, _, code := runTool(t, "%"+conv, arg); out != "nan" || code != 0 {
				t.Errorf("printf %%%s %q = (%q, code=%d), want (\"nan\", 0)", conv, arg, out, code)
			}
		}
		for _, conv := range upper {
			if out, _, code := runTool(t, "%"+conv, arg); out != "NAN" || code != 0 {
				t.Errorf("printf %%%s %q = (%q, code=%d), want (\"NAN\", 0)", conv, arg, out, code)
			}
		}
	}
	for _, arg := range []string{"-NAN", "-nan", "-Nan", "-nAn"} {
		for _, conv := range lower {
			if out, _, code := runTool(t, "%"+conv, arg); out != "-nan" || code != 0 {
				t.Errorf("printf %%%s %q = (%q, code=%d), want (\"-nan\", 0)", conv, arg, out, code)
			}
		}
		for _, conv := range upper {
			if out, _, code := runTool(t, "%"+conv, arg); out != "-NAN" || code != 0 {
				t.Errorf("printf %%%s %q = (%q, code=%d), want (\"-NAN\", 0)", conv, arg, out, code)
			}
		}
	}
}

// TestPrintfNonFiniteFieldWidth: width applies to a signed NaN or infinity and
// the '0' flag is ignored (padding is always spaces), while '-' left-justifies.
func TestPrintfNonFiniteFieldWidth(t *testing.T) {
	cases := []struct {
		args []string
		want string
	}{
		{[]string{"%6f", "-NAN"}, "  -nan"},
		{[]string{"%06f", "-NAN"}, "  -nan"},
		{[]string{"%06F", "-NAN"}, "  -NAN"},
		{[]string{"%06a", "NAN"}, "   nan"},
		{[]string{"%-6f|", "-NAN"}, "-nan  |"},
		{[]string{"%-06g|", "-NAN"}, "-nan  |"},
		{[]string{"%-06G|", "-NAN"}, "-NAN  |"},
		{[]string{"%010e", "-INF"}, "      -inf"},
		{[]string{"%010E", "INF"}, "       INF"},
	}
	for _, c := range cases {
		out, errb, code := runTool(t, c.args...)
		if out != c.want || code != 0 {
			t.Errorf("printf %v = (%q, err=%q, code=%d), want (%q, 0)", c.args, out, errb, code, c.want)
		}
	}
}

// TestPrintfHexFloatOperand: the binary-exponent part of a hexadecimal operand
// is optional in POSIX (Go requires it), so "0x4d2" converts to 1234 rather
// than stopping after the leading "0". The spellings below are the equivalent
// hexadecimal renderings of that same value.
func TestPrintfHexFloatOperand(t *testing.T) {
	for _, arg := range []string{"0x1.348p+10", "+0X02.69p9", "0x4d2", "+0x4D2P0", "0X9a4.P-1"} {
		cases := []struct{ format, want string }{
			{"%.6a", "0x1.348000p+10"},
			{"%.6A", "0X1.348000P+10"},
			{"%e", "1.234000e+03"},
			{"%f", "1234.000000"},
			{"%g", "1234"},
		}
		for _, c := range cases {
			out, errb, code := runTool(t, c.format, arg)
			if out != c.want || code != 0 {
				t.Errorf("printf %s %q = (%q, err=%q, code=%d), want (%q, 0)", c.format, arg, out, errb, code, c.want)
			}
		}
	}
	// The sign belongs to the whole subject sequence.
	for _, arg := range []string{"-0x4d2", "-0x1.348p+10", "-0X9a4.P-1"} {
		if out, _, code := runTool(t, "%f", arg); out != "-1234.000000" || code != 0 {
			t.Errorf("printf %%f %q = (%q, code=%d), want (\"-1234.000000\", 0)", arg, out, code)
		}
	}
}

// TestPrintfStrtodSubjectSequence: only the leading subject sequence converts,
// so trailing bytes raise "value not completely converted" while the value
// already scanned is still formatted — and a truncated spelling converts as far
// as it legally can ("infin" is "inf" plus leftovers, a bare "0x" is "0" plus
// leftovers, an "e" with no digits is not part of the token).
func TestPrintfStrtodSubjectSequence(t *testing.T) {
	cases := []struct {
		args   []string
		want   string
		code   int
		errHas string
	}{
		{[]string{"%f", "infinity"}, "inf", 0, ""},
		{[]string{"%f", "-inFINitY"}, "-inf", 0, ""},
		{[]string{"%f", "infin"}, "inf", 1, "not completely converted"},
		{[]string{"%f", "0x"}, "0.000000", 1, "not completely converted"},
		{[]string{"%f", "1e"}, "1.000000", 1, "not completely converted"},
		{[]string{"%f", "1e+"}, "1.000000", 1, "not completely converted"},
		{[]string{"%f", "0x1p"}, "1.000000", 1, "not completely converted"},
		{[]string{"%f", "1.5xyz"}, "1.500000", 1, "not completely converted"},
		{[]string{"%f", "nanny"}, "nan", 1, "not completely converted"},
		{[]string{"%f", "-nan(1_a)"}, "-nan", 0, ""},
		{[]string{"%f", "-nan("}, "-nan", 1, "not completely converted"},
		{[]string{"%f", "  -2.5"}, "-2.500000", 0, ""},
		{[]string{"%f", "."}, "0.000000", 1, "expected a numeric value"},
		{[]string{"%f", "x"}, "0.000000", 1, "expected a numeric value"},
		{[]string{"%f", "5."}, "5.000000", 0, ""},
		{[]string{"%f", ".5"}, "0.500000", 0, ""},
		{[]string{"%f", "1e999"}, "inf", 1, "out of range"},
	}
	for _, c := range cases {
		out, errb, code := runTool(t, c.args...)
		if out != c.want || code != c.code {
			t.Errorf("printf %v = (%q, code=%d), want (%q, %d)", c.args, out, code, c.want, c.code)
		}
		if c.errHas != "" && !strings.Contains(errb, c.errHas) {
			t.Errorf("printf %v stderr=%q, want contains %q", c.args, errb, c.errHas)
		}
		if c.errHas == "" && errb != "" {
			t.Errorf("printf %v unexpected stderr=%q", c.args, errb)
		}
	}
}
