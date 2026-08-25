package expandcmd

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/qiangli/coreutils/tool"
)

func runExpandBytesEnv(
	t *testing.T, env []string, input []byte, args ...string,
) ([]byte, string, int) {
	t.Helper()
	var out, errOut bytes.Buffer
	rc := &tool.RunContext{
		Ctx: context.Background(), Dir: t.TempDir(), Env: env,
		Stdio: tool.Stdio{In: bytes.NewReader(input), Out: &out, Err: &errOut},
	}
	code := run(rc, args)
	return out.Bytes(), errOut.String(), code
}

func TestIssue737LocaleCharacterBoundariesPreserveOriginalBytes(t *testing.T) {
	latin1 := []string{"LC_ALL=de_DE.ISO-8859-1"}
	utf8Locale := []string{"LC_ALL=C.UTF-8"}
	for _, tc := range []struct {
		name  string
		env   []string
		input []byte
		args  []string
		want  []byte
	}{
		{
			// Each Latin-1 byte is one character: 0xe4, 'b' reach column 2,
			// the tab expands two spaces to the stop at 4, and 0xe4 is
			// copied as the single original byte.
			"latin1-one-byte-characters", latin1,
			[]byte{0xe4, 'b', '\t', 'c', '\n'}, []string{"-t", "4"},
			[]byte{0xe4, 'b', ' ', ' ', 'c', '\n'},
		},
		{
			// Three Latin-1 accented letters count three columns, so the
			// tab adds one space.
			"latin1-high-bytes-count-columns", latin1,
			[]byte{0xe4, 0xf6, 0xfc, '\t', 'x', '\n'}, []string{"-t", "4"},
			[]byte{0xe4, 0xf6, 0xfc, ' ', 'x', '\n'},
		},
		{
			// In a UTF-8 locale the two-byte ä is one character at column 1.
			"utf8-two-byte-character", utf8Locale,
			[]byte("äb\tx\n"), []string{"-t", "4"},
			[]byte("äb  x\n"),
		},
		{
			// A wide character advances two display columns.
			"utf8-wide-character", utf8Locale,
			[]byte("漢\tx\n"), []string{"-t", "4"},
			[]byte("漢  x\n"),
		},
		{
			// A combining mark advances zero columns but keeps its bytes.
			"utf8-combining-mark", utf8Locale,
			[]byte("e\u0301\tx\n"), []string{"-t", "4"},
			[]byte("e\u0301   x\n"),
		},
		{
			// The C locale treats every byte — including the UTF-8 lead and
			// continuation bytes of ä — as one character each.
			"c-locale-byte-characters", []string{"LC_ALL=C"},
			[]byte{0xc3, 0xa4, '\t', 'x', '\n'}, []string{"-t", "4"},
			[]byte{0xc3, 0xa4, ' ', ' ', 'x', '\n'},
		},
		{
			// -U forces byte columns even in a UTF-8 locale.
			"utf8-bytes-extension", utf8Locale,
			[]byte("é\tx\n"), []string{"-U", "-t", "4"},
			[]byte("é  x\n"),
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, errOut, code := runExpandBytesEnv(t, tc.env, tc.input, tc.args...)
			if code != 0 || errOut != "" || !bytes.Equal(out, tc.want) {
				t.Fatalf(
					"code=%d stdout=% x stderr=%q, want stdout=% x",
					code, out, errOut, tc.want,
				)
			}
		})
	}
}

func TestIssue737MalformedAndCLocaleBytesAreNeverReencoded(t *testing.T) {
	for _, tc := range []struct {
		name string
		env  []string
		args []string
	}{
		{"malformed-utf8", []string{"LC_ALL=C.UTF-8"}, []string{"-t", "4"}},
		{"c-locale", []string{"LC_ALL=C"}, []string{"-t", "4"}},
		{"posix-locale", []string{"LC_ALL=POSIX"}, []string{"-t", "4"}},
		{"default-posix", nil, []string{"-t", "4"}},
		{"bytes-extension", []string{"LC_ALL=C.UTF-8"}, []string{"-U", "-t", "4"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// 0xff and 0xfe are malformed in UTF-8; each must stay one
			// original byte and count one column, never becoming U+FFFD.
			input := []byte{0xff, 0xfe, '\t', 'x', '\n'}
			want := []byte{0xff, 0xfe, ' ', ' ', 'x', '\n'}
			out, errOut, code := runExpandBytesEnv(t, tc.env, input, tc.args...)
			if code != 0 || errOut != "" || !bytes.Equal(out, want) {
				t.Fatalf(
					"code=%d stdout=% x stderr=%q, want stdout=% x",
					code, out, errOut, want,
				)
			}
		})
	}
}

func TestIssue737InitialRegionAndBackspaceAcrossLocales(t *testing.T) {
	for _, tc := range []struct {
		name  string
		env   []string
		input []byte
		args  []string
		want  []byte
	}{
		{
			// -i: the leading tab converts, the Latin-1 accented letter
			// ends the initial region, the later tab is kept verbatim.
			"latin1-initial", []string{"LC_ALL=de_DE.ISO-8859-1"},
			[]byte{'\t', 0xe4, '\t', 'x', '\n'}, []string{"-i", "-t", "4"},
			[]byte{' ', ' ', ' ', ' ', 0xe4, '\t', 'x', '\n'},
		},
		{
			"utf8-initial-wide", []string{"LC_ALL=C.UTF-8"},
			[]byte("\t漢\tx\n"), []string{"-i", "-t", "4"},
			[]byte("    漢\tx\n"),
		},
		{
			// Backspace decrements the column from 2 to 1 in every model,
			// so the tab reaches the stop at 4 with three spaces.
			"latin1-backspace", []string{"LC_ALL=de_DE.ISO-8859-1"},
			[]byte{'a', 0xe4, '\b', '\t', 'x', '\n'}, []string{"-t", "4"},
			[]byte{'a', 0xe4, '\b', ' ', ' ', ' ', 'x', '\n'},
		},
		{
			"utf8-backspace", []string{"LC_ALL=C.UTF-8"},
			[]byte("ab\b\tx\n"), []string{"-t", "4"},
			[]byte("ab\b   x\n"),
		},
		{
			// Tabs past the last explicit stop become single spaces with
			// exact-byte text preserved around them.
			"latin1-past-last-stop", []string{"LC_ALL=de_DE.ISO-8859-1"},
			[]byte{0xe4, '\t', 'b', '\t', 'c', '\n'}, []string{"-t", "2,4"},
			[]byte{0xe4, ' ', 'b', ' ', 'c', '\n'},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, errOut, code := runExpandBytesEnv(t, tc.env, tc.input, tc.args...)
			if code != 0 || errOut != "" || !bytes.Equal(out, tc.want) {
				t.Fatalf(
					"code=%d stdout=% x stderr=%q, want stdout=% x",
					code, out, errOut, tc.want,
				)
			}
		})
	}
}

func TestIssue737LCCTypePrecedence(t *testing.T) {
	for _, tc := range []struct {
		name string
		env  []string
		want encodingMode
	}{
		{"lang", []string{"LANG=de_DE.ISO-8859-1"}, encodingLatin1},
		{
			"lc-ctype-over-lang",
			[]string{"LANG=de_DE.ISO-8859-1", "LC_CTYPE=C.UTF-8"}, encodingUTF8,
		},
		{
			"lc-all-over-category",
			[]string{"LANG=de_DE.ISO-8859-1", "LC_CTYPE=C.UTF-8", "LC_ALL=C"},
			encodingSingleByte,
		},
		{
			"empty-values-fall-through",
			[]string{"LANG=de_DE.iso88591", "LC_CTYPE=", "LC_ALL="}, encodingLatin1,
		},
		{"posix-utf8-alias", []string{"LC_CTYPE=POSIX.utf8"}, encodingUTF8},
	} {
		t.Run(tc.name, func(t *testing.T) {
			model, err := resolveCharacterModel(tc.env)
			if err != nil || model.encoding != tc.want {
				t.Fatalf("model=(%v, %v), want encoding %v", model, err, tc.want)
			}
		})
	}
}

type issue737PanicReader struct{}

func (issue737PanicReader) Read([]byte) (int, error) {
	panic("expand read input before validating LC_CTYPE")
}

func TestIssue737UnsupportedLocaleFailsBeforeInput(t *testing.T) {
	var out, errOut bytes.Buffer
	rc := &tool.RunContext{
		Ctx: context.Background(), Env: []string{"LC_ALL=unknown_LOCALE"},
		Stdio: tool.Stdio{In: issue737PanicReader{}, Out: &out, Err: &errOut},
	}
	code := run(rc, []string{"-t", "4"})
	if code != 1 || out.Len() != 0 || !strings.Contains(errOut.String(), "LC_CTYPE") {
		t.Fatalf("code=%d stdout=%q stderr=%q", code, out.Bytes(), errOut.String())
	}
}

func TestIssue737UnsupportedLocaleFailsBeforeOpeningOperand(t *testing.T) {
	var out, errOut bytes.Buffer
	rc := &tool.RunContext{
		Ctx: context.Background(), Dir: t.TempDir(), Env: []string{"LC_ALL=x-test"},
		Stdio: tool.Stdio{Out: &out, Err: &errOut},
	}
	code := run(rc, []string{"does-not-exist"})
	if code != 1 || out.Len() != 0 ||
		!strings.Contains(errOut.String(), `LC_CTYPE "x-test"`) ||
		strings.Contains(errOut.String(), "does-not-exist") {
		t.Fatalf("code=%d stdout=%q stderr=%q", code, out.Bytes(), errOut.String())
	}
}

func TestIssue737UsageErrorPrecedesLocaleValidation(t *testing.T) {
	var out, errOut bytes.Buffer
	rc := &tool.RunContext{
		Ctx: context.Background(), Env: []string{"LC_ALL=unknown_LOCALE"},
		Stdio: tool.Stdio{In: strings.NewReader("a\tb\n"), Out: &out, Err: &errOut},
	}
	code := run(rc, []string{"-t", "0"})
	if code != 2 || out.Len() != 0 || !strings.Contains(errOut.String(), "tab size cannot be 0") {
		t.Fatalf("code=%d stdout=%q stderr=%q", code, out.Bytes(), errOut.String())
	}
}

func TestIssue737ExitStatusMatrix(t *testing.T) {
	for _, tc := range []struct {
		name string
		env  []string
		args []string
		code int
		want string
	}{
		{"success", []string{"LC_ALL=C.UTF-8"}, []string{"-t", "4"}, 0, ""},
		{"invalid-tablist", []string{"LC_ALL=C.UTF-8"}, []string{"-t", "x"}, 2, "invalid character"},
		{"unsupported-locale", []string{"LC_ALL=no_SUCH"}, []string{"-t", "4"}, 1, "LC_CTYPE"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, errOut, code := runExpandBytesEnv(t, tc.env, []byte("a\tb\n"), tc.args...)
			if code != tc.code || !strings.Contains(errOut, tc.want) {
				t.Fatalf("code=%d stderr=%q, want code %d containing %q", code, errOut, tc.code, tc.want)
			}
			if tc.code != 0 && out != nil {
				t.Fatalf("stdout=% x on a failing invocation", out)
			}
		})
	}
}

type issue737ErrorReader struct {
	done bool
}

func (r *issue737ErrorReader) Read(p []byte) (int, error) {
	if r.done {
		return 0, errors.New("injected read failure")
	}
	r.done = true
	return copy(p, "a\tb"), nil
}

type issue737ShortWriter struct{}

func (issue737ShortWriter) Write(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	return len(p) - 1, nil
}

func TestIssue737ReadAndShortWriteErrors(t *testing.T) {
	model, err := resolveCharacterModel([]string{"LC_ALL=C.UTF-8"})
	if err != nil {
		t.Fatal(err)
	}
	tabs, err := parseTabStops([]string{"4"})
	if err != nil {
		t.Fatal(err)
	}
	if err := expandStreamModel(
		&issue737ErrorReader{}, io.Discard, tabs, false, model,
	); err == nil || !strings.Contains(err.Error(), "injected read failure") {
		t.Fatalf("read error = %v", err)
	}
	if err := expandStreamModel(
		strings.NewReader("a\tb\n"), issue737ShortWriter{}, tabs, false, model,
	); !errors.Is(err, io.ErrShortWrite) {
		t.Fatalf("write error = %v, want io.ErrShortWrite", err)
	}
}

func TestIssue737RunReportsReadAndShortWriteErrors(t *testing.T) {
	for _, tc := range []struct {
		name string
		in   io.Reader
		out  io.Writer
		want string
	}{
		{"read", &issue737ErrorReader{}, io.Discard, "injected read failure"},
		{"write", strings.NewReader("a\tb\n"), issue737ShortWriter{}, "short write"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var errOut bytes.Buffer
			rc := &tool.RunContext{
				Ctx: context.Background(), Env: []string{"LC_ALL=C.UTF-8"},
				Stdio: tool.Stdio{In: tc.in, Out: tc.out, Err: &errOut},
			}
			if code := run(rc, []string{"-t", "4"}); code != 1 || !strings.Contains(errOut.String(), tc.want) {
				t.Fatalf("code=%d stderr=%q, want diagnostic containing %q", code, errOut.String(), tc.want)
			}
		})
	}
}
