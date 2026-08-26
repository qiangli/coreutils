package wccmd

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/qiangli/coreutils/tool"
)

func runWCEnv(t *testing.T, env []string, input string, args ...string) (string, string, int) {
	t.Helper()
	var out, err bytes.Buffer
	rc := &tool.RunContext{
		Ctx: context.Background(), Dir: t.TempDir(), Env: env,
		Stdio: tool.Stdio{In: strings.NewReader(input), Out: &out, Err: &err},
	}
	code := run(rc, args)
	return out.String(), err.String(), code
}

type fakeSpaceProvider struct {
	space    map[byte]bool
	classErr error
	closeErr error
	closed   bool
}

func (p *fakeSpaceProvider) IsSpace(b byte) (bool, error) {
	if p.classErr != nil {
		return false, p.classErr
	}
	return p.space[b], nil
}
func (p *fakeSpaceProvider) Close() error { p.closed = true; return p.closeErr }

func TestPOSIXUTF8CountsCharactersAndLocaleWords(t *testing.T) {
	env := []string{"POSIXLY_CORRECT=", "LC_ALL=en_US.UTF-8"}
	out, errOut, code := runWCEnv(t, env, "a\u00a0b\n", "-lmwc")
	if code != 0 || errOut != "" || out != "      1       2       4       5\n" {
		t.Fatalf("code=%d stdout=%q stderr=%q", code, out, errOut)
	}

	// Invalid UTF-8 bytes contribute to -c, but are not characters for -m.
	// They remain non-space for word-boundary purposes.
	out, errOut, code = runWCEnv(t, env, "a\xffé", "-mwc")
	if code != 0 || errOut != "" || out != "      1       2       4\n" {
		t.Fatalf("malformed: code=%d stdout=%q stderr=%q", code, out, errOut)
	}

	// A valid encoding of U+FFFD is one character, unlike a malformed byte.
	out, errOut, code = runWCEnv(t, env, "\ufffd", "-mc")
	if code != 0 || errOut != "" || out != "      1       3\n" {
		t.Fatalf("replacement rune: code=%d stdout=%q stderr=%q", code, out, errOut)
	}
}

func TestPOSIXUTF8MaximumLineUsesDisplayColumns(t *testing.T) {
	env := []string{"POSIXLY_CORRECT=1", "LC_ALL=C.UTF-8"}
	out, errOut, code := runWCEnv(t, env, "界e\u0301\n", "-L")
	if code != 0 || errOut != "" || out != "3\n" {
		t.Fatalf("code=%d stdout=%q stderr=%q", code, out, errOut)
	}
}

func TestPOSIXCLocaleCountsBytesAsCharacters(t *testing.T) {
	old := openCTypeFn
	defer func() { openCTypeFn = old }()
	openCTypeFn = func(string) (ctypeProvider, error) {
		t.Fatal("C locale must not open a locale provider")
		return nil, nil
	}
	out, errOut, code := runWCEnv(t, []string{"POSIXLY_CORRECT=", "LC_ALL=POSIX"}, "é\n", "-mwc")
	if code != 0 || errOut != "" || out != "      1       3       3\n" {
		t.Fatalf("code=%d stdout=%q stderr=%q", code, out, errOut)
	}
}

func TestPOSIXSingleByteLocaleUsesProviderAndPrecedence(t *testing.T) {
	old := openCTypeFn
	defer func() { openCTypeFn = old }()
	p := &fakeSpaceProvider{space: map[byte]bool{0xa0: true, ' ': true, '\t': true, '\n': true, '\r': true, '\v': true, '\f': true}}
	openCTypeFn = func(name string) (ctypeProvider, error) {
		if name != "chosen_8bit" {
			t.Fatalf("resolved locale = %q, want chosen_8bit", name)
		}
		return p, nil
	}
	env := []string{"POSIXLY_CORRECT=", "LANG=ignored", "LC_CTYPE=ignored_too", "LC_ALL=chosen_8bit"}
	out, errOut, code := runWCEnv(t, env, "a\xa0b", "-wm")
	if code != 0 || errOut != "" || out != "      2       3\n" {
		t.Fatalf("code=%d stdout=%q stderr=%q", code, out, errOut)
	}
	if !p.closed {
		t.Fatal("LC_CTYPE provider was not closed")
	}
}

func TestLocaleProviderFailuresAreDiagnosedOnlyWhenNeeded(t *testing.T) {
	old := openCTypeFn
	defer func() { openCTypeFn = old }()

	openCTypeFn = func(string) (ctypeProvider, error) { return nil, errors.New("locale unavailable") }
	env := []string{"POSIXLY_CORRECT=1", "LC_ALL=x"}
	out, errOut, code := runWCEnv(t, env, "abc", "-c")
	if code != 0 || errOut != "" || out != "3\n" {
		t.Fatalf("-c must not need LC_CTYPE: code=%d stdout=%q stderr=%q", code, out, errOut)
	}
	_, errOut, code = runWCEnv(t, env, "abc", "-w")
	if code != 1 || !strings.Contains(errOut, `LC_CTYPE "x": locale unavailable`) {
		t.Fatalf("-w: code=%d stderr=%q", code, errOut)
	}

	p := &fakeSpaceProvider{classErr: errors.New("classification failed")}
	openCTypeFn = func(string) (ctypeProvider, error) { return p, nil }
	_, errOut, code = runWCEnv(t, env, "", "-m")
	if code != 1 || !p.closed || !strings.Contains(errOut, "classification failed") {
		t.Fatalf("classification: code=%d closed=%v stderr=%q", code, p.closed, errOut)
	}

	p = &fakeSpaceProvider{closeErr: errors.New("close failed")}
	openCTypeFn = func(string) (ctypeProvider, error) { return p, nil }
	_, errOut, code = runWCEnv(t, env, "", "-w")
	if code != 1 || !p.closed || !strings.Contains(errOut, "close failed") {
		t.Fatalf("close: code=%d closed=%v stderr=%q", code, p.closed, errOut)
	}
}

func TestOutsidePOSIXModeKeepsLegacyByteCounts(t *testing.T) {
	old := openCTypeFn
	defer func() { openCTypeFn = old }()
	openCTypeFn = func(string) (ctypeProvider, error) {
		t.Fatal("locale provider must not be opened outside POSIX mode")
		return nil, nil
	}
	out, errOut, code := runWCEnv(t, []string{"LC_ALL=en_US.UTF-8"}, "é\n", "-mwc")
	if code != 0 || errOut != "" || out != "      1       3       3\n" {
		t.Fatalf("code=%d stdout=%q stderr=%q", code, out, errOut)
	}
}

// TestPOSIXLocalePrecedenceSelectsCTypeCategory proves the XBD
// Internationalization Variables precedence that XCU:wc:ENVIRONMENT_VARIABLES
// inherits: LC_ALL overrides LC_CTYPE, LC_CTYPE overrides LANG, and an empty
// assignment falls through instead of shadowing the next level.
func TestPOSIXLocalePrecedenceSelectsCTypeCategory(t *testing.T) {
	old := openCTypeFn
	defer func() { openCTypeFn = old }()
	for _, tc := range []struct {
		name string
		env  []string
		want string
	}{
		{"LC_ALL wins", []string{"POSIXLY_CORRECT=", "LANG=lang", "LC_CTYPE=ctype", "LC_ALL=chosen_8bit"}, "chosen_8bit"},
		{"LC_CTYPE over LANG", []string{"POSIXLY_CORRECT=", "LANG=lang", "LC_CTYPE=chosen_8bit"}, "chosen_8bit"},
		{"LANG last", []string{"POSIXLY_CORRECT=", "LANG=chosen_8bit"}, "chosen_8bit"},
		{"empty values fall through", []string{"POSIXLY_CORRECT=", "LC_ALL=", "LC_CTYPE=", "LANG=chosen_8bit"}, "chosen_8bit"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := &fakeSpaceProvider{space: map[byte]bool{' ': true, 0xa0: true}}
			openCTypeFn = func(name string) (ctypeProvider, error) {
				if name != tc.want {
					t.Fatalf("resolved LC_CTYPE = %q, want %q", name, tc.want)
				}
				return p, nil
			}
			// 0xA0 separates words only for this provider's <space> set.
			out, errOut, code := runWCEnv(t, tc.env, "a\xa0b", "-w")
			if code != 0 || errOut != "" || out != "2\n" {
				t.Fatalf("code=%d stdout=%q stderr=%q", code, out, errOut)
			}
			if !p.closed {
				t.Fatal("LC_CTYPE provider was not closed")
			}
		})
	}

	// A UTF-8 codeset selected through LANG alone reaches the multi-byte
	// model without any provider being opened.
	openCTypeFn = func(string) (ctypeProvider, error) {
		t.Fatal("a UTF-8 locale must not open the single-byte provider")
		return nil, nil
	}
	out, errOut, code := runWCEnv(t, []string{"POSIXLY_CORRECT=1", "LANG=en_US.UTF-8"}, "é\n", "-mc")
	if code != 0 || errOut != "" || out != "      2       3\n" {
		t.Fatalf("LANG UTF-8: code=%d stdout=%q stderr=%q", code, out, errOut)
	}
}

// TestPOSIXUTF8CharacterBoundaryAcrossReadBuffer proves that the LC_CTYPE
// requirement to interpret "sequences of bytes of text data as characters"
// holds across the internal read boundary: a multi-byte character split by a
// buffer refill is still exactly one character.
func TestPOSIXUTF8CharacterBoundaryAcrossReadBuffer(t *testing.T) {
	env := []string{"POSIXLY_CORRECT=1", "LC_ALL=en_US.UTF-8"}
	// bufio's default reader buffer is 4096 bytes; place the 3-byte
	// character so that its first byte is the last byte of the first fill.
	for _, lead := range []int{4094, 4095, 4096} {
		input := strings.Repeat("a", lead) + "界\n"
		wantChars := int64(lead) + 2
		wantBytes := int64(lead) + 4
		out, errOut, code := runWCEnv(t, env, input, "-mc")
		want := fmt.Sprintf("%7d %7d\n", wantChars, wantBytes)
		if code != 0 || errOut != "" || out != want {
			t.Fatalf("lead=%d: code=%d stdout=%q stderr=%q, want %q", lead, code, out, errOut, want)
		}
	}
}

// TestPOSIXUTF8WordsUseLocaleWhiteSpace covers XCU:wc:STDOUT's word count:
// "a word is a non-zero-length string of characters delimited by white space",
// where LC_CTYPE decides which characters are white space.
func TestPOSIXUTF8WordsUseLocaleWhiteSpace(t *testing.T) {
	// U+3000 IDEOGRAPHIC SPACE is white space under a UTF-8 LC_CTYPE and
	// three ordinary bytes in the C locale.
	const in = "a　b\n"
	out, errOut, code := runWCEnv(t, []string{"POSIXLY_CORRECT=1", "LC_ALL=en_US.UTF-8"}, in, "-w")
	if code != 0 || errOut != "" || out != "2\n" {
		t.Fatalf("UTF-8: code=%d stdout=%q stderr=%q", code, out, errOut)
	}
	out, errOut, code = runWCEnv(t, []string{"POSIXLY_CORRECT=1", "LC_ALL=C"}, in, "-w")
	if code != 0 || errOut != "" || out != "1\n" {
		t.Fatalf("C: code=%d stdout=%q stderr=%q", code, out, errOut)
	}
}

// TestPOSIXUTF8MaximumLineWidthUsesEffectiveLocale exercises the display-width
// half of the LC_CTYPE contract for the -L extension: U+00A1 is East Asian
// Ambiguous, so its column count follows the selected locale.
func TestPOSIXUTF8MaximumLineWidthUsesEffectiveLocale(t *testing.T) {
	out, errOut, code := runWCEnv(t, []string{"POSIXLY_CORRECT=1", "LC_ALL=ja_JP.UTF-8"}, "¡\n", "-L")
	if code != 0 || errOut != "" || out != "2\n" {
		t.Fatalf("ja: code=%d stdout=%q stderr=%q", code, out, errOut)
	}
	out, errOut, code = runWCEnv(t, []string{"POSIXLY_CORRECT=1", "LC_ALL=en_US.UTF-8"}, "¡\n", "-L")
	if code != 0 || errOut != "" || out != "1\n" {
		t.Fatalf("en: code=%d stdout=%q stderr=%q", code, out, errOut)
	}
}

// TestPOSIXNumericLocaleDoesNotAlterCounts pins that wc's counts are written
// as plain decimal numbers: XCU:wc:ENVIRONMENT_VARIABLES lists no LC_NUMERIC,
// so no radix or grouping convention may reach standard output.
func TestPOSIXNumericLocaleDoesNotAlterCounts(t *testing.T) {
	input := strings.Repeat("word ", 2000) + "\n"
	want := "      1    2000   10001\n"
	for _, env := range [][]string{
		{"POSIXLY_CORRECT=1", "LC_ALL=C"},
		{"POSIXLY_CORRECT=1", "LC_CTYPE=en_US.UTF-8", "LC_NUMERIC=de_DE.UTF-8"},
		{"POSIXLY_CORRECT=1", "LC_CTYPE=en_US.UTF-8", "LC_NUMERIC=en_US.UTF-8"},
	} {
		out, errOut, code := runWCEnv(t, env, input, "-lwc")
		if code != 0 || errOut != "" || out != want {
			t.Fatalf("env=%v: code=%d stdout=%q stderr=%q, want %q", env, code, out, errOut, want)
		}
	}
}
