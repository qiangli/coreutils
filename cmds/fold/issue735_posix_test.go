package foldcmd

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/qiangli/coreutils/tool"
)

func runFoldBytesEnv(
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

func TestIssue735LocaleCharacterBoundariesPreserveOriginalBytes(t *testing.T) {
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
			"latin1-columns", latin1, []byte{0xe4, 'b', 'c', '\n'},
			[]string{"-w", "2"}, []byte{0xe4, 'b', '\n', 'c', '\n'},
		},
		{
			"latin1-characters", latin1, []byte{0xe4, 'b', 'c', '\n'},
			[]string{"-c", "-w", "2"}, []byte{0xe4, 'b', '\n', 'c', '\n'},
		},
		{
			"latin1-bytes", latin1, []byte{0xe4, 'b', 'c', '\n'},
			[]string{"-b", "-w", "2"}, []byte{0xe4, 'b', '\n', 'c', '\n'},
		},
		{
			"utf8-columns", utf8Locale, []byte("äbc\n"),
			[]string{"-w", "2"}, []byte("äb\nc\n"),
		},
		{
			"utf8-characters", utf8Locale, []byte("äbc\n"),
			[]string{"-c", "-w", "2"}, []byte("äb\nc\n"),
		},
		{
			"utf8-bytes-no-split", utf8Locale, []byte("äbc\n"),
			[]string{"-b", "-w", "2"}, []byte("ä\nbc\n"),
		},
		{
			"utf8-wide-column", utf8Locale, []byte("界a\n"),
			[]string{"-w", "2"}, []byte("界\na\n"),
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, errOut, code := runFoldBytesEnv(t, tc.env, tc.input, tc.args...)
			if code != 0 || errOut != "" || !bytes.Equal(out, tc.want) {
				t.Fatalf(
					"code=%d stdout=% x stderr=%q, want stdout=% x",
					code, out, errOut, tc.want,
				)
			}
		})
	}
}

func TestIssue735MalformedAndCLocaleBytesAreNeverReencoded(t *testing.T) {
	for _, tc := range []struct {
		name string
		env  []string
	}{
		{"malformed-utf8", []string{"LC_ALL=C.UTF-8"}},
		{"c-locale-byte-characters", []string{"LC_ALL=C"}},
		{"posix-locale-byte-characters", []string{"LC_ALL=POSIX"}},
		{"default-posix-byte-characters", nil},
	} {
		for _, mode := range []struct {
			name string
			args []string
		}{
			{"columns", []string{"-w", "2"}},
			{"characters", []string{"-c", "-w", "2"}},
			{"bytes", []string{"-b", "-w", "2"}},
		} {
			t.Run(tc.name+"/"+mode.name, func(t *testing.T) {
				input := []byte{0xff, 0xfe, 'x', '\n'}
				want := []byte{0xff, 0xfe, '\n', 'x', '\n'}
				out, errOut, code := runFoldBytesEnv(t, tc.env, input, mode.args...)
				if code != 0 || errOut != "" || !bytes.Equal(out, want) {
					t.Fatalf(
						"code=%d stdout=% x stderr=%q, want stdout=% x",
						code, out, errOut, want,
					)
				}
			})
		}
	}
}

func TestIssue735SpacesAndControlCharactersAcrossLocales(t *testing.T) {
	for _, tc := range []struct {
		name  string
		env   []string
		input []byte
		args  []string
		want  []byte
	}{
		{
			"latin1-space-break", []string{"LC_ALL=de_DE.iso88591"},
			[]byte{'a', 'a', ' ', 0xe4, 'b', 'b'}, []string{"-s", "-w", "3"},
			[]byte{'a', 'a', ' ', '\n', 0xe4, 'b', 'b'},
		},
		{
			"utf8-space-break", []string{"LC_ALL=C.UTF-8"},
			[]byte("aa äbb"), []string{"-s", "-c", "-w", "3"},
			[]byte("aa \näbb"),
		},
		{
			"latin1-controls", []string{"LC_ALL=de_DE.ISO-8859-1"},
			[]byte{'a', 0xe4, '\b', 'b', '\r', 'c', 'd'}, []string{"-w", "2"},
			[]byte{'a', 0xe4, '\b', 'b', '\r', 'c', 'd'},
		},
		{
			"byte-mode-controls", []string{"LC_ALL=C.UTF-8"},
			[]byte{'a', '\t', '\b', '\r'}, []string{"-b", "-w", "2"},
			[]byte{'a', '\t', '\n', '\b', '\r'},
		},
		{
			"latin1-byte-mode-space-break", []string{"LC_ALL=de_DE.ISO-8859-1"},
			[]byte{'a', ' ', 0xe4, 'b'}, []string{"-b", "-s", "-w", "2"},
			[]byte{'a', ' ', '\n', 0xe4, 'b'},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, errOut, code := runFoldBytesEnv(t, tc.env, tc.input, tc.args...)
			if code != 0 || errOut != "" || !bytes.Equal(out, tc.want) {
				t.Fatalf(
					"code=%d stdout=% x stderr=%q, want stdout=% x",
					code, out, errOut, tc.want,
				)
			}
		})
	}
}

func TestIssue735LCCTypePrecedence(t *testing.T) {
	for _, tc := range []struct {
		name string
		env  []string
		want encodingMode
	}{
		{
			"lang", []string{"LANG=de_DE.ISO-8859-1"}, encodingLatin1,
		},
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
		{
			"posix-utf8-alias", []string{"LC_CTYPE=POSIX.utf8"}, encodingUTF8,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			model, err := resolveCharacterModel(tc.env)
			if err != nil || model.encoding != tc.want {
				t.Fatalf("model=(%v, %v), want encoding %v", model, err, tc.want)
			}
		})
	}
}

type issue735PanicReader struct{}

func (issue735PanicReader) Read([]byte) (int, error) {
	panic("fold read input before validating LC_CTYPE")
}

func TestIssue735UnsupportedLocaleFailsBeforeInput(t *testing.T) {
	var out, errOut bytes.Buffer
	rc := &tool.RunContext{
		Ctx: context.Background(), Env: []string{"LC_ALL=unknown_LOCALE"},
		Stdio: tool.Stdio{In: issue735PanicReader{}, Out: &out, Err: &errOut},
	}
	code := run(rc, []string{"-w", "2"})
	if code != 1 || out.Len() != 0 || !strings.Contains(errOut.String(), "LC_CTYPE") {
		t.Fatalf("code=%d stdout=%q stderr=%q", code, out.Bytes(), errOut.String())
	}
}

func TestIssue735UnsupportedLocaleFailsBeforeOpeningOperand(t *testing.T) {
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

type issue735ErrorReader struct {
	done bool
}

func (r *issue735ErrorReader) Read(p []byte) (int, error) {
	if r.done {
		return 0, errors.New("injected read failure")
	}
	r.done = true
	return copy(p, "ab"), nil
}

type issue735ShortWriter struct{}

func (issue735ShortWriter) Write(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	return len(p) - 1, nil
}

func TestIssue735ReadAndShortWriteErrors(t *testing.T) {
	model, err := resolveCharacterModel([]string{"LC_ALL=C.UTF-8"})
	if err != nil {
		t.Fatal(err)
	}
	if err := foldStream(
		&issue735ErrorReader{}, io.Discard, 80, countColumns, false, model,
	); err == nil || !strings.Contains(err.Error(), "injected read failure") {
		t.Fatalf("read error = %v", err)
	}
	if err := foldStream(
		strings.NewReader("ab\n"), issue735ShortWriter{}, 80,
		countColumns, false, model,
	); !errors.Is(err, io.ErrShortWrite) {
		t.Fatalf("write error = %v, want io.ErrShortWrite", err)
	}
}

func TestIssue735RunReportsReadAndShortWriteErrors(t *testing.T) {
	for _, tc := range []struct {
		name string
		in   io.Reader
		out  io.Writer
		want string
	}{
		{"read", &issue735ErrorReader{}, io.Discard, "injected read failure"},
		{"write", strings.NewReader("ab\n"), issue735ShortWriter{}, "short write"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var errOut bytes.Buffer
			rc := &tool.RunContext{
				Ctx: context.Background(), Env: []string{"LC_ALL=C.UTF-8"},
				Stdio: tool.Stdio{In: tc.in, Out: tc.out, Err: &errOut},
			}
			if code := run(rc, nil); code != 1 || !strings.Contains(errOut.String(), tc.want) {
				t.Fatalf("code=%d stderr=%q, want diagnostic containing %q", code, errOut.String(), tc.want)
			}
		})
	}
}
