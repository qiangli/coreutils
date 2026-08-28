package exprcmd

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"testing"

	"github.com/qiangli/coreutils/tool"
)

func checkExpr(t *testing.T, wantOut string, wantCode int, args ...string) {
	t.Helper()
	var out, err bytes.Buffer
	rc := &tool.RunContext{Ctx: context.Background(), Stdio: tool.Stdio{Out: &out, Err: &err, In: strings.NewReader("")}}
	if code := run(rc, args); code != wantCode || out.String() != wantOut {
		t.Errorf("expr %q = (code=%d, out=%q, err=%q), want (code=%d, out=%q)", args, code, out.String(), err.String(), wantCode, wantOut)
	}
}

func TestExprArithmetic(t *testing.T) {
	var out, err bytes.Buffer
	code := run(&tool.RunContext{Ctx: context.Background(), Stdio: tool.Stdio{Out: &out, Err: &err, In: strings.NewReader("")}}, []string{"1", "+", "2", "*", "3"})
	if code != 0 || out.String() != "7\n" {
		t.Fatalf("code=%d out=%q err=%s", code, out.String(), err.String())
	}
}

func TestExprMatch(t *testing.T) {
	var out, err bytes.Buffer
	code := run(&tool.RunContext{Ctx: context.Background(), Stdio: tool.Stdio{Out: &out, Err: &err, In: strings.NewReader("")}}, []string{"abc123", ":", "[a-z]*\\([0-9]*\\)"})
	if code != 0 || out.String() != "123\n" {
		t.Fatalf("code=%d out=%q err=%s", code, out.String(), err.String())
	}
}

func TestExprRegexpAtAdvertisedLimit(t *testing.T) {
	checkExpr(t, "255\n", 0, strings.Repeat("a", 255), ":", `a\{255\}`)
	checkExpr(t, "", 2, "a", ":", `a\{256\}`)
}

func TestExprCapturedRegexpAtAdvertisedLimit(t *testing.T) {
	// A subexpression repeated through RE_DUP_MAX participates in the match,
	// so expr returns the captured text rather than the null string.  This
	// complements the match-length boundary above by pinning capture output.
	operand := strings.Repeat("a", 255)
	checkExpr(t, operand+"\n", 0, operand, ":", `\(a\{1,255\}\)`)
}

func TestExprHelpVersionAliases(t *testing.T) {
	for _, args := range [][]string{{"--help"}, {"-h"}, {"--version"}, {"-V"}} {
		var out, err bytes.Buffer
		code := run(&tool.RunContext{Ctx: context.Background(), Stdio: tool.Stdio{Out: &out, Err: &err, In: strings.NewReader("")}}, args)
		if code != 0 || err.String() != "" || out.String() == "" {
			t.Fatalf("expr %v: code=%d out=%q err=%q", args, code, out.String(), err.String())
		}
		if args[0] == "--help" && (!strings.Contains(out.String(), "--help") || !strings.Contains(out.String(), "--version")) {
			t.Fatalf("expr help missing long options: %q", out.String())
		}
	}
}

func TestExprPOSIXArithmeticAndComparison(t *testing.T) {
	checkExpr(t, "7\n", 0, "1", "+", "2", "*", "3")
	checkExpr(t, "9223372036854775808\n", 0, "9223372036854775807", "+", "1")
	checkExpr(t, "18446744073709551616\n", 0, "4294967296", "*", "4294967296")
	checkExpr(t, "-2\n", 0, "-7", "/", "3")
	checkExpr(t, "-1\n", 0, "-7", "%", "3")
	checkExpr(t, "1\n", 0, "0000000000000000000002", ">", "1")
	checkExpr(t, "1\n", 0, "01", "=", "1")
	// Division/modulo by zero is a well-formed expression that fails at
	// evaluation: GNU expr reports EXPR_FAILURE and POSIX mandates an exit
	// status greater than 2 ("an error occurred"), distinct from the exit 2
	// used for a syntactically invalid expression.
	checkExpr(t, "", 3, "1", "/", "0")
	checkExpr(t, "", 3, "1", "%", "0")
	checkExpr(t, "", 3, "5", "+", "3", "*", "0", "/", "0")
	// A genuinely invalid expression still exits 2.
	checkExpr(t, "", 2, "1", "+")
	checkExpr(t, "", 2, "3.5", "+", "1")
}

func TestExprPOSIXBooleanAndExitStatus(t *testing.T) {
	checkExpr(t, "2\n", 0, "2", "&", "3")
	checkExpr(t, "0\n", 1, "0", "&", "3")
	checkExpr(t, "1\n", 0, "1", "|", "2")
	checkExpr(t, "2\n", 0, "0", "|", "2")
	checkExpr(t, "0\n", 1, "", "|", "")
	checkExpr(t, "0\n", 1, "0")
	checkExpr(t, "-0\n", 1, "-0")
	checkExpr(t, "value\n", 0, "value")
}

func TestExprPOSIXOperandsStdinAndDiagnostics(t *testing.T) {
	var out, errb bytes.Buffer
	rc := &tool.RunContext{
		Ctx:   context.Background(),
		Stdio: tool.Stdio{In: strings.NewReader("999"), Out: &out, Err: &errb},
	}
	code := run(rc, []string{"--", "-1", "+", "2"})
	if code != 0 || out.String() != "1\n" || errb.String() != "" {
		t.Fatalf("expr -- -1 + 2: code=%d out=%q err=%q", code, out.String(), errb.String())
	}

	out.Reset()
	errb.Reset()
	code = run(rc, nil)
	if code != 2 || out.String() != "" || !strings.Contains(errb.String(), "missing operand") {
		t.Fatalf("expr with no operands: code=%d out=%q err=%q", code, out.String(), errb.String())
	}
}

func TestExprLeadingPlusQuotesKeyword(t *testing.T) {
	checkExpr(t, "length\n", 0, "+", "length")
	checkExpr(t, "match\n", 0, "+", "match")
	checkExpr(t, "0\n", 1, "+", "0")
	checkExpr(t, "+\n", 0, "+", "+")
}

func TestExprPOSIXMatchAndStringFunctions(t *testing.T) {
	checkExpr(t, "ab\n", 0, "abab", ":", `\(ab\)\1`)
	checkExpr(t, "3\n", 0, "abc123", ":", "[[:alpha:]]*")
	checkExpr(t, "\n", 1, "abc", ":", `a\(z\)`)
	checkExpr(t, "3\n", 0, "length", "éx")
	checkExpr(t, "bc\n", 0, "substr", "abc", "2", "5")
	checkExpr(t, "abc\n", 0, "substr", "abc", "1", "9223372036854775808")
	checkExpr(t, "bc\n", 0, "substr", "abc", "2", "999999999999999999999999999999")
	checkExpr(t, "\n", 1, "substr", "abc", "999999999999999999999999999999", "1")
	checkExpr(t, "\n", 1, "substr", "abc", "0", "2")
	checkExpr(t, "2\n", 0, "index", "abc", "xcb")
	checkExpr(t, "b\n", 0, "match", "abc", `a\(b\)`)
}

type fakeExprCType struct {
	closeCalls int
	classErr   error
}

func (p *fakeExprCType) classify(b byte, member bool) (bool, error) {
	if p.classErr != nil {
		return false, p.classErr
	}
	return member, nil
}
func (p *fakeExprCType) IsAlpha(b byte) (bool, error) {
	return p.classify(b, b >= 'A' && b <= 'Z' || b >= 'a' && b <= 'z' || b == 0xe9)
}
func (p *fakeExprCType) IsAlnum(b byte) (bool, error) {
	ok, err := p.IsAlpha(b)
	return ok || b >= '0' && b <= '9', err
}
func (p *fakeExprCType) IsBlank(b byte) (bool, error) { return p.classify(b, b == ' ' || b == '\t') }
func (p *fakeExprCType) IsCntrl(b byte) (bool, error) {
	return p.classify(b, b < 0x20 || b == 0x7f)
}
func (p *fakeExprCType) IsDigit(b byte) (bool, error) { return p.classify(b, b >= '0' && b <= '9') }
func (p *fakeExprCType) IsGraph(b byte) (bool, error) { return p.classify(b, b > 0x20 && b != 0x7f) }
func (p *fakeExprCType) IsLower(b byte) (bool, error) {
	return p.classify(b, b >= 'a' && b <= 'z' || b == 0xe9)
}
func (p *fakeExprCType) IsPrint(b byte) (bool, error) { return p.classify(b, b >= 0x20 && b != 0x7f) }
func (p *fakeExprCType) IsPunct(b byte) (bool, error) { return p.classify(b, b == '!' || b == '.') }
func (p *fakeExprCType) IsSpace(b byte) (bool, error) {
	return p.classify(b, b == ' ' || b == '\t' || b == '\n')
}
func (p *fakeExprCType) IsUpper(b byte) (bool, error) {
	return p.classify(b, b >= 'A' && b <= 'Z')
}
func (p *fakeExprCType) IsXDigit(b byte) (bool, error) {
	return p.classify(b, b >= '0' && b <= '9' || b >= 'a' && b <= 'f' || b >= 'A' && b <= 'F')
}
func (p *fakeExprCType) ToLower(in []byte) ([]byte, error) {
	if p.classErr != nil {
		return nil, p.classErr
	}
	out := append([]byte(nil), in...)
	for i, b := range out {
		if b >= 'A' && b <= 'Z' {
			out[i] = b + 32
		}
	}
	return out, nil
}
func (p *fakeExprCType) Close() error { p.closeCalls++; return nil }

type fakeExprCollate struct {
	closeCalls int
	closeErr   error
}

func (p *fakeExprCollate) Compare(a, b string) (int, error) {
	return -strings.Compare(a, b), nil
}
func (p *fakeExprCollate) Equivalents(value byte) ([]byte, error) {
	if value == 0 {
		return nil, nil
	}
	if value == 'e' || value == 0xe9 {
		return []byte{'e', 0xe9}, nil
	}
	return []byte{value}, nil
}
func (p *fakeExprCollate) EquivalenceClasses() ([]bool, error) {
	result := make([]bool, 256)
	for i := 1; i < len(result); i++ {
		result[i] = true
	}
	return result, nil
}
func (p *fakeExprCollate) CollationWeights() ([]byte, error) {
	result := make([]byte, 256)
	for i := range result {
		result[i] = byte(i)
	}
	result[0xe9] = 'e'
	return result, nil
}
func (p *fakeExprCollate) CollatingElements() ([]bool, error) {
	result := make([]bool, 256)
	for i := 1; i < len(result); i++ {
		result[i] = true
	}
	return result, nil
}
func (p *fakeExprCollate) Close() error { p.closeCalls++; return p.closeErr }

func runExprLocale(env []string, args []string, ctypeOpen ctypeOpener, collateOpen collateOpener) (string, string, int) {
	var out, errb bytes.Buffer
	rc := &tool.RunContext{Ctx: context.Background(), Env: env, Stdio: tool.Stdio{In: strings.NewReader(""), Out: &out, Err: &errb}}
	code := runWithLocales(rc, args, ctypeOpen, collateOpen)
	return out.String(), errb.String(), code
}

func TestExprLocaleCharacterBoundaries(t *testing.T) {
	out, errb, code := runExprLocale([]string{"LC_ALL=C"}, []string{"length", "é"}, func(string) (ctypeProvider, error) {
		panic("C locale must not open provider")
	}, func(string) (collateProvider, error) {
		panic("C locale must not open provider")
	})
	if code != 0 || errb != "" || out != "2\n" {
		t.Fatalf("C length = (%q,%q,%d), want byte character count", out, errb, code)
	}

	out, errb, code = runExprLocale([]string{"LC_ALL=C"}, []string{"é", ":", ".*"}, func(string) (ctypeProvider, error) {
		panic("C locale must not open provider")
	}, func(string) (collateProvider, error) {
		panic("C locale must not open provider")
	})
	if code != 0 || errb != "" || out != "2\n" {
		t.Fatalf("C match length = (%q,%q,%d), want byte character count", out, errb, code)
	}

	out, errb, code = runExprLocale([]string{"LANG=C", "LC_CTYPE=C.UTF-8", "LC_COLLATE=C"}, []string{"substr", "éx", "2", "1"}, func(string) (ctypeProvider, error) {
		panic("C.UTF-8 character operations must not open byte provider")
	}, func(string) (collateProvider, error) {
		panic("C.UTF-8 character operations must not open collator")
	})
	if code != 0 || errb != "" || out != "x\n" {
		t.Fatalf("C.UTF-8 substr = (%q,%q,%d), want UTF-8 character boundary", out, errb, code)
	}

	latin1 := string([]byte{'a', 0xe9, 'z'})
	ctypeFake := &fakeExprCType{}
	out, errb, code = runExprLocale([]string{"LANG=C", "LC_CTYPE=de_DE.iso88591", "LC_COLLATE=C"}, []string{"substr", latin1, "2", "1"}, func(string) (ctypeProvider, error) {
		return ctypeFake, nil
	}, func(string) (collateProvider, error) {
		return nil, errors.New("collator should not open")
	})
	if code != 0 || errb != "" || out != string([]byte{0xe9, '\n'}) || ctypeFake.closeCalls != 1 {
		t.Fatalf("Latin-1 substr = (%q,%q,%d) close=%d", out, errb, code, ctypeFake.closeCalls)
	}
}

func TestExprLocaleRegexClassesEquivalenceAndRanges(t *testing.T) {
	input := string([]byte{0xe9})
	for _, pattern := range []string{"[[:alpha:]]", "[[=e=]]", "[d-f]"} {
		t.Run(pattern, func(t *testing.T) {
			out, errb, code := runExprLocale([]string{"LC_CTYPE=de_DE.iso88591", "LC_COLLATE=de_DE.iso88591"}, []string{input, ":", pattern}, func(string) (ctypeProvider, error) {
				return &fakeExprCType{}, nil
			}, func(string) (collateProvider, error) {
				return &fakeExprCollate{}, nil
			})
			if code != 0 || errb != "" || out != "1\n" {
				t.Fatalf("expr %q = (%q,%q,%d), want locale match length", pattern, out, errb, code)
			}
		})
	}
}

func TestExprLocaleBREBackreferences(t *testing.T) {
	high := string([]byte{0xe9})
	for _, tc := range []struct {
		name, input, pattern, want string
	}{
		{"class", high + high, `\([[:alpha:]]\)\1`, high + "\n"},
		{"range", high + high, `\([d-f]\)\1`, high + "\n"},
		{"leftmost-longest", "aaaa", `\(a\|aa\)\1`, "aa\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, errb, code := runExprLocale([]string{"LC_CTYPE=de_DE.iso88591", "LC_COLLATE=de_DE.iso88591"}, []string{tc.input, ":", tc.pattern}, func(string) (ctypeProvider, error) {
				return &fakeExprCType{}, nil
			}, func(string) (collateProvider, error) {
				return &fakeExprCollate{}, nil
			})
			if code != 0 || errb != "" || out != tc.want {
				t.Fatalf("expr = (%q,%q,%d), want (%q,empty,0)", out, errb, code, tc.want)
			}
		})
	}
}

func TestExprLocaleCollationComparison(t *testing.T) {
	collateFake := &fakeExprCollate{}
	out, errb, code := runExprLocale([]string{"LC_COLLATE=de_DE.iso88591"}, []string{"a", ">", "b"}, func(string) (ctypeProvider, error) {
		panic("LC_COLLATE alone must not open LC_CTYPE provider")
	}, func(string) (collateProvider, error) {
		return collateFake, nil
	})
	if code != 0 || errb != "" || out != "1\n" || collateFake.closeCalls != 1 {
		t.Fatalf("collation compare = (%q,%q,%d) close=%d", out, errb, code, collateFake.closeCalls)
	}
}

type epipeWriter struct{}

func (w epipeWriter) Write(p []byte) (int, error) { return 0, io.ErrClosedPipe }

func TestExprEPIPE(t *testing.T) {
	cases := []struct {
		name string
		args []string
	}{
		{name: "result", args: []string{"1", "+", "1"}},
		{name: "help", args: []string{"--help"}},
		{name: "version", args: []string{"--version"}},
	}
	for _, tc := range cases {
		for _, ignored := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/ignored=%v", tc.name, ignored), func(t *testing.T) {
				var errb bytes.Buffer
				rc := &tool.RunContext{
					Ctx: context.Background(),
					Stdio: tool.Stdio{
						In:  strings.NewReader(""),
						Out: epipeWriter{},
						Err: &errb,
					},
					SIGPIPEIgnored: ignored,
				}
				code := run(rc, tc.args)
				wantCode, wantErr := 0, ""
				if ignored {
					wantCode, wantErr = 1, "expr: stdout: Broken pipe\n"
				}
				if code != wantCode || errb.String() != wantErr {
					t.Errorf("code=%d stderr=%q, want code=%d stderr=%q", code, errb.String(), wantCode, wantErr)
				}
			})
		}
	}
}
