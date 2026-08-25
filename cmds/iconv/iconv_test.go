package iconvcmd

import (
	"bytes"
	"context"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/qiangli/coreutils/tool"
)

func invoke(t *testing.T, input string, args ...string) (int, []byte, string) {
	t.Helper()
	var out, err bytes.Buffer
	rc := &tool.RunContext{
		Ctx: context.Background(), Dir: t.TempDir(),
		Stdio: tool.Stdio{In: strings.NewReader(input), Out: &out, Err: &err},
	}
	return run(rc, args), out.Bytes(), err.String()
}

func TestUTF8ToISO88591(t *testing.T) {
	code, out, errout := invoke(t, "café\n", "-f", "UTF-8", "-t", "ISO-8859-1")
	if code != 0 || !bytes.Equal(out, []byte{'c', 'a', 'f', 0xe9, '\n'}) || errout != "" {
		t.Fatalf("code=%d out=% x err=%q", code, out, errout)
	}
}

func TestISO88591ToUTF8(t *testing.T) {
	code, out, errout := invoke(t, string([]byte{'c', 'a', 'f', 0xe9}), "--from-code=ISO-8859-1", "--to-code=UTF-8")
	if code != 0 || string(out) != "café" || errout != "" {
		t.Fatalf("code=%d out=%q err=%q", code, out, errout)
	}
}

func TestGB18030RoundTrip(t *testing.T) {
	code, encoded, errout := invoke(t, "中国", "-f", "UTF-8", "-t", "GB18030")
	if code != 0 || errout != "" {
		t.Fatalf("encode: code=%d err=%q", code, errout)
	}
	code, decoded, errout := invoke(t, string(encoded), "-f", "GB18030", "-t", "UTF-8")
	if code != 0 || string(decoded) != "中国" || errout != "" {
		t.Fatalf("decode: code=%d out=%q err=%q", code, decoded, errout)
	}
}

func TestFilesResolveAgainstRunContextDir(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(dir+"/one", []byte("one "), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dir+"/two", []byte("two"), 0o600); err != nil {
		t.Fatal(err)
	}
	var out, errout bytes.Buffer
	rc := &tool.RunContext{Ctx: context.Background(), Dir: dir, Stdio: tool.Stdio{Out: &out, Err: &errout}}
	if code := run(rc, []string{"-f", "UTF-8", "-t", "UTF-8", "one", "two"}); code != 0 {
		t.Fatalf("code=%d err=%q", code, errout.String())
	}
	if out.String() != "one two" {
		t.Fatalf("out=%q", out.String())
	}
}

func TestMalformedInputFailsAndSilentOnlySuppressesConversionMessage(t *testing.T) {
	for _, args := range [][]string{
		{"-f", "UTF-8", "-t", "ISO-8859-1"},
		{"-s", "-f", "UTF-8", "-t", "ISO-8859-1"},
	} {
		code, _, errout := invoke(t, string([]byte{0xff}), args...)
		if code != 1 {
			t.Fatalf("args=%v code=%d", args, code)
		}
		if args[0] == "-s" && errout != "" {
			t.Fatalf("silent stderr=%q", errout)
		}
		if args[0] != "-s" && !strings.Contains(errout, "standard input") {
			t.Fatalf("stderr=%q", errout)
		}
	}
}

func TestUnrepresentableOutputFails(t *testing.T) {
	code, _, errout := invoke(t, "€", "-f", "UTF-8", "-t", "ISO-8859-1")
	if code != 1 || !strings.Contains(errout, "standard input") {
		t.Fatalf("code=%d err=%q", code, errout)
	}
}

func TestUnsupportedEncodingFailsLoudly(t *testing.T) {
	code, _, errout := invoke(t, "", "-f", "not-a-charset", "-t", "UTF-8")
	if code != 1 || !strings.Contains(errout, "unsupported encoding") {
		t.Fatalf("encoding: code=%d err=%q", code, errout)
	}
}

// POSIX -c omits invalid input characters while preserving the conversion's
// non-zero status; -s controls only the corresponding diagnostic.
func TestDiscardInvalidOmitsUntranslatableCharacters(t *testing.T) {
	// A byte invalid in the input codeset (0xff is not valid UTF-8) is dropped;
	// the surrounding valid text still converts. POSIX explicitly says -c must
	// not alter the exit status, so conversion loss remains non-zero.
	code, out, errout := invoke(t, "a"+string([]byte{0xff})+"b", "-c", "-f", "UTF-8", "-t", "UTF-8")
	if code != 1 || errout != "" || string(out) != "ab" {
		t.Fatalf("invalid input: code=%d out=%q err=%q", code, string(out), errout)
	}
	// A character with no representation in the output codeset (€ is not in
	// ISO-8859-1) is likewise omitted while retaining non-zero status.
	code, out, errout = invoke(t, "a€b", "-c", "-f", "UTF-8", "-t", "ISO-8859-1")
	if code != 1 || errout != "" || string(out) != "ab" {
		t.Fatalf("unrepresentable output: code=%d out=%q err=%q", code, string(out), errout)
	}
	// -s cannot launder that status either.
	code, out, errout = invoke(t, "a"+string([]byte{0xff})+"b", "-c", "-s", "-f", "UTF-8", "-t", "UTF-8")
	if code != 1 || errout != "" || string(out) != "ab" {
		t.Fatalf("-c -s status: code=%d out=%q err=%q", code, string(out), errout)
	}
	// Without -c the same unrepresentable input is a loud failure.
	code, _, errout = invoke(t, "a€b", "-f", "UTF-8", "-t", "ISO-8859-1")
	if code != 1 || !strings.Contains(errout, "standard input") {
		t.Fatalf("no -c must still fail: code=%d err=%q", code, errout)
	}
}

// POSIX synopsis: -f and/or -t may be omitted, in which case the codeset of the
// current locale (LC_CTYPE) is used — this is NOT a usage error. The
// deterministic default locale is POSIX, whose codeset is US-ASCII.
func TestOmittedEncodingUsesLocaleCodeset(t *testing.T) {
	// Omitted -f: default input codeset (US-ASCII) converts to UTF-8.
	code, out, errout := invoke(t, "abc", "-t", "UTF-8")
	if code != 0 || errout != "" || string(out) != "abc" {
		t.Fatalf("omitted -f: code=%d out=%q err=%q", code, string(out), errout)
	}
	// Omitted -t: default output codeset (US-ASCII) accepts ASCII input.
	code, out, errout = invoke(t, "abc", "-f", "UTF-8")
	if code != 0 || errout != "" || string(out) != "abc" {
		t.Fatalf("omitted -t: code=%d out=%q err=%q", code, string(out), errout)
	}
	// Both omitted matches no POSIX synopsis and fails as a usage error.
	code, out, errout = invoke(t, "abc")
	if code != 2 || len(out) != 0 || !strings.Contains(errout, "at least one") {
		t.Fatalf("both omitted must fail closed: code=%d out=%q err=%q", code, string(out), errout)
	}
	// The default OUTPUT codeset is genuinely US-ASCII, not a silent UTF-8: a
	// non-ASCII character has no ASCII representation and fails to encode.
	code, _, errout = invoke(t, "é", "-f", "UTF-8")
	if code != 1 || !strings.Contains(errout, "standard input") {
		t.Fatalf("non-ASCII to default US-ASCII output must fail: code=%d err=%q", code, errout)
	}
}

// An LC_CTYPE codeset selects the omitted-encoding default.
func TestOmittedEncodingHonorsLocaleCodeset(t *testing.T) {
	var out, errout bytes.Buffer
	rc := &tool.RunContext{
		Ctx: context.Background(), Dir: t.TempDir(),
		Env:   []string{"LC_CTYPE=de_DE.ISO-8859-1"},
		Stdio: tool.Stdio{In: strings.NewReader(string([]byte{0xe9})), Out: &out, Err: &errout},
	}
	// Omitted -f resolves to ISO-8859-1, so 0xe9 decodes to é and encodes as UTF-8.
	if code := run(rc, []string{"-t", "UTF-8"}); code != 0 || out.String() != "é" {
		t.Fatalf("code=%d out=%q err=%q", code, out.String(), errout.String())
	}
}

func TestOmittedEncodingLocaleMappingAndFailClosed(t *testing.T) {
	for _, tc := range []struct {
		name string
		env  []string
		in   string
		args []string
		want string
	}{
		{"C.UTF-8", []string{"LC_CTYPE=C.UTF-8"}, "é", []string{"-t", "UTF-8"}, "é"},
		{"C.utf8 alias", []string{"LC_CTYPE=C.utf8"}, "é", []string{"-t", "UTF-8"}, "é"},
		{"unqualified de_DE", []string{"LC_CTYPE=de_DE"}, string([]byte{0xe9}), []string{"-t", "UTF-8"}, "é"},
		{"explicit codeset on otherwise uncarried locale", []string{"LC_CTYPE=fr_FR.UTF-8"}, "é", []string{"-t", "UTF-8"}, "é"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var out, errout bytes.Buffer
			rc := &tool.RunContext{Ctx: context.Background(), Dir: t.TempDir(), Env: tc.env,
				Stdio: tool.Stdio{In: strings.NewReader(tc.in), Out: &out, Err: &errout}}
			if code := run(rc, tc.args); code != 0 || out.String() != tc.want || errout.Len() != 0 {
				t.Fatalf("code=%d out=%q err=%q", code, out.String(), errout.String())
			}
		})
	}

	var errout bytes.Buffer
	rc := &tool.RunContext{Ctx: context.Background(), Dir: t.TempDir(), Env: []string{"LC_CTYPE=en_US"},
		Stdio: tool.Stdio{In: panicReader{}, Out: panicWriter{}, Err: &errout}}
	if code := run(rc, []string{"-t", "UTF-8"}); code != 1 || !strings.Contains(errout.String(), "en_US") || !strings.Contains(errout.String(), "no carried default codeset") {
		t.Fatalf("unsupported unqualified locale: code=%d err=%q", code, errout.String())
	}
}

type oneByteReader struct{ io.Reader }

func (r oneByteReader) Read(p []byte) (int, error) {
	if len(p) > 1 {
		p = p[:1]
	}
	return r.Reader.Read(p)
}

func TestDiscardInvalidPreservesLiteralReplacementAndStreamingState(t *testing.T) {
	for _, tc := range []struct {
		name, from string
		input      []byte
	}{
		{"UTF-8", "UTF-8", append(append([]byte("x�"), 0xff), 'y')},
		{"UTF-16BE", "UTF-16BE", []byte{0, 'x', 0xff, 0xfd, 0xd8, 0, 0, 'y'}},
		{"GB18030", "GB18030", []byte{'x', 0x84, 0x31, 0xa4, 0x37, 0xff, 'y'}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var out, errout bytes.Buffer
			rc := &tool.RunContext{Ctx: context.Background(), Dir: t.TempDir(),
				Stdio: tool.Stdio{In: oneByteReader{bytes.NewReader(tc.input)}, Out: &out, Err: &errout}}
			if code := run(rc, []string{"-c", "-f", tc.from, "-t", "UTF-8"}); code != 1 || out.String() != "x�y" || errout.Len() != 0 {
				t.Fatalf("code=%d out=%q err=%q", code, out.String(), errout.String())
			}
		})
	}

	code, encoded, errout := invoke(t, "日本€日本", "-c", "-f", "UTF-8", "-t", "ISO-2022-JP")
	if code != 1 || errout != "" {
		t.Fatalf("stateful encode: code=%d err=%q", code, errout)
	}
	code, decoded, errout := invoke(t, string(encoded), "-f", "ISO-2022-JP", "-t", "UTF-8")
	if code != 0 || string(decoded) != "日本日本" || errout != "" {
		t.Fatalf("stateful round trip: code=%d out=%q err=%q encoded=%x", code, decoded, errout, encoded)
	}
}

func TestListEncodings(t *testing.T) {
	for _, flag := range []string{"-l", "--list"} {
		code, out, errout := invoke(t, "", flag)
		if code != 0 || errout != "" {
			t.Fatalf("flag=%s: code=%d err=%q", flag, code, errout)
		}
		lines := strings.Split(strings.TrimSpace(string(out)), "\n")
		if len(lines) == 0 {
			t.Fatalf("flag=%s: empty list", flag)
		}
		foundUTF8 := false
		for _, l := range lines {
			if l == "UTF-8" {
				foundUTF8 = true
				break
			}
		}
		if !foundUTF8 {
			t.Fatalf("flag=%s: UTF-8 not found in output: %q", flag, string(out))
		}
	}
}

func TestEncodingAliases(t *testing.T) {
	tests := []struct {
		input string
		from  string
		to    string
		want  string
	}{
		{"abc", "utf8", "ISO-8859-1", "abc"},
		{"abc", "UTF8", "ISO-8859-1", "abc"},
		{"abc", "ASCII", "UTF-8", "abc"},
		{"abc", "CP1252", "UTF-8", "abc"},
		{"abc", "LATIN1", "UTF-8", "abc"},
		{"\xff\xfea\x00b\x00c\x00", "UTF16", "UTF-8", "abc"},
		{"\x00a\x00b\x00c", "UTF16BE", "UTF-8", "abc"},
		{"a\x00b\x00c\x00", "UTF16LE", "UTF-8", "abc"},
	}

	for _, tc := range tests {
		code, out, errout := invoke(t, tc.input, "-f", tc.from, "-t", tc.to)
		if code != 0 || errout != "" || string(out) != tc.want {
			t.Fatalf("from=%q to=%q code=%d out=%q err=%q", tc.from, tc.to, code, string(out), errout)
		}
	}
}

type panicReader struct{}

func (panicReader) Read([]byte) (int, error) {
	panic("reader accessed before encoding validation")
}

type panicWriter struct{}

func (panicWriter) Write([]byte) (int, error) {
	panic("writer accessed before encoding validation")
}

func TestSuffixRejectionPreIO(t *testing.T) {
	tests := []struct {
		from string
		to   string
		bad  string
	}{
		// Standard uppercase suffixes
		{"UTF-8//IGNORE", "ISO-8859-1", "UTF-8//IGNORE"},
		{"UTF-8//TRANSLIT", "ISO-8859-1", "UTF-8//TRANSLIT"},
		{"UTF-8", "ISO-8859-1//IGNORE", "ISO-8859-1//IGNORE"},
		{"UTF-8", "ISO-8859-1//TRANSLIT", "ISO-8859-1//TRANSLIT"},
		// Lowercase suffixes
		{"utf-8//ignore", "ISO-8859-1", "utf-8//ignore"},
		{"iso-8859-1//translit", "UTF-8", "iso-8859-1//translit"},
		{"UTF-8", "iso-8859-1//ignore", "iso-8859-1//ignore"},
		// Multiple suffixes
		{"UTF-8//IGNORE//TRANSLIT", "ISO-8859-1", "UTF-8//IGNORE//TRANSLIT"},
		{"UTF-8", "ISO-8859-1//TRANSLIT//IGNORE", "ISO-8859-1//TRANSLIT//IGNORE"},
		// Unknown suffixes
		{"UTF-8//UNKNOWN", "ISO-8859-1", "UTF-8//UNKNOWN"},
		{"UTF-8", "ISO-8859-1//FOO", "ISO-8859-1//FOO"},
	}

	for _, tc := range tests {
		var errout bytes.Buffer
		rc := &tool.RunContext{
			Ctx: context.Background(),
			Dir: t.TempDir(),
			Stdio: tool.Stdio{
				In:  panicReader{},
				Out: panicWriter{},
				Err: &errout,
			},
		}
		code := run(rc, []string{"-f", tc.from, "-t", tc.to})
		if code != 1 {
			t.Fatalf("from=%q to=%q: expected code 1, got %d", tc.from, tc.to, code)
		}
		expectedErr := "iconv: unsupported encoding \"" + tc.bad + "\"\n"
		if errout.String() != expectedErr {
			t.Fatalf("from=%q to=%q: expected err %q, got %q", tc.from, tc.to, expectedErr, errout.String())
		}
	}
}

func TestAllSupportedEncodingsAreResolvable(t *testing.T) {
	for _, encName := range supportedEncodings {
		enc, err := lookupEncoding(encName)
		if err != nil || enc == nil {
			t.Fatalf("encoding %q failed to resolve: %v", encName, err)
		}
	}
}
