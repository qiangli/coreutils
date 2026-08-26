package uniqcmd

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/qiangli/coreutils/tool"
)

// runUniqEnv runs uniq with an explicit invocation environment. POSIX Issue 7
// makes LC_CTYPE decide both how bytes are interpreted as characters and which
// characters constitute a <blank>, so every locale assertion below goes through
// rc.Env and never through process-global state.
func runUniqEnv(t *testing.T, env []string, stdin string, args ...string) (string, string, int) {
	t.Helper()
	var out, errb bytes.Buffer
	rc := &tool.RunContext{
		Ctx: context.Background(), Dir: t.TempDir(), Env: env,
		Stdio: tool.Stdio{In: strings.NewReader(stdin), Out: &out, Err: &errb},
	}
	code := run(rc, args)
	return out.String(), errb.String(), code
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

// TestUniqPOSIXMultibyteSkipsCharacters covers XCU:uniq:OPTIONS for -s:
// "Ignore the first chars characters when doing comparisons". Under a UTF-8
// LC_CTYPE the unit is the multi-byte character, not the byte, so a skip that
// lands inside a character is impossible.
func TestUniqPOSIXMultibyteSkipsCharacters(t *testing.T) {
	utf8Env := []string{"POSIXLY_CORRECT=", "LC_ALL=en_US.UTF-8"}
	cEnv := []string{"POSIXLY_CORRECT=1", "LC_ALL=C"}

	// "é" and "ê" share a lead byte and differ in their trailing byte.
	// One character skipped drops both entirely; one byte skipped leaves
	// the differing continuation byte in the key.
	const in = "éx\nêx\n"
	out, errOut, code := runUniqEnv(t, utf8Env, in, "-s", "1")
	if code != 0 || errOut != "" || out != "éx\n" {
		t.Fatalf("UTF-8 -s 1: code=%d stdout=%q stderr=%q", code, out, errOut)
	}
	out, errOut, code = runUniqEnv(t, cEnv, in, "-s", "1")
	if code != 0 || errOut != "" || out != in {
		t.Fatalf("C -s 1: code=%d stdout=%q stderr=%q", code, out, errOut)
	}

	// A malformed byte is one character wide and never merges with the
	// character that follows it, so the input is still advanced exactly once.
	out, errOut, code = runUniqEnv(t, utf8Env, "\xffx\n\xffy\n", "-s", "1")
	if code != 0 || errOut != "" || out != "\xffx\n\xffy\n" {
		t.Fatalf("UTF-8 malformed -s 1: code=%d stdout=%q stderr=%q", code, out, errOut)
	}
}

// TestUniqPOSIXMultibyteFieldsUseLocaleBlanks covers XCU:uniq:OPTIONS for -f
// ("a field is the maximal string matched by the basic regular expression
// [[:blank:]]*[^[:blank:]]*") together with the LC_CTYPE clause "which
// characters constitute a <blank> in the current locale".
func TestUniqPOSIXMultibyteFieldsUseLocaleBlanks(t *testing.T) {
	// U+2003 EM SPACE is a <blank> under a UTF-8 LC_CTYPE and an ordinary
	// non-blank byte sequence in the C locale, so the first field ends in a
	// different place and the resulting keys disagree.
	const in = "a X\nb Y\n"
	out, errOut, code := runUniqEnv(t, []string{"POSIXLY_CORRECT=1", "LC_ALL=en_US.UTF-8"}, in, "-f", "1")
	if code != 0 || errOut != "" || out != in {
		t.Fatalf("UTF-8 -f 1: code=%d stdout=%q stderr=%q", code, out, errOut)
	}
	out, errOut, code = runUniqEnv(t, []string{"POSIXLY_CORRECT=1", "LC_ALL=C"}, in, "-f", "1")
	if code != 0 || errOut != "" || out != "a X\n" {
		t.Fatalf("C -f 1: code=%d stdout=%q stderr=%q", code, out, errOut)
	}

	// Leading blanks belong to the field they precede, and -s counts
	// characters after the skipped fields.
	const nested = " f1 éX\n f2 êX\n"
	out, errOut, code = runUniqEnv(t, []string{"POSIXLY_CORRECT=1", "LC_ALL=en_US.UTF-8"}, nested, "-f", "1", "-s", "2")
	if code != 0 || errOut != "" || out != " f1 éX\n" {
		t.Fatalf("UTF-8 -f 1 -s 2: code=%d stdout=%q stderr=%q", code, out, errOut)
	}
}

// TestUniqPOSIXSkipsPastEndOfLineCompareNullString covers the two Issue 7
// sentences requiring a null string when the option-argument names more
// fields, or more characters, than the line holds.
func TestUniqPOSIXSkipsPastEndOfLineCompareNullString(t *testing.T) {
	env := []string{"POSIXLY_CORRECT=1", "LC_ALL=en_US.UTF-8"}
	out, errOut, code := runUniqEnv(t, env, "é\nê\n", "-s", "9")
	if code != 0 || errOut != "" || out != "é\n" {
		t.Fatalf("-s past end: code=%d stdout=%q stderr=%q", code, out, errOut)
	}
	out, errOut, code = runUniqEnv(t, env, "a b\nc d\n", "-f", "9")
	if code != 0 || errOut != "" || out != "a b\n" {
		t.Fatalf("-f past end: code=%d stdout=%q stderr=%q", code, out, errOut)
	}
}

// TestUniqPOSIXSingleByteLocaleUsesProviderAndPrecedence proves the
// LC_ALL > LC_CTYPE > LANG precedence of XBD Internationalization Variables
// and that a non-C single-byte locale takes its <blank> set from the provider
// instead of silently inheriting the C locale's.
func TestUniqPOSIXSingleByteLocaleUsesProviderAndPrecedence(t *testing.T) {
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
			// 0xA0 is a <blank> only for this provider, so it alone ends
			// the first field and exposes the differing tails.
			const in = "a\xa0X\nb\xa0Y\n"
			out, errOut, code := runUniqEnv(t, tc.env, in, "-f", "1")
			if code != 0 || errOut != "" || out != in {
				t.Fatalf("code=%d stdout=%q stderr=%q", code, out, errOut)
			}
			if !p.closed {
				t.Fatal("LC_CTYPE provider was not closed")
			}
		})
	}
}

// TestUniqPOSIXCLocaleOpensNoProvider pins that the C/POSIX locale is answered
// from the built-in tables, so uniq cannot fail for want of installed locale
// data in the locale POSIX requires every conforming system to provide.
func TestUniqPOSIXCLocaleOpensNoProvider(t *testing.T) {
	old := openCTypeFn
	defer func() { openCTypeFn = old }()
	openCTypeFn = func(string) (ctypeProvider, error) {
		t.Fatal("C locale must not open a locale provider")
		return nil, nil
	}
	for _, env := range [][]string{
		{"POSIXLY_CORRECT=1", "LC_ALL=C"},
		{"POSIXLY_CORRECT=1", "LC_ALL=POSIX"},
		{"POSIXLY_CORRECT=1"},
	} {
		out, errOut, code := runUniqEnv(t, env, "a b\na c\n", "-f", "1")
		if code != 0 || errOut != "" || out != "a b\na c\n" {
			t.Fatalf("env=%v: code=%d stdout=%q stderr=%q", env, code, out, errOut)
		}
	}
}

// TestUniqLocaleProviderFailuresAreDiagnosed proves the CONSEQUENCES OF ERRORS
// contract for an unusable LC_CTYPE: a diagnostic on standard error, no
// standard output, and a status greater than zero.
func TestUniqLocaleProviderFailuresAreDiagnosed(t *testing.T) {
	old := openCTypeFn
	defer func() { openCTypeFn = old }()
	env := []string{"POSIXLY_CORRECT=1", "LC_ALL=broken_8bit"}

	t.Run("open", func(t *testing.T) {
		openCTypeFn = func(string) (ctypeProvider, error) { return nil, errors.New("locale unavailable") }
		out, errOut, code := runUniqEnv(t, env, "a\na\n")
		if code != 1 || out != "" || !strings.Contains(errOut, `LC_CTYPE "broken_8bit": locale unavailable`) {
			t.Fatalf("code=%d stdout=%q stderr=%q", code, out, errOut)
		}
	})

	t.Run("classification", func(t *testing.T) {
		p := &fakeBlankProvider{classErr: errors.New("classification failed")}
		openCTypeFn = func(string) (ctypeProvider, error) { return p, nil }
		out, errOut, code := runUniqEnv(t, env, "a\na\n")
		if code != 1 || out != "" || !p.closed || !strings.Contains(errOut, "classification failed") {
			t.Fatalf("code=%d stdout=%q closed=%v stderr=%q", code, out, p.closed, errOut)
		}
	})

	t.Run("close", func(t *testing.T) {
		p := &fakeBlankProvider{blank: map[byte]bool{' ': true}, closeErr: errors.New("close failed")}
		openCTypeFn = func(string) (ctypeProvider, error) { return p, nil }
		out, errOut, code := runUniqEnv(t, env, "a\na\n")
		if code != 1 || out != "" || !p.closed || !strings.Contains(errOut, "close failed") {
			t.Fatalf("code=%d stdout=%q closed=%v stderr=%q", code, out, p.closed, errOut)
		}
	})
}

// TestUniqOutsidePOSIXModeKeepsByteKeys pins the GNU-compatible extension
// tier: without the POSIX switch uniq keeps its historical byte model and
// opens no provider, so no locale name can make it fail.
func TestUniqOutsidePOSIXModeKeepsByteKeys(t *testing.T) {
	old := openCTypeFn
	defer func() { openCTypeFn = old }()
	openCTypeFn = func(string) (ctypeProvider, error) {
		t.Fatal("locale provider must not be opened outside POSIX mode")
		return nil, nil
	}
	const in = "éx\nêx\n"
	out, errOut, code := runUniqEnv(t, []string{"LC_ALL=en_US.UTF-8"}, in, "-s", "1")
	if code != 0 || errOut != "" || out != in {
		t.Fatalf("UTF-8 outside POSIX mode: code=%d stdout=%q stderr=%q", code, out, errOut)
	}
	out, errOut, code = runUniqEnv(t, []string{"LC_ALL=unsupported_locale"}, "a b\na c\n", "-f", "1")
	if code != 0 || errOut != "" || out != "a b\na c\n" {
		t.Fatalf("unsupported locale outside POSIX mode: code=%d stdout=%q stderr=%q", code, out, errOut)
	}
}
