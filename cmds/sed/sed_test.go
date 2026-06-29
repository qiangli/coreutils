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
	var o, e bytes.Buffer
	rc := &tool.RunContext{
		Ctx:   context.Background(),
		Dir:   t.TempDir(),
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

func TestSedEREMode(t *testing.T) {
	if out, _, _ := runSed(t, "aaa\n", "-E", "s/a+/X/"); out != "X\n" {
		t.Errorf("-E s/a+/X/ = %q, want X", out)
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

// A back-reference in a PATTERN can't be expressed by RE2 → must fail loudly,
// not silently mis-match (the coreutils rule).
func TestSedPatternBackrefFailsLoudly(t *testing.T) {
	_, errOut, code := runSed(t, "aa\n", `s/\(a\)\1/X/`)
	if code == 0 {
		t.Error("a pattern back-reference should be a hard error")
	}
	if !strings.Contains(strings.ToLower(errOut), "back-reference") {
		t.Errorf("error should name the cause: %q", errOut)
	}
}
