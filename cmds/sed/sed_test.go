package sedcmd

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/qiangli/coreutils/tool"
)

func runSed(t *testing.T, in string, args ...string) (out, errOut string, code int) {
	t.Helper()
	return runSedInDir(t, t.TempDir(), in, args...)
}

func runSedInDir(t *testing.T, dir, in string, args ...string) (out, errOut string, code int) {
	t.Helper()
	var o, e bytes.Buffer
	rc := &tool.RunContext{
		Ctx:   context.Background(),
		Dir:   dir,
		Stdio: tool.Stdio{In: strings.NewReader(in), Out: &o, Err: &e},
	}
	code = cmd.Run(rc, args)
	return o.String(), e.String(), code
}

func TestSedBasicSubstitution(t *testing.T) {
	if out, _, _ := runSed(t, "hello\n", "s/l/L/g"); out != "heLLo\n" {
		t.Errorf("s/l/L/g = %q, want heLLo", out)
	}
}

// The headline GNU-compat case: BRE \(...\) groups + \1/\2 backrefs, which the
// upstream (Go/ERE) engine could not do — proves the translation layer.
func TestSedBREGroupsAndBackrefs(t *testing.T) {
	if out, _, _ := runSed(t, "ab\n", `s/\(a\)\(b\)/\2\1/`); out != "ba\n" {
		t.Errorf(`s/\(a\)\(b\)/\2\1/ = %q, want ba`, out)
	}
}

func TestSedBREBackrefConformance(t *testing.T) {
	if out, _, _ := runSed(t, "a^bb\naXbb\n", `s/a^\(b\)\1/Z/`); out != "Z\naXbb\n" {
		t.Errorf(`s/a^\(b\)\1/Z/ = %q, want literal-caret replacement`, out)
	}
	if out, _, _ := runSed(t, "__\n!!\n", `s/\(\w\)\1/Z/`); out != "Z\n!!\n" {
		t.Errorf(`s/\(\w\)\1/Z/ = %q, want GNU word-class escape through backref matcher`, out)
	}
}

func TestSedAmpersandWholeMatch(t *testing.T) {
	if out, _, _ := runSed(t, "abc\n", `s/b/[&]/`); out != "a[b]c\n" {
		t.Errorf("s/b/[&]/ = %q, want a[b]c", out)
	}
	// Escaped & is a literal ampersand.
	if out, _, _ := runSed(t, "abc\n", `s/b/\&/`); out != "a&c\n" {
		t.Errorf(`s/b/\&/ = %q, want a&c`, out)
	}
}

func TestSedBREInterval(t *testing.T) {
	if out, _, _ := runSed(t, "aaaa\n", `s/a\{2\}/X/`); out != "Xaa\n" {
		t.Errorf(`s/a\{2\}/X/ = %q, want Xaa`, out)
	}
}

func TestSedBREWordEdgeAnchors(t *testing.T) {
	if out, _, _ := runSed(t, "sword word words\n", `s/\<word\>/X/g`); out != "sword X words\n" {
		t.Errorf(`s/\<word\>/X/g = %q, want sword X words`, out)
	}
	if out, _, _ := runSed(t, "sword\nword\nwords\n", "-n", `/\<word\>/p`); out != "word\n" {
		t.Errorf(`-n /\<word\>/p = %q, want word`, out)
	}
}

func TestSedEREMode(t *testing.T) {
	if out, _, _ := runSed(t, "aaa\n", "-E", "s/a+/X/"); out != "X\n" {
		t.Errorf("-E s/a+/X/ = %q, want X", out)
	}
	if out, _, _ := runSed(t, "sword word words\n", "-E", `s/\<word\>/X/g`); out != "sword X words\n" {
		t.Errorf(`-E s/\<word\>/X/g = %q, want sword X words`, out)
	}
}

func TestSedCaseInsensitiveFlag(t *testing.T) {
	if out, _, _ := runSed(t, "Hello\n", "s/hello/hi/I"); out != "hi\n" {
		t.Errorf("s/hello/hi/I = %q, want hi", out)
	}
}

func TestSedDeleteAndRange(t *testing.T) {
	if out, _, _ := runSed(t, "a\nb\nc\n", "2d"); out != "a\nc\n" {
		t.Errorf("2d = %q, want a\\nc", out)
	}
	if out, _, _ := runSed(t, "1\n2\n3\n4\n", "2,3d"); out != "1\n4\n" {
		t.Errorf("2,3d = %q, want 1\\n4", out)
	}
}

func TestSedReadCommandUsesRunDirectory(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "include.txt"), []byte("included\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	out, errOut, code := runSedInDir(t, dir, "1\n2\n", "1r include.txt")
	if code != 0 || errOut != "" || out != "1\nincluded\n2\n" {
		t.Errorf("1r include.txt: out=%q err=%q code=%d", out, errOut, code)
	}
}

func TestSedReadCommandMissingFileIsEmpty(t *testing.T) {
	// GNU sed documents r filename as treating unreadable files as empty,
	// without an error indication.
	out, errOut, code := runSed(t, "1\n2\n", "1r missing.txt")
	if code != 0 || errOut != "" || out != "1\n2\n" {
		t.Errorf("1r missing.txt: out=%q err=%q code=%d", out, errOut, code)
	}
}

func TestSedQuietRegexAddressPrint(t *testing.T) {
	if out, _, _ := runSed(t, "a\nbb\nc\n", "-n", "/b/p"); out != "bb\n" {
		t.Errorf("-n /b/p = %q, want bb", out)
	}
}

func TestSedTransliterate(t *testing.T) {
	if out, _, _ := runSed(t, "abc\n", "y/abc/xyz/"); out != "xyz\n" {
		t.Errorf("y/abc/xyz/ = %q, want xyz", out)
	}
}

func TestSedInPlace(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "f.txt")
	os.WriteFile(f, []byte("foo\nbar\n"), 0o644)
	var o, e bytes.Buffer
	rc := &tool.RunContext{Ctx: context.Background(), Dir: dir, Stdio: tool.Stdio{In: strings.NewReader(""), Out: &o, Err: &e}}
	if code := cmd.Run(rc, []string{"-i", "s/o/0/g", f}); code != 0 {
		t.Fatalf("-i exit %d: %s", code, e.String())
	}
	b, _ := os.ReadFile(f)
	if string(b) != "f00\nbar\n" {
		t.Errorf("in-place result = %q, want f00\\nbar", b)
	}
}

func TestSedInPlaceBackup(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "f.txt")
	os.WriteFile(f, []byte("x\n"), 0o644)
	rc := &tool.RunContext{Ctx: context.Background(), Dir: dir, Stdio: tool.Stdio{In: strings.NewReader(""), Out: &bytes.Buffer{}, Err: &bytes.Buffer{}}}
	cmd.Run(rc, []string{"-i.bak", "s/x/y/", f})
	if b, _ := os.ReadFile(f); string(b) != "y\n" {
		t.Errorf("edited = %q, want y", b)
	}
	if b, _ := os.ReadFile(f + ".bak"); string(b) != "x\n" {
		t.Errorf("backup = %q, want x (original)", b)
	}
}

func TestSedPatternBackref(t *testing.T) {
	out, errOut, code := runSed(t, "aa\n", `s/\(a\)\1/X/`)
	if code != 0 || errOut != "" || out != "X\n" {
		t.Errorf("sed pattern backref: out=%q err=%q code=%d", out, errOut, code)
	}
}

// POSIX behaviors that sed's regex layer previously got wrong. Each expectation
// is the byte-for-byte output GNU sed produces.
func TestSedPOSIXRegexConformance(t *testing.T) {
	cases := []struct {
		name   string
		in     string
		script string
		want   string
	}{
		// POSIX XCU sed: "the escape sequence '\n' shall match a <newline>
		// embedded in the pattern space". This used to be a hard parse error
		// ("unsupported escape \n"), so no N;s/…\n…/ script could run at all.
		{"backslash-n matches embedded newline", "a\nb\n", `N;s/a\nb/X/`, "X\n"},
		{"backslash-n in a bracket expression", "a\nb\n", `N;s/a[\n]b/X/`, "X\n"},
		{"backslash-n in the replacement is unchanged", "ab\n", `s/ab/a\nb/`, "a\nb\n"},
		{"backslash-t matches a tab", "a\tb\n", `s/a\tb/X/`, "X\n"},
		// An escaped backslash must not turn the next byte into an escape:
		// \\n is a literal backslash followed by 'n'.
		{"escaped backslash then n is literal", "a\\nb\n", `s/a\\nb/X/`, "X\n"},

		// POSIX: a period matches any character — including the newlines sed's
		// pattern space holds after N.
		{"dot matches embedded newline", "a\nb\n", `N;s/a.b/X/`, "X\n"},
		{"dot-star spans embedded newline", "a\nb\n", `N;s/a.*b/X/`, "X\n"},
		// ...except under the M modifier, where GNU documents the opposite:
		// "the dot character does not match a new-line character in multi-line
		// mode".
		{"M modifier turns dot-all back off", "a\nb\n", `N;s/a.b/X/M`, "a\nb\n"},
		// M also makes ^/$ match at the embedded newlines, which still works.
		{"M anchors match at embedded newlines", "a\nb\n", `N;s/^b$/X/M`, "a\nX\n"},

		// POSIX XBD 9.1: leftmost-longest. The shorter alternative is written
		// first, but the longest match at the leftmost offset is the one
		// substituted.
		{"leftmost-longest alternation", "ab\n", `s/a\|ab/X/`, "X\n"},
		{"leftmost-longest, longer alternative later", "foobar\n", `s/foo\|foobar/X/`, "X\n"},

		// POSIX XBD 9.3.6: a back-reference matches the same string the group
		// matched — the empty string, when the group matched empty. \(a*\)
		// matches empty before "b", so \1 matches empty and the whole pattern
		// matches "b".
		{"backref to a group that matched empty", "b\n", `s/\(a*\)b\1/X/`, "X\n"},
		// And the match reported is the leftmost one: "aba" at offset 1
		// (group 1 = "a"), not the empty-capture match "b" at offset 2.
		{"leftmost match beats a later empty-capture match", "aaba\n", `s/\(a*\)b\1/X/`, "aX\n"},
	}
	for _, c := range cases {
		if out, errOut, code := runSed(t, c.in, c.script); out != c.want || code != 0 {
			t.Errorf("%s: sed %q on %q = (%q, code %d, err %q), want %q",
				c.name, c.script, c.in, out, code, errOut, c.want)
		}
	}
}
