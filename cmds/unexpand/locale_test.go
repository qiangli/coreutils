package unexpandcmd

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/qiangli/coreutils/tool"
)

func runUnexpandEnv(t *testing.T, env []string, input string, args ...string) (string, string, int) {
	t.Helper()
	var out, err bytes.Buffer
	rc := &tool.RunContext{
		Ctx: context.Background(), Dir: t.TempDir(), Env: env,
		Stdio: tool.Stdio{In: strings.NewReader(input), Out: &out, Err: &err},
	}
	code := run(rc, args)
	return out.String(), err.String(), code
}

type fakeBlankProvider struct {
	blank    map[byte]bool
	classErr error
	closeErr error
	closed   bool
}

func (p *fakeBlankProvider) IsBlank(b byte) (bool, error) {
	if p.classErr != nil {
		return false, p.classErr
	}
	return p.blank[b], nil
}
func (p *fakeBlankProvider) Close() error { p.closed = true; return p.closeErr }

func TestPOSIXUTF8UsesDisplayColumnsAndLocaleBlanks(t *testing.T) {
	env := []string{"POSIXLY_CORRECT=", "LC_ALL=en_US.UTF-8"}
	for _, tc := range []struct {
		name, input, want string
	}{
		{"wide", "界      x\n", "界\tx\n"},
		{"zero-width", "e\u0301       x\n", "e\u0301\tx\n"},
		{"unicode-blank", "\u00a0       x\n", "\tx\n"},
		{"malformed-preserved", "\xff       x\n", "\xff\tx\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, errOut, code := runUnexpandEnv(t, env, tc.input, "-a")
			if code != 0 || errOut != "" || out != tc.want {
				t.Fatalf("code=%d stdout=%q stderr=%q, want stdout=%q", code, out, errOut, tc.want)
			}
		})
	}
}

func TestPOSIXUTF8DisplayWidthUsesEffectiveLocale(t *testing.T) {
	input := "¡      x\n" // U+00A1 is East Asian Ambiguous: width 2 in ja, 1 in en.
	out, errOut, code := runUnexpandEnv(t, []string{"POSIXLY_CORRECT=1", "LC_ALL=ja_JP.UTF-8"}, input, "-a")
	if code != 0 || errOut != "" || out != "¡\tx\n" {
		t.Fatalf("ja locale: code=%d stdout=%q stderr=%q", code, out, errOut)
	}
	out, errOut, code = runUnexpandEnv(t, []string{"POSIXLY_CORRECT=1", "LC_ALL=en_US.UTF-8"}, input, "-a")
	if code != 0 || errOut != "" || out != input {
		t.Fatalf("en locale: code=%d stdout=%q stderr=%q", code, out, errOut)
	}
}

func TestPOSIXCLocaleUsesByteColumns(t *testing.T) {
	old := openCTypeFn
	defer func() { openCTypeFn = old }()
	openCTypeFn = func(string) (ctypeProvider, error) {
		t.Fatal("C locale must not open a locale provider")
		return nil, nil
	}
	out, errOut, code := runUnexpandEnv(t, []string{"POSIXLY_CORRECT=", "LC_ALL=C"}, "é      x\n", "-a")
	if code != 0 || errOut != "" || out != "é\tx\n" {
		t.Fatalf("code=%d stdout=%q stderr=%q", code, out, errOut)
	}
}

func TestPOSIXSingleByteLocaleUsesProviderAndPrecedence(t *testing.T) {
	old := openCTypeFn
	defer func() { openCTypeFn = old }()
	p := &fakeBlankProvider{blank: map[byte]bool{0xa0: true, ' ': true, '\t': true}}
	openCTypeFn = func(name string) (ctypeProvider, error) {
		if name != "chosen_8bit" {
			t.Fatalf("resolved locale = %q, want chosen_8bit", name)
		}
		return p, nil
	}
	env := []string{"POSIXLY_CORRECT=", "LANG=ignored", "LC_CTYPE=ignored_too", "LC_ALL=chosen_8bit"}
	out, errOut, code := runUnexpandEnv(t, env, "\xa0       x\n", "-a")
	if code != 0 || errOut != "" || out != "\tx\n" {
		t.Fatalf("code=%d stdout=%q stderr=%q", code, out, errOut)
	}
	if !p.closed {
		t.Fatal("LC_CTYPE provider was not closed")
	}
}

func TestLocaleProviderFailuresAreDiagnosedInPOSIXMode(t *testing.T) {
	old := openCTypeFn
	defer func() { openCTypeFn = old }()

	t.Run("open", func(t *testing.T) {
		openCTypeFn = func(string) (ctypeProvider, error) { return nil, errors.New("locale unavailable") }
		out, errOut, code := runUnexpandEnv(t, []string{"POSIXLY_CORRECT=1", "LC_ALL=x"}, "        x\n")
		if code != 1 || out != "" || !strings.Contains(errOut, `LC_CTYPE "x": locale unavailable`) {
			t.Fatalf("code=%d stdout=%q stderr=%q", code, out, errOut)
		}
	})

	t.Run("classification", func(t *testing.T) {
		p := &fakeBlankProvider{classErr: errors.New("classification failed")}
		openCTypeFn = func(string) (ctypeProvider, error) { return p, nil }
		_, errOut, code := runUnexpandEnv(t, []string{"POSIXLY_CORRECT=1", "LC_ALL=x"}, "")
		if code != 1 || !p.closed || !strings.Contains(errOut, "classification failed") {
			t.Fatalf("code=%d closed=%v stderr=%q", code, p.closed, errOut)
		}
	})

	t.Run("close", func(t *testing.T) {
		p := &fakeBlankProvider{closeErr: errors.New("close failed")}
		openCTypeFn = func(string) (ctypeProvider, error) { return p, nil }
		_, errOut, code := runUnexpandEnv(t, []string{"POSIXLY_CORRECT=1", "LC_ALL=x"}, "")
		if code != 1 || !p.closed || !strings.Contains(errOut, "close failed") {
			t.Fatalf("code=%d closed=%v stderr=%q", code, p.closed, errOut)
		}
	})
}

func TestExtensionsKeepLegacyLocaleBehaviorOutsidePOSIXMode(t *testing.T) {
	old := openCTypeFn
	defer func() { openCTypeFn = old }()
	openCTypeFn = func(string) (ctypeProvider, error) {
		t.Fatal("locale provider must not be opened outside POSIX mode")
		return nil, nil
	}
	out, errOut, code := runUnexpandEnv(t, []string{"LC_ALL=unsupported"}, "界      x\n", "-a")
	if code != 0 || errOut != "" || out != "界      x\n" {
		t.Fatalf("code=%d stdout=%q stderr=%q", code, out, errOut)
	}
}

// TestPOSIXLocalePrecedenceSelectsCTypeCategory proves the XBD
// Internationalization Variables precedence inherited by
// XCU:unexpand:ENVIRONMENT_VARIABLES: LC_ALL beats LC_CTYPE, LC_CTYPE beats
// LANG, and an empty assignment falls through rather than shadowing.
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
			p := &fakeBlankProvider{blank: map[byte]bool{' ': true, '\t': true, 0xa0: true}}
			openCTypeFn = func(name string) (ctypeProvider, error) {
				if name != tc.want {
					t.Fatalf("resolved LC_CTYPE = %q, want %q", name, tc.want)
				}
				return p, nil
			}
			out, errOut, code := runUnexpandEnv(t, tc.env, "\xa0       x\n", "-a")
			if code != 0 || errOut != "" || out != "\tx\n" {
				t.Fatalf("code=%d stdout=%q stderr=%q", code, out, errOut)
			}
			if !p.closed {
				t.Fatal("LC_CTYPE provider was not closed")
			}
		})
	}

	// A UTF-8 codeset selected through LANG alone reaches the multi-byte
	// model without opening the single-byte provider.
	openCTypeFn = func(string) (ctypeProvider, error) {
		t.Fatal("a UTF-8 locale must not open the single-byte provider")
		return nil, nil
	}
	out, errOut, code := runUnexpandEnv(t, []string{"POSIXLY_CORRECT=1", "LANG=en_US.UTF-8"}, "界      x\n", "-a")
	if code != 0 || errOut != "" || out != "界\tx\n" {
		t.Fatalf("LANG UTF-8: code=%d stdout=%q stderr=%q", code, out, errOut)
	}
}

// TestPOSIXUTF8WideCharactersAdvanceDisplayColumns proves that the column
// arithmetic behind XCU:unexpand:STDOUT ("replacing the maximum eligible runs
// of blanks with tabs") counts display columns of multi-byte characters, not
// bytes: the same blank run reaches a tab stop under a UTF-8 LC_CTYPE and
// falls short of one in the C locale, where each byte is a column.
func TestPOSIXUTF8WideCharactersAdvanceDisplayColumns(t *testing.T) {
	const in = "界界界   x\n"
	out, errOut, code := runUnexpandEnv(t, []string{"POSIXLY_CORRECT=1", "LC_ALL=en_US.UTF-8"}, in, "-a")
	if code != 0 || errOut != "" || out != "界界界\t x\n" {
		t.Fatalf("UTF-8: code=%d stdout=%q stderr=%q", code, out, errOut)
	}
	out, errOut, code = runUnexpandEnv(t, []string{"POSIXLY_CORRECT=1", "LC_ALL=C"}, in, "-a")
	if code != 0 || errOut != "" || out != in {
		t.Fatalf("C: code=%d stdout=%q stderr=%q", code, out, errOut)
	}
}

// TestPOSIXUTF8LocaleBlanksBeyondLatin1 extends the <blank> half of the
// LC_CTYPE contract past the single-byte range: U+2003 EM SPACE and U+3000
// IDEOGRAPHIC SPACE are blanks whose display widths differ, and both are
// ordinary non-blank bytes in the C locale.
func TestPOSIXUTF8LocaleBlanksBeyondLatin1(t *testing.T) {
	env := []string{"POSIXLY_CORRECT=1", "LC_ALL=en_US.UTF-8"}
	for _, tc := range []struct {
		name, input, want, wantC string
	}{
		// U+2003 is a one-column blank, so it plus seven spaces are eight
		// blanks reaching column 8. In the C locale its three bytes are
		// three non-blank columns, so only the five spaces that follow
		// reach column 8 and two spaces are left past the tab.
		{"em-space", "        x\n", "\tx\n", " \t  x\n"},
		// U+3000 is a two-column blank, so it plus six spaces reach column
		// 8. In the C locale its three bytes are three non-blank columns
		// and one space is left past the tab.
		{"ideographic-space", "　      x\n", "\tx\n", "　\t x\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, errOut, code := runUnexpandEnv(t, env, tc.input, "-a")
			if code != 0 || errOut != "" || out != tc.want {
				t.Fatalf("code=%d stdout=%q stderr=%q, want %q", code, out, errOut, tc.want)
			}
			cOut, cErr, cCode := runUnexpandEnv(t, []string{"POSIXLY_CORRECT=1", "LC_ALL=C"}, tc.input, "-a")
			if cCode != 0 || cErr != "" || cOut != tc.wantC {
				t.Fatalf("C locale: code=%d stdout=%q stderr=%q, want %q", cCode, cOut, cErr, tc.wantC)
			}
		})
	}
}

// TestPOSIXUTF8CharacterBoundaryBeyondReadBuffer proves the multi-byte
// interpretation survives a line longer than the internal read buffer: the
// trailing blank run is still measured from display columns, not from a
// re-synchronized byte offset.
func TestPOSIXUTF8CharacterBoundaryBeyondReadBuffer(t *testing.T) {
	// 2050 two-column characters occupy 4100 columns from 6150 bytes, so the
	// line crosses bufio's 4096-byte buffer inside a character.
	lead := strings.Repeat("界", 2050)
	out, errOut, code := runUnexpandEnv(t, []string{"POSIXLY_CORRECT=1", "LC_ALL=en_US.UTF-8"}, lead+"    x\n", "-a")
	if code != 0 || errOut != "" || out != lead+"\tx\n" {
		t.Fatalf("code=%d stdout=%q stderr=%q", code, out, errOut)
	}
}

// TestPOSIXNumericLocaleDoesNotAlterTablist pins that the -t option-argument
// is read as plain decimal integers: XCU:unexpand:ENVIRONMENT_VARIABLES lists
// no LC_NUMERIC, so no locale radix or grouping character may be accepted.
func TestPOSIXNumericLocaleDoesNotAlterTablist(t *testing.T) {
	for _, env := range [][]string{
		{"POSIXLY_CORRECT=1", "LC_ALL=C"},
		{"POSIXLY_CORRECT=1", "LC_CTYPE=en_US.UTF-8", "LC_NUMERIC=de_DE.UTF-8"},
	} {
		out, errOut, code := runUnexpandEnv(t, env, "    x\n", "-t", "4")
		if code != 0 || errOut != "" || out != "\tx\n" {
			t.Fatalf("env=%v: code=%d stdout=%q stderr=%q", env, code, out, errOut)
		}
		_, errOut, code = runUnexpandEnv(t, env, "    x\n", "-t", "4.5")
		if code != 2 || !strings.Contains(errOut, "invalid character") {
			t.Fatalf("env=%v radix: code=%d stderr=%q", env, code, errOut)
		}
	}
}
