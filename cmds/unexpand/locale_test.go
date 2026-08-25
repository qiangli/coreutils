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
