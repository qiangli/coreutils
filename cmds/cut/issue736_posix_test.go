package cutcmd

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/qiangli/coreutils/tool"
)

// runCutBytesEnv runs cut with an explicit invocation environment and
// returns the exact stdout bytes: every assertion in this file is an
// original-byte comparison, never a string/rune-level one.
func runCutBytesEnv(
	t *testing.T, env []string, input []byte, args ...string,
) ([]byte, string, int) {
	t.Helper()
	var out, errOut bytes.Buffer
	rc := &tool.RunContext{
		Ctx: context.Background(), Dir: t.TempDir(), Env: env,
		Stdio: tool.Stdio{In: bytes.NewReader(input), Out: &out, Err: &errOut},
	}
	code := cmd.Run(rc, args)
	return out.Bytes(), errOut.String(), code
}

var (
	cLocale      = []string{"LC_ALL=C"}
	posixLocale  = []string{"LC_ALL=POSIX"}
	utf8Locale   = []string{"LC_ALL=C.UTF-8"}
	latin1Locale = []string{"LC_ALL=de_DE.ISO-8859-1"}
)

func TestIssue736CharacterSpansFollowLCCType(t *testing.T) {
	// "éx日" is 0xc3 0xa9 'x' 0xe6 0x97 0xa5.
	for _, tc := range []struct {
		name  string
		env   []string
		input []byte
		args  []string
		want  []byte
	}{
		// Single-byte locales: -c positions are byte positions and the
		// selected spans are the exact source bytes.
		{
			"default-posix-first-byte", nil, []byte("éx\n"),
			[]string{"-c", "1"}, []byte{0xc3, '\n'},
		},
		{
			"c-locale-first-byte", cLocale, []byte("éx\n"),
			[]string{"-c", "1"}, []byte{0xc3, '\n'},
		},
		{
			"posix-locale-span", posixLocale, []byte("éx\n"),
			[]string{"-c", "1-2"}, []byte("é\n"),
		},
		{
			"c-locale-positions", cLocale, []byte("éx日\n"),
			[]string{"-c", "1,3"}, []byte{0xc3, 'x', '\n'},
		},
		{
			"latin1-first-character", latin1Locale, []byte{0xe4, 'b', 'c', '\n'},
			[]string{"-c", "1"}, []byte{0xe4, '\n'},
		},
		{
			"latin1-range", latin1Locale, []byte{0xe4, 'b', 'c', '\n'},
			[]string{"-c", "2-3"}, []byte("bc\n"),
		},
		// UTF-8 locales: -c positions are characters carrying their
		// original encoded bytes.
		{
			"utf8-first-character", utf8Locale, []byte("éx\n"),
			[]string{"-c", "1"}, []byte("é\n"),
		},
		{
			"utf8-second-character", utf8Locale, []byte("éx\n"),
			[]string{"-c", "2"}, []byte("x\n"),
		},
		{
			"utf8-list", utf8Locale, []byte("éx日\n"),
			[]string{"-c", "1,3"}, []byte("é日\n"),
		},
		{
			"utf8-open-range", utf8Locale, []byte("日éx\n"),
			[]string{"-c", "2-"}, []byte("éx\n"),
		},
		{
			"utf8-complement", utf8Locale, []byte("éx日\n"),
			[]string{"--complement", "-c", "2"}, []byte("é日\n"),
		},
		{
			"posix-utf8-alias", []string{"LC_ALL=POSIX.utf8"}, []byte("éx\n"),
			[]string{"-c", "1"}, []byte("é\n"),
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, errOut, code := runCutBytesEnv(t, tc.env, tc.input, tc.args...)
			if code != 0 || errOut != "" || !bytes.Equal(out, tc.want) {
				t.Fatalf(
					"code=%d stdout=% x stderr=%q, want stdout=% x",
					code, out, errOut, tc.want,
				)
			}
		})
	}
}

func TestIssue736MalformedBytesAreNeverReencoded(t *testing.T) {
	// 0xff and 0xfe can begin no UTF-8 sequence: each malformed byte is
	// one character position and its original byte must reach stdout —
	// never the UTF-8 encoding of U+FFFD (0xef 0xbf 0xbd).
	replacement := []byte{0xef, 0xbf, 0xbd}
	for _, tc := range []struct {
		name  string
		input []byte
		args  []string
		want  []byte
	}{
		{
			"chars-first-malformed", []byte{0xff, 0xfe, 'x', '\n'},
			[]string{"-c", "1"}, []byte{0xff, '\n'},
		},
		{
			"chars-malformed-pair", []byte{0xff, 0xfe, 'x', '\n'},
			[]string{"-c", "1-2"}, []byte{0xff, 0xfe, '\n'},
		},
		{
			"chars-after-malformed", []byte{0xff, 0xfe, 'x', '\n'},
			[]string{"-c", "3"}, []byte{'x', '\n'},
		},
		{
			"chars-truncated-sequence", []byte{0xc3, 'x', '\n'},
			[]string{"-c", "1"}, []byte{0xc3, '\n'},
		},
		{
			"bytes-nosplit-malformed", []byte{0xff, 0xfe, 'x', '\n'},
			[]string{"-b", "1", "-n"}, []byte{0xff, '\n'},
		},
		{
			"bytes-malformed", []byte{0xff, 0xfe, 'x', '\n'},
			[]string{"-b", "2-3"}, []byte{0xfe, 'x', '\n'},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, errOut, code := runCutBytesEnv(t, utf8Locale, tc.input, tc.args...)
			if code != 0 || errOut != "" || !bytes.Equal(out, tc.want) {
				t.Fatalf(
					"code=%d stdout=% x stderr=%q, want stdout=% x",
					code, out, errOut, tc.want,
				)
			}
			if bytes.Contains(out, replacement) {
				t.Fatalf("stdout % x contains the U+FFFD encoding", out)
			}
		})
	}
}

func TestIssue736ByteNoSplitFollowsLCCType(t *testing.T) {
	for _, tc := range []struct {
		name  string
		env   []string
		input []byte
		args  []string
		want  []byte
	}{
		// UTF-8: -b ranges shrink/expand to character boundaries.
		{
			"utf8-drops-partial-first", utf8Locale, []byte("éx\n"),
			[]string{"-b", "1", "-n"}, []byte("\n"),
		},
		{
			"utf8-expands-into-character", utf8Locale, []byte("éx\n"),
			[]string{"-b", "2", "-n"}, []byte("é\n"),
		},
		{
			"utf8-trims-trailing-partial", utf8Locale, []byte("x日\n"),
			[]string{"-b", "1-3", "-n"}, []byte("x\n"),
		},
		// Single-byte locales: -n is a no-op and -b keeps exact bytes.
		{
			"posix-default-noop", nil, []byte("éx\n"),
			[]string{"-b", "1", "-n"}, []byte{0xc3, '\n'},
		},
		{
			"c-locale-noop", cLocale, []byte("éx\n"),
			[]string{"-b", "1", "-n"}, []byte{0xc3, '\n'},
		},
		{
			"latin1-noop", latin1Locale, []byte{0xe4, 'b', '\n'},
			[]string{"-b", "1", "-n"}, []byte{0xe4, '\n'},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, errOut, code := runCutBytesEnv(t, tc.env, tc.input, tc.args...)
			if code != 0 || errOut != "" || !bytes.Equal(out, tc.want) {
				t.Fatalf(
					"code=%d stdout=% x stderr=%q, want stdout=% x",
					code, out, errOut, tc.want,
				)
			}
		})
	}
}

func TestIssue736MultibyteDelimiter(t *testing.T) {
	for _, tc := range []struct {
		name  string
		env   []string
		input []byte
		args  []string
		want  []byte
	}{
		{
			"utf8-two-byte-delim", utf8Locale, []byte("aäbäc\n"),
			[]string{"-d", "ä", "-f", "2"}, []byte("b\n"),
		},
		{
			"utf8-default-output-delim", utf8Locale, []byte("aäbäc\n"),
			[]string{"-d", "ä", "-f", "1,3"}, []byte("aäc\n"),
		},
		{
			"utf8-output-delim-override", utf8Locale, []byte("aäbäc\n"),
			[]string{"-d", "ä", "-f", "1,3", "--output-delimiter", ":"},
			[]byte("a:c\n"),
		},
		{
			"utf8-three-byte-delim", utf8Locale, []byte("a日b日c\n"),
			[]string{"-d", "日", "-f", "2-"}, []byte("b日c\n"),
		},
		{
			"utf8-complement", utf8Locale, []byte("aäbäc\n"),
			[]string{"--complement", "-d", "ä", "-f", "2"}, []byte("aäc\n"),
		},
		{
			"utf8-passthrough-without-delim", utf8Locale, []byte("plain\n"),
			[]string{"-d", "ä", "-f", "2"}, []byte("plain\n"),
		},
		{
			"utf8-only-delimited-suppresses", utf8Locale,
			[]byte("aäb\nplain\n"), []string{"-d", "ä", "-f", "2", "-s"},
			[]byte("b\n"),
		},
		{
			"latin1-high-byte-delim", latin1Locale,
			[]byte{'a', 0xe4, 'b', 0xe4, 'c', '\n'},
			[]string{"-d", "\xe4", "-f", "2"}, []byte("b\n"),
		},
		{
			"latin1-default-output-delim", latin1Locale,
			[]byte{'a', 0xe4, 'b', 0xe4, 'c', '\n'},
			[]string{"-d", "\xe4", "-f", "1,3"},
			[]byte{'a', 0xe4, 'c', '\n'},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, errOut, code := runCutBytesEnv(t, tc.env, tc.input, tc.args...)
			if code != 0 || errOut != "" || !bytes.Equal(out, tc.want) {
				t.Fatalf(
					"code=%d stdout=% x stderr=%q, want stdout=% x",
					code, out, errOut, tc.want,
				)
			}
		})
	}
}

func TestIssue736DelimiterMustBeOneCharacterInLocale(t *testing.T) {
	for _, tc := range []struct {
		name  string
		env   []string
		delim string
	}{
		{"c-locale-multibyte", cLocale, "ä"},
		{"posix-default-multibyte", nil, "ä"},
		{"latin1-two-characters", latin1Locale, "ä"}, // 0xc3 0xa4 = two Latin-1 chars
		{"utf8-two-ascii", utf8Locale, "ab"},
		{"utf8-two-characters", utf8Locale, "äb"},
		{"utf8-invalid-byte", utf8Locale, "\xe4"},
		{"utf8-truncated-sequence", utf8Locale, "\xc3"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, errOut, code := runCutBytesEnv(
				t, tc.env, []byte("a:b\n"), "-d", tc.delim, "-f", "1",
			)
			if code != 2 || len(out) != 0 ||
				!strings.Contains(errOut, "the delimiter must be a single character") {
				t.Fatalf("code=%d stdout=%q stderr=%q", code, out, errOut)
			}
		})
	}
}

func TestIssue736LCCTypePrecedence(t *testing.T) {
	input := []byte("éx\n")
	utf8First := []byte("é\n")
	byteFirst := []byte{0xc3, '\n'}
	for _, tc := range []struct {
		name string
		env  []string
		want []byte
	}{
		{"lang-latin1", []string{"LANG=de_DE.ISO-8859-1"}, byteFirst},
		{"lang-utf8", []string{"LANG=C.UTF-8"}, utf8First},
		{
			"lc-ctype-over-lang",
			[]string{"LANG=de_DE.ISO-8859-1", "LC_CTYPE=C.UTF-8"}, utf8First,
		},
		{
			"lc-all-over-category",
			[]string{"LANG=de_DE.ISO-8859-1", "LC_CTYPE=C.UTF-8", "LC_ALL=C"},
			byteFirst,
		},
		{
			"empty-values-fall-through",
			[]string{"LANG=C.UTF-8", "LC_CTYPE=", "LC_ALL="}, utf8First,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, errOut, code := runCutBytesEnv(t, tc.env, input, "-c", "1")
			if code != 0 || errOut != "" || !bytes.Equal(out, tc.want) {
				t.Fatalf(
					"code=%d stdout=% x stderr=%q, want stdout=% x",
					code, out, errOut, tc.want,
				)
			}
		})
	}
}

type issue736PanicReader struct{}

func (issue736PanicReader) Read([]byte) (int, error) {
	panic("cut read input before validating LC_CTYPE")
}

func TestIssue736UnsupportedLocaleFailsBeforeInput(t *testing.T) {
	for _, args := range [][]string{
		{"-c", "1"},
		{"-b", "1"},
		{"-f", "1", "-d", ":"},
	} {
		var out, errOut bytes.Buffer
		rc := &tool.RunContext{
			Ctx: context.Background(), Env: []string{"LC_ALL=unknown_LOCALE"},
			Stdio: tool.Stdio{In: issue736PanicReader{}, Out: &out, Err: &errOut},
		}
		code := cmd.Run(rc, args)
		if code != 1 || out.Len() != 0 ||
			!strings.Contains(errOut.String(), `LC_CTYPE "unknown_LOCALE" is unavailable`) {
			t.Fatalf(
				"cut %v: code=%d stdout=%q stderr=%q",
				args, code, out.Bytes(), errOut.String(),
			)
		}
	}
}

func TestIssue736UnsupportedLocaleFailsBeforeOpeningOperand(t *testing.T) {
	var out, errOut bytes.Buffer
	rc := &tool.RunContext{
		Ctx: context.Background(), Dir: t.TempDir(), Env: []string{"LC_ALL=x-test"},
		Stdio: tool.Stdio{Out: &out, Err: &errOut},
	}
	code := cmd.Run(rc, []string{"-c", "1", "does-not-exist"})
	if code != 1 || out.Len() != 0 ||
		!strings.Contains(errOut.String(), `LC_CTYPE "x-test"`) ||
		strings.Contains(errOut.String(), "does-not-exist") {
		t.Fatalf("code=%d stdout=%q stderr=%q", code, out.Bytes(), errOut.String())
	}
}

func TestIssue736TextFileBoundaries(t *testing.T) {
	// A final line missing the POSIX text-file trailing newline is still
	// processed, and the output reproduces exact input bytes without
	// appending a newline — including when the boundary byte is a
	// multibyte character or a malformed sequence.
	for _, tc := range []struct {
		name  string
		env   []string
		input []byte
		args  []string
		want  []byte
	}{
		{
			"utf8-no-trailing-newline", utf8Locale, []byte("aä"),
			[]string{"-c", "2"}, []byte("ä"),
		},
		{
			"utf8-truncated-final-sequence", utf8Locale, []byte{'a', 0xc3},
			[]string{"-c", "1-2"}, []byte{'a', 0xc3},
		},
		{
			"utf8-malformed-final-byte", utf8Locale, []byte{0xff},
			[]string{"-c", "1"}, []byte{0xff},
		},
		{
			"latin1-no-trailing-newline", latin1Locale, []byte{'a', 0xe4},
			[]string{"-c", "2"}, []byte{0xe4},
		},
		{
			"posix-no-trailing-newline", nil, []byte("aé"),
			[]string{"-b", "2-", "-n"}, []byte("é"),
		},
		{
			"utf8-zero-terminated-records", utf8Locale,
			[]byte("äx\x00日y\x00"), []string{"-z", "-c", "1"},
			[]byte("ä\x00日\x00"),
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, errOut, code := runCutBytesEnv(t, tc.env, tc.input, tc.args...)
			if code != 0 || errOut != "" || !bytes.Equal(out, tc.want) {
				t.Fatalf(
					"code=%d stdout=% x stderr=%q, want stdout=% x",
					code, out, errOut, tc.want,
				)
			}
		})
	}
}

func TestIssue736LongLineCharacterSelection(t *testing.T) {
	// A line longer than the initial 4 KiB read buffer, with a multibyte
	// character spanning the original buffer edge: character selection
	// still sees one contiguous line and exact bytes.
	prefix := bytes.Repeat([]byte{'a'}, 4095)
	input := append(append([]byte{}, prefix...), []byte("éz\n")...)
	out, errOut, code := runCutBytesEnv(
		t, utf8Locale, input, "-c", "4096-4097",
	)
	if code != 0 || errOut != "" || !bytes.Equal(out, []byte("éz\n")) {
		t.Fatalf("code=%d stdout=% x stderr=%q", code, out, errOut)
	}
}
