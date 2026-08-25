package wccmd

import (
	"bytes"
	"context"
	"errors"
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
