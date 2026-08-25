package iconvcmd

import (
	"bytes"
	"context"
	"os"
	"strings"
	"testing"

	"github.com/qiangli/coreutils/tool"
)

func TestIssue732DiscardMalformedAndTruncatedSequences(t *testing.T) {
	for _, tc := range []struct {
		name, from string
		input      []byte
		want       string
	}{
		{"UTF-8 malformed", "UTF-8", []byte{'a', 0xff, 'b'}, "ab"},
		{"UTF-16BE unpaired surrogate", "UTF-16BE", []byte{0, 'a', 0xd8, 0, 0, 'b'}, "ab"},
		{"Shift_JIS malformed", "Shift_JIS", []byte{'a', 0xff, 'b'}, "ab"},
		{"EUC-JP truncated", "EUC-JP", []byte{'a', 0x8f, 0xa1}, "a"},
		{"ISO-2022-JP truncated", "ISO-2022-JP", []byte{'a', 0x1b, '$', 'B', 0x24}, "a"},
		{"GBK truncated", "GBK", []byte{'a', 0x81}, "a"},
		{"Big5 truncated", "Big5", []byte{'a', 0x81}, "a"},
		{"EUC-KR truncated", "EUC-KR", []byte{'a', 0xa1}, "a"},
		{"HZ-GB-2312 truncated", "HZ-GB-2312", []byte{'a', '~', '{', '!'}, "a"},
		{"GB18030 truncated", "GB18030", []byte{'a', 0x81, 0x30}, "a0"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// POSIX explicitly makes status independent of -c. Without -c the
			// resulting output is unspecified, but failure and the diagnostic are
			// not: -s suppresses only the invalid-character diagnostic.
			code, _, errout := invoke(t, string(tc.input), "-f", tc.from, "-t", "UTF-8")
			if code != 1 || errout == "" {
				t.Fatalf("without -c: code=%d err=%q", code, errout)
			}
			code, _, errout = invoke(t, string(tc.input), "-s", "-f", tc.from, "-t", "UTF-8")
			if code != 1 || errout != "" {
				t.Fatalf("with -s: code=%d err=%q", code, errout)
			}
			code, out, errout := invoke(t, string(tc.input), "-c", "-f", tc.from, "-t", "UTF-8")
			if code != 1 || string(out) != tc.want || errout != "" {
				t.Fatalf("code=%d out=%q err=%q", code, out, errout)
			}
		})
	}
}

func TestIssue732SilentDoesNotSuppressShortOutputWrite(t *testing.T) {
	var errout bytes.Buffer
	rc := &tool.RunContext{Ctx: context.Background(), Dir: t.TempDir(),
		Stdio: tool.Stdio{In: strings.NewReader("content"), Out: shortNilWriter{}, Err: &errout}}
	if code := run(rc, []string{"-s", "-f", "UTF-8", "-t", "UTF-8"}); code != 1 ||
		!strings.Contains(errout.String(), "short write") {
		t.Fatalf("code=%d err=%q", code, errout.String())
	}
}

func TestIssue732LocalePrecedenceForOmittedEncoding(t *testing.T) {
	for _, tc := range []struct {
		name string
		env  []string
		want string
	}{
		{"LC_ALL overrides categories", []string{"LANG=C", "LC_CTYPE=de_DE", "LC_ALL=ja_JP.UTF-8"}, "UTF-8"},
		{"LC_CTYPE overrides LANG", []string{"LANG=C", "LC_CTYPE=de_DE"}, "ISO-8859-1"},
		{"empty LC_ALL falls through", []string{"LANG=C", "LC_CTYPE=de_DE", "LC_ALL="}, "ISO-8859-1"},
		{"LANG fallback", []string{"LANG=fr_FR.UTF-8"}, "UTF-8"},
		{"modifier ignored", []string{"LC_CTYPE=de_DE.ISO-8859-1@euro"}, "ISO-8859-1"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := localeCodeset(tc.env)
			if err != nil || got != tc.want {
				t.Fatalf("codeset=%q err=%v, want %q", got, err, tc.want)
			}
		})
	}
}

func TestIssue732FileAndStandardInputOperandsAreOrderedStreams(t *testing.T) {
	dir := t.TempDir()
	// A decoder is fresh for every file, so each independent UTF-16 stream can
	// carry its own BOM. The encoder remains shared across operands.
	for name, data := range map[string][]byte{
		"one": {0xfe, 0xff, 0x00, 'A'},
		"two": {0xff, 0xfe, 'B', 0x00},
	} {
		if err := os.WriteFile(dir+"/"+name, data, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	var out, errout bytes.Buffer
	rc := &tool.RunContext{Ctx: context.Background(), Dir: dir,
		Stdio: tool.Stdio{In: bytes.NewReader([]byte{0xfe, 0xff, 0x00, 'C'}), Out: &out, Err: &errout}}
	if code := run(rc, []string{"-f", "UTF-16", "-t", "UTF-8", "one", "-", "-", "two"}); code != 0 || out.String() != "ACB" || errout.Len() != 0 {
		t.Fatalf("code=%d out=%q err=%q", code, out.String(), errout.String())
	}

	if err := os.WriteFile(dir+"/utf8-one", []byte("A"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dir+"/utf8-two", []byte("B"), 0o600); err != nil {
		t.Fatal(err)
	}
	out.Reset()
	errout.Reset()
	rc.Stdio.In = strings.NewReader("")
	if code := run(rc, []string{"-f", "UTF-8", "-t", "UTF-16", "utf8-one", "utf8-two"}); code != 0 || !bytes.Equal(out.Bytes(), []byte{0xfe, 0xff, 0x00, 'A', 0x00, 'B'}) || errout.Len() != 0 {
		t.Fatalf("shared encoder: code=%d out=%x err=%q", code, out.Bytes(), errout.String())
	}
}

func TestIssue732ValidMultibyteCharactersCrossEveryReadBoundary(t *testing.T) {
	for _, tc := range []struct {
		encoding, text string
	}{
		{"UTF-8", "é日本"},
		{"UTF-16", "é日本"},
		{"UTF-16BE", "é日本"},
		{"UTF-16LE", "é日本"},
		{"Shift_JIS", "日本"},
		{"EUC-JP", "日本"},
		{"ISO-2022-JP", "日本"},
		{"GBK", "中文"},
		{"GB18030", "中文😀"},
		{"HZ-GB-2312", "中文"},
		{"Big5", "中文"},
		{"EUC-KR", "한국"},
	} {
		t.Run(tc.encoding, func(t *testing.T) {
			code, encoded, errout := invoke(t, tc.text, "-f", "UTF-8", "-t", tc.encoding)
			if code != 0 || errout != "" {
				t.Fatalf("encode: code=%d err=%q", code, errout)
			}
			var out, decodeErr bytes.Buffer
			rc := &tool.RunContext{Ctx: context.Background(), Dir: t.TempDir(),
				Stdio: tool.Stdio{In: oneByteReader{bytes.NewReader(encoded)}, Out: &out, Err: &decodeErr}}
			if code := run(rc, []string{"-f", tc.encoding, "-t", "UTF-8"}); code != 0 || out.String() != tc.text || decodeErr.Len() != 0 {
				t.Fatalf("decode: code=%d out=%q err=%q encoded=%x", code, out.String(), decodeErr.String(), encoded)
			}
		})
	}
}

func TestIssue732OpenFailureContinuesAndSilentDoesNotHideIt(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(dir+"/good", []byte("converted"), 0o600); err != nil {
		t.Fatal(err)
	}
	var out, errout bytes.Buffer
	rc := &tool.RunContext{Ctx: context.Background(), Dir: dir,
		Stdio: tool.Stdio{Out: &out, Err: &errout}}
	if code := run(rc, []string{"-s", "-f", "UTF-8", "-t", "UTF-8", "missing", "good"}); code != 1 || out.String() != "converted" || !strings.Contains(errout.String(), "missing") {
		t.Fatalf("code=%d out=%q err=%q", code, out.String(), errout.String())
	}
}

func TestIssue732HelpShowsAllPOSIXSynopses(t *testing.T) {
	code, out, errout := invoke(t, "", "--help")
	if code != 0 || errout != "" {
		t.Fatalf("code=%d err=%q", code, errout)
	}
	for _, synopsis := range []string{
		"iconv [-cs] -f frommap -t tomap [file...]",
		"iconv -f fromcode [-cs] [-t tocode] [file...]",
		"iconv -t tocode [-cs] [-f fromcode] [file...]",
		"iconv -l",
	} {
		if !bytes.Contains(out, []byte(synopsis)) {
			t.Errorf("help missing %q:\n%s", synopsis, out)
		}
	}
}

func TestIssue732CharmapDropsWholeUnrepresentableMultibyteCharacter(t *testing.T) {
	from := &charmapTable{
		bySymbol: map[string][]byte{"<wide>": {0x81, 0x41}, "<A>": {'A'}},
		byBytes:  map[string][]string{string([]byte{0x81, 0x41}): {"<wide>"}, "A": {"<A>"}},
		maxBytes: 2,
	}
	to := &charmapTable{bySymbol: map[string][]byte{"<A>": {'a'}}}
	consumed, replacement := charmapMatch(from, to, []byte{0x81, 0x41})
	if consumed != 2 || replacement != nil {
		t.Fatalf("match consumed=%d replacement=%x", consumed, replacement)
	}

	// Pin the conversion loop's required advance: an unrepresentable two-byte
	// source character must not be reinterpreted from its second byte.
	dir := t.TempDir()
	writeCharmapFixture(t, dir+"/from.map", "CHARMAP\n<wide> \\x81\\x41\n<A> \\x41\nEND CHARMAP\n")
	writeCharmapFixture(t, dir+"/to.map", "CHARMAP\n<A> \\x61\nEND CHARMAP\n")
	code, out, errout := invokeDir(t, dir, string([]byte{0x81, 0x41}), "-c", "-f", "./from.map", "-t", "./to.map")
	if code != 1 || len(out) != 0 || errout != "" {
		t.Fatalf("code=%d out=%x err=%q", code, out, errout)
	}
}

func writeCharmapFixture(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}
