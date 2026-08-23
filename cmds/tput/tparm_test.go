package tputcmd

import "testing"

func nums(vs ...int) []value {
	out := make([]value, len(vs))
	for i, v := range vs {
		out[i] = numValue(v)
	}
	return out
}

// The engine is a stack machine, so every directive is tested against the
// value it leaves behind rather than only through a whole capability string.
func TestTparmDirectives(t *testing.T) {
	for _, c := range []struct {
		name   string
		in     string
		params []value
		want   string
	}{
		{"literal text passes through", "hello", nil, "hello"},
		{"escaped percent", "100%%", nil, "100%"},
		{"push and print a parameter", "%p1%d", nums(42), "42"},
		{"parameters are positional", "%p2%d,%p1%d", nums(7, 9), "9,7"},
		{"a missing parameter reads as zero", "%p3%d", nums(1), "0"},

		// %i is the reason cup works: terminfo counts from 0, ANSI from 1.
		{"increment affects later pushes", "%i%p1%d;%p2%d", nums(5, 10), "6;11"},
		{"increment touches only the first two", "%i%p3%d", nums(1, 1, 1), "1"},
		{"cup as real entries write it", "\x1b[%i%p1%d;%p2%dH", nums(5, 10), "\x1b[6;11H"},

		{"character output is a raw byte", "%p1%c", nums(65), "A"},
		{"character output above ASCII stays one byte", "%p1%c", nums(0xB5), "\xb5"},
		{"string output", "%p1%s", []value{strValue("hi")}, "hi"},
		{"a number printed as a string", "%p1%s", nums(12), "12"},

		{"integer constant", "%{65}%c", nil, "A"},
		{"negative integer constant", "%{-3}%d", nil, "-3"},
		{"character constant", "%'A'%d", nil, "65"},
		{"character constant then add", "%p1%'@'%+%c", nums(3), "C"},

		{"addition", "%p1%p2%+%d", nums(3, 4), "7"},
		{"subtraction", "%p1%p2%-%d", nums(10, 4), "6"},
		{"multiplication", "%p1%p2%*%d", nums(6, 7), "42"},
		{"division", "%p1%p2%/%d", nums(20, 6), "3"},
		{"modulo", "%p1%p2%m%d", nums(20, 6), "2"},
		{"division by zero yields zero", "%p1%p2%/%d", nums(20, 0), "0"},
		{"modulo by zero yields zero", "%p1%p2%m%d", nums(20, 0), "0"},
		{"bitwise and", "%p1%p2%&%d", nums(12, 10), "8"},
		{"bitwise or", "%p1%p2%|%d", nums(12, 10), "14"},
		{"bitwise xor", "%p1%p2%^%d", nums(12, 10), "6"},
		{"complement", "%p1%~%d", nums(0), "-1"},

		{"equality", "%p1%p2%=%d", nums(4, 4), "1"},
		{"inequality reads as zero", "%p1%p2%=%d", nums(4, 5), "0"},
		{"greater than", "%p1%p2%>%d", nums(5, 4), "1"},
		{"less than", "%p1%p2%<%d", nums(5, 4), "0"},
		{"logical and", "%p1%p2%A%d", nums(1, 0), "0"},
		{"logical or", "%p1%p2%O%d", nums(1, 0), "1"},
		{"logical not", "%p1%!%d", nums(0), "1"},

		{"string length", "%p1%l%d", []value{strValue("abcd")}, "4"},

		{"dynamic variable round trip", "%p1%Pa%ga%d", nums(9), "9"},
		{"static variable round trip", "%p1%PZ%gZ%d", nums(8), "8"},
		{"a variable survives other work", "%{3}%Pa%{99}%d%ga%d", nil, "993"},

		// Conditionals.
		{"then branch taken", "%?%p1%tyes%eno%;", nums(1), "yes"},
		{"else branch taken", "%?%p1%tyes%eno%;", nums(0), "no"},
		{"conditional without else", "%?%p1%tyes%;!", nums(0), "!"},
		{"else-if chain first arm", "%?%p1%tA%e%p2%tB%eC%;", nums(1, 0), "A"},
		{"else-if chain second arm", "%?%p1%tA%e%p2%tB%eC%;", nums(0, 1), "B"},
		{"else-if chain final arm", "%?%p1%tA%e%p2%tB%eC%;", nums(0, 0), "C"},
		{"nested conditional inside a taken arm", "%?%p1%t[%?%p2%tX%eY%;]%eZ%;", nums(1, 0), "[Y]"},
		{"nested conditional inside a skipped arm", "%?%p1%t[%?%p2%tX%eY%;]%eZ%;", nums(0, 1), "Z"},
		{"a brace constant inside a skipped arm is not a directive", "%?%p1%t%{59}%c%eno%;", nums(0), "no"},

		// Formatted conversions.
		{"zero padded width", "%p1%02d", nums(7), "07"},
		{"width", "%p1%3d", nums(7), "  7"},
		{"hex", "%p1%x", nums(255), "ff"},
		{"upper hex with width", "%p1%04X", nums(255), "00FF"},
		{"octal", "%p1%o", nums(8), "10"},
		{"string precision", "%p1%.2s", []value{strValue("abcdef")}, "ab"},
		{"left justified string needs the colon", "%p1%:-4s|", []value{strValue("ab")}, "ab  |"},
		// A bare "-" after "%" is subtraction, never a printf flag — so this
		// pops two values and the rest is literal text, exactly as the grammar says.
		{"a bare minus is still subtraction", "%{9}%{4}%-%d", nil, "5"},
		// The ':' exists only so a leading '-' or '+' is read as a printf flag
		// rather than as the subtraction/addition operator.
		{"colon lets a minus be a flag", "%p1%:-4d|", nums(7), "7   |"},
		{"colon lets a plus be a flag", "%p1%:+d", nums(7), "+7"},
		{"a bare plus is still addition", "%p1%p2%+%d", nums(2, 3), "5"},

		// The 256-colour setaf every modern entry ships: three-way branch,
		// arithmetic, and a nested comparison in one string.
		{"xterm setaf below 8", xtermSetaf, nums(3), "\x1b[33m"},
		{"xterm setaf in the aixterm range", xtermSetaf, nums(9), "\x1b[91m"},
		{"xterm setaf in the 256 range", xtermSetaf, nums(200), "\x1b[38;5;200m"},
		{"xterm setab in the 256 range", xtermSetab, nums(200), "\x1b[48;5;200m"},
	} {
		t.Run(c.name, func(t *testing.T) {
			got, err := tparm(c.in, c.params)
			if err != nil {
				t.Fatalf("tparm(%q) error: %v", c.in, err)
			}
			if got != c.want {
				t.Errorf("tparm(%q) = %q, want %q", c.in, got, c.want)
			}
		})
	}
}

// A malformed capability string must say so. Silent tolerance is how a
// mistyped entry ends up looking like it worked.
func TestTparmRejectsMalformedInput(t *testing.T) {
	for _, c := range []struct{ name, in string }{
		{"trailing percent", "abc%"},
		{"unterminated integer constant", "%{12"},
		{"unterminated character constant", "%'A"},
		{"parameter without a digit", "%pX"},
		{"variable without a name", "%g"},
		{"non-letter variable name", "%P1"},
		{"unknown directive", "%Q"},
		{"unterminated conditional", "%?%{0}%tyes"},
		{"conversion specification with no conversion", "%p1%12"},
	} {
		t.Run(c.name, func(t *testing.T) {
			if got, err := tparm(c.in, nums(1)); err == nil {
				t.Errorf("tparm(%q) = %q, want an error", c.in, got)
			}
		})
	}
}

// Padding is an instruction to the output driver, not text. It is removed;
// anything that only looks like padding is not.
func TestStripPadding(t *testing.T) {
	for _, c := range []struct{ name, in, want string }{
		{"simple delay", "\x1b[K$<5>", "\x1b[K"},
		{"fractional delay", "a$<12.5>b", "ab"},
		{"per-line delay", "a$<5*>b", "ab"},
		{"mandatory delay", "a$<5/>b", "ab"},
		{"both delay modifiers", "a$<5*/>b", "ab"},
		{"several delays", "$<1>x$<2>y$<3>", "xy"},
		{"no delay", "plain", "plain"},
		{"dollar alone", "cost $5", "cost $5"},
		{"unterminated delay is literal", "a$<5b", "a$<5b"},
		{"non-numeric body is literal", "a$<abc>b", "a$<abc>b"},
		{"empty body is literal", "a$<>b", "a$<>b"},
	} {
		t.Run(c.name, func(t *testing.T) {
			if got := stripPadding(c.in); got != c.want {
				t.Errorf("stripPadding(%q) = %q, want %q", c.in, got, c.want)
			}
		})
	}
}

// A command-line operand that reads as a number must arrive as a number: the
// arithmetic in a capability like cup would otherwise silently compute on zero.
func TestParseParams(t *testing.T) {
	got := parseParams([]string{"5", "-3", "abc", "0x10", ""})
	want := []value{numValue(5), numValue(-3), strValue("abc"), strValue("0x10"), strValue("")}
	if len(got) != len(want) {
		t.Fatalf("got %d params, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("param %d = %+v, want %+v", i, got[i], want[i])
		}
	}
	if n := len(parseParams(make([]string, 20))); n != 9 {
		t.Errorf("parseParams kept %d operands, want the 9 the format can address", n)
	}
}
