package iconvcmd

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/qiangli/coreutils/tool"
)

func TestIssue723MalformedStatusDoesNotDependOnDiscard(t *testing.T) {
	for _, tc := range []struct {
		name, from string
		input      []byte
	}{
		{"UTF-8", "UTF-8", []byte{'a', 0xff, 'b'}},
		{"UTF-16BE", "UTF-16BE", []byte{0, 'a', 0xd8, 0, 0, 'b'}},
		{"Shift_JIS", "Shift_JIS", []byte{'a', 0xff, 'b'}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, discard := range []bool{false, true} {
				args := []string{"-f", tc.from, "-t", "UTF-8"}
				if discard {
					args = append([]string{"-c"}, args...)
				}
				code, out, errout := invoke(t, string(tc.input), args...)
				if code != 1 || string(out) != "ab" {
					t.Fatalf("discard=%v code=%d out=%q err=%q", discard, code, out, errout)
				}
			}
		})
	}
}

func TestIssue723CharmapPathnamesUseSymbolicJoin(t *testing.T) {
	dir := t.TempDir()
	from := `<code_set_name> FROM
<mb_cur_max> 1
CHARMAP
<A> \x41
<letter002>...<letter004> \x42
<euro-sign> \x80
END CHARMAP
`
	to := `<code_set_name> TO
<mb_cur_max> 2
CHARMAP
<A> \x61
<letter002>...<letter004> \x62
<euro-sign> \xE2\x82\xAC
END CHARMAP
`
	if err := os.WriteFile(dir+"/from.map", []byte(from), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dir+"/to.map", []byte(to), 0o600); err != nil {
		t.Fatal(err)
	}
	code, out, errout := invokeDir(t, dir, string([]byte{0x41, 0x42, 0x43, 0x44, 0x80}),
		"-f", "./from.map", "-t", "./to.map")
	if code != 0 || string(out) != "abcd€" || errout != "" {
		t.Fatalf("code=%d out=%q err=%q", code, out, errout)
	}
}

func TestIssue723CharmapInvalidStatusAndSilentScope(t *testing.T) {
	dir := t.TempDir()
	from := "CHARMAP\n<A> \\x41\nEND CHARMAP\n"
	to := "CHARMAP\n<A> \\x61\nEND CHARMAP\n"
	if err := os.WriteFile(dir+"/f.map", []byte(from), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dir+"/t.map", []byte(to), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, discard := range []bool{false, true} {
		args := []string{"-s", "-f", "./f.map", "-t", "./t.map"}
		if discard {
			args = append([]string{"-c"}, args...)
		}
		code, out, errout := invokeDir(t, dir, "A!A", args...)
		if code != 1 || string(out) != "aa" || errout != "" {
			t.Fatalf("discard=%v code=%d out=%q err=%q", discard, code, out, errout)
		}
	}
}

func TestIssue723CharmapSyntaxAndAliasJoin(t *testing.T) {
	dir := t.TempDir()
	// Exercise the three normative byte notations, custom syntax characters,
	// and duplicate symbolic names for one source encoding.
	from := `<escape_char> /
<comment_char> %
% ignored
CHARMAP
<A> /d65 inline comment is ignored
<A-alias> /101 another inline comment
<cent-sign> /x80 hexadecimal comment
<gt/>> /x81
END CHARMAP
`
	to := `CHARMAP
<A-alias> \x61
<cent-sign> \xC2\xA2
<gt\>> \x21
END CHARMAP
`
	if err := os.WriteFile(dir+"/from.map", []byte(from), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dir+"/to.map", []byte(to), 0o600); err != nil {
		t.Fatal(err)
	}
	code, out, errout := invokeDir(t, dir, string([]byte{65, 0x80, 0x81}),
		"-f", "./from.map", "-t", "./to.map")
	if code != 0 || string(out) != "a¢!" || errout != "" {
		t.Fatalf("code=%d out=%q err=%q", code, out, errout)
	}
}

func TestIssue723CharmapOperandFailuresAreNotSilent(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(dir+"/bad.map", []byte("not a charmap\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{
		{"-s", "-f", "./missing.map", "-t", "./bad.map"},
		{"-s", "-f", "./bad.map", "-t", "./bad.map"},
		{"-s", "-f", "./bad.map", "-t", "UTF-8"},
	} {
		var errout bytes.Buffer
		rc := &tool.RunContext{Ctx: context.Background(), Dir: dir,
			Stdio: tool.Stdio{In: panicReader{}, Out: panicWriter{}, Err: &errout}}
		if code := run(rc, args); code != 1 || errout.Len() == 0 {
			t.Fatalf("args=%v code=%d err=%q", args, code, errout.String())
		}
	}
}

func TestIssue723CharmapParserRejectsMalformedDefinitions(t *testing.T) {
	for _, tc := range []struct {
		name, content, want string
	}{
		{"missing start", "<A> \\x41\nEND CHARMAP\n", "missing CHARMAP"},
		{"missing end", "CHARMAP\n<A> \\x41\n", "missing CHARMAP or END CHARMAP"},
		{"empty", "CHARMAP\nEND CHARMAP\n", "empty CHARMAP"},
		{"truncated hex", "CHARMAP\n<A> \\x4\nEND CHARMAP\n", "truncated encoding"},
		{"short decimal", "CHARMAP\n<A> \\d4\nEND CHARMAP\n", "invalid decimal"},
		{"short octal", "CHARMAP\n<A> \\4\nEND CHARMAP\n", "invalid encoding"},
		{"bad range", "CHARMAP\n<A1>...<A02> \\x41\nEND CHARMAP\n", "invalid symbolic range"},
		{"conflicting symbol", "CHARMAP\n<A> \\x41\n<A> \\x42\nEND CHARMAP\n", "conflicting definition"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := parseCharmap(strings.NewReader(tc.content))
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("err=%v, want substring %q", err, tc.want)
			}
		})
	}
}

func TestIssue723EllipsisInsideSingletonSymbolIsNotARange(t *testing.T) {
	table, err := parseCharmap(strings.NewReader("CHARMAP\n<dot...name> \\x41\nEND CHARMAP\n"))
	if err != nil {
		t.Fatal(err)
	}
	if got := table.bySymbol["<dot...name>"]; !bytes.Equal(got, []byte{'A'}) {
		t.Fatalf("mapping=%x", got)
	}
}

type terminalErrorReader struct{ done bool }

func (r *terminalErrorReader) Read(p []byte) (int, error) {
	if r.done {
		return 0, errors.New("injected read failure")
	}
	r.done = true
	copy(p, "ok")
	return 2, nil
}

func TestIssue723SilentDoesNotSuppressInputIOErrors(t *testing.T) {
	var out, errout bytes.Buffer
	rc := &tool.RunContext{Ctx: context.Background(), Dir: t.TempDir(),
		Stdio: tool.Stdio{In: &terminalErrorReader{}, Out: &out, Err: &errout}}
	if code := run(rc, []string{"-s", "-f", "UTF-8", "-t", "UTF-8"}); code != 1 ||
		!strings.Contains(errout.String(), "injected read failure") {
		t.Fatalf("code=%d out=%q err=%q", code, out.String(), errout.String())
	}
}

type terminalErrorWriter struct{}

func (terminalErrorWriter) Write([]byte) (int, error) { return 0, errors.New("injected write failure") }

func TestIssue723SilentDoesNotSuppressOutputIOErrors(t *testing.T) {
	var errout bytes.Buffer
	rc := &tool.RunContext{Ctx: context.Background(), Dir: t.TempDir(),
		Stdio: tool.Stdio{In: strings.NewReader("ok"), Out: terminalErrorWriter{}, Err: &errout}}
	if code := run(rc, []string{"-s", "-f", "UTF-8", "-t", "UTF-8"}); code != 1 ||
		!strings.Contains(errout.String(), "injected write failure") {
		t.Fatalf("code=%d err=%q", code, errout.String())
	}
}

type shortNilWriter struct{}

func (shortNilWriter) Write(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	return len(p) - 1, nil
}

func TestIssue723ShortWritesFailForCharmapAndList(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"f.map", "t.map"} {
		if err := os.WriteFile(dir+"/"+name, []byte("CHARMAP\n<A> \\x41\nEND CHARMAP\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	for _, args := range [][]string{
		{"-s", "-f", "./f.map", "-t", "./t.map"},
		{"-l"},
	} {
		var errout bytes.Buffer
		rc := &tool.RunContext{Ctx: context.Background(), Dir: dir,
			Stdio: tool.Stdio{In: strings.NewReader("A"), Out: shortNilWriter{}, Err: &errout}}
		if code := run(rc, args); code != 1 || !strings.Contains(errout.String(), io.ErrShortWrite.Error()) {
			t.Fatalf("args=%v code=%d err=%q", args, code, errout.String())
		}
	}
}

func TestIssue723ListIsStandaloneAndEveryEntryRoundTrips(t *testing.T) {
	for _, args := range [][]string{
		{"-l", "file"}, {"-l", "-c"}, {"-l", "-s"},
		{"-l", "-f", "UTF-8"}, {"-l", "-t", "UTF-8"},
	} {
		code, out, errout := invoke(t, "", args...)
		if code != 2 || len(out) != 0 || !strings.Contains(errout, "standalone") {
			t.Fatalf("args=%v code=%d out=%q err=%q", args, code, out, errout)
		}
	}

	code, listed, errout := invoke(t, "", "-l")
	if code != 0 || errout != "" {
		t.Fatalf("list code=%d err=%q", code, errout)
	}
	for _, name := range strings.Fields(string(listed)) {
		code, encoded, encodeErr := invoke(t, "A", "-f", "UTF-8", "-t", name)
		if code != 0 || encodeErr != "" {
			t.Fatalf("to role %q: code=%d err=%q", name, code, encodeErr)
		}
		code, decoded, decodeErr := invoke(t, string(encoded), "-f", name, "-t", "UTF-8")
		if code != 0 || string(decoded) != "A" || decodeErr != "" {
			t.Fatalf("from role %q: code=%d out=%q err=%q encoded=%x", name, code, decoded, decodeErr, encoded)
		}
	}
}

func TestIssue723CP858AliasAndListing(t *testing.T) {
	code, out, errout := invoke(t, string([]byte{0xd5}), "-f", "CP858", "-t", "UTF-8")
	if code != 0 || string(out) != "€" || errout != "" {
		t.Fatalf("convert code=%d out=%q err=%q", code, out, errout)
	}
	code, out, errout = invoke(t, "", "-l")
	if code != 0 || !bytes.Contains(out, []byte("CP858\n")) || errout != "" {
		t.Fatalf("list code=%d out=%q err=%q", code, out, errout)
	}
}
