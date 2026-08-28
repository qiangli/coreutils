package edcmd

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/qiangli/coreutils/tool"
)

func runEd(t *testing.T, input string, args ...string) (int, string, string, string) {
	t.Helper()
	dir := t.TempDir()
	var out, errb bytes.Buffer
	rc := &tool.RunContext{
		Ctx: context.Background(), Dir: dir, FS: tool.NewLocalFS(),
		Stdio: tool.Stdio{In: strings.NewReader(input), Out: &out, Err: &errb},
	}
	edTool := tool.Lookup("ed")
	if edTool == nil {
		t.Fatal("registered ed tool is missing")
	}
	code := edTool.Run(rc, args)
	return code, out.String(), errb.String(), dir
}

func TestRegisteredToolScriptEditingAndWrite(t *testing.T) {
	code, out, stderr, dir := runEd(t, "a\nalpha\nbeta\ngamma\n.\n2i\nbefore beta\n.\n3c\nBETA\n.\n1d\n1,$n\nw result.txt\nq\n", "-s")
	if code != 0 || stderr != "" {
		t.Fatalf("exit=%d stderr=%q", code, stderr)
	}
	if out != "1\tbefore beta\n2\tBETA\n3\tgamma\n" {
		t.Fatalf("stdout=%q", out)
	}
	data, err := os.ReadFile(filepath.Join(dir, "result.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if got := string(data); got != "before beta\nBETA\ngamma\n" {
		t.Fatalf("file=%q", got)
	}
}

func TestRegisteredToolAddressesSearchAndList(t *testing.T) {
	code, out, stderr, _ := runEd(t, "0a\none\ntwo\t2\nthree\n.\n/two/p\n-1,+1n\n1l\nQ\n", "-s")
	if code != 0 || stderr != "" {
		t.Fatalf("exit=%d stderr=%q stdout=%q", code, stderr, out)
	}
	want := "two\t2\n1\tone\n2\ttwo\t2\n3\tthree\none$\n"
	if out != want {
		t.Fatalf("stdout=%q want=%q", out, want)
	}
}

func TestRegisteredToolBRESubstitution(t *testing.T) {
	input := "a\nbook 12\nboot 34\n.\n1,$s/^b\\(oo\\)/B\\1/p\n1,$s/[[:digit:]][[:digit:]]/(&)/g\n1,$p\nQ\n"
	code, out, stderr, _ := runEd(t, input, "-s")
	if code != 0 || stderr != "" {
		t.Fatalf("exit=%d stderr=%q stdout=%q", code, stderr, out)
	}
	want := "Boot 34\nBook (12)\nBoot (34)\n"
	if out != want {
		t.Fatalf("stdout=%q want=%q", out, want)
	}
}

func TestRegisteredToolChangeEmptyAndOmittedSubstituteDelimiter(t *testing.T) {
	code, out, stderr, _ := runEd(t, "0c\nabc\n.\ns/b/B\nQ\n", "-s")
	if code != 0 || stderr != "" {
		t.Fatalf("exit=%d stderr=%q stdout=%q", code, stderr, out)
	}
	if out != "aBc\n" {
		t.Fatalf("stdout=%q", out)
	}
}

func TestRegisteredToolEditWriteCountsAndRememberedFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "in.txt")
	if err := os.WriteFile(path, []byte("old\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	var out, errb bytes.Buffer
	rc := &tool.RunContext{Ctx: context.Background(), Dir: dir, FS: tool.NewLocalFS(), Stdio: tool.Stdio{In: strings.NewReader("1c\nnew\n.\nw\ne\n1p\nq\n"), Out: &out, Err: &errb}}
	code := tool.Lookup("ed").Run(rc, []string{"in.txt"})
	if code != 0 || errb.String() != "" {
		t.Fatalf("exit=%d stderr=%q", code, errb.String())
	}
	if out.String() != "4\n4\n4\nnew\n" {
		t.Fatalf("stdout=%q", out.String())
	}
	data, _ := os.ReadFile(path)
	if string(data) != "new\n" {
		t.Fatalf("file=%q", data)
	}
}

func TestRegisteredToolDirtyQuitAndDiagnostics(t *testing.T) {
	code, out, stderr, _ := runEd(t, "a\nx\n.\nq\nq\n", "-s")
	if code != 1 || stderr != "" {
		t.Fatalf("exit=%d stderr=%q", code, stderr)
	}
	if out != "?\n" {
		t.Fatalf("stdout=%q", out)
	}

	code, out, stderr, _ = runEd(t, "H\n9p\nQ\n", "-s")
	if code != 1 || stderr != "" {
		t.Fatalf("exit=%d stderr=%q", code, stderr)
	}
	if out != "?\ninvalid address\n" {
		t.Fatalf("stdout=%q", out)
	}

	code, out, _, _ = runEd(t, "9p\nh\nQ\n", "-s")
	if code != 1 || out != "?\ninvalid address\n" {
		t.Fatalf("help diagnostic: exit=%d stdout=%q", code, out)
	}
}

func TestRegisteredToolPromptSilentAndUsage(t *testing.T) {
	code, out, stderr, _ := runEd(t, "Q\n", "-p", "> ", "-s")
	if code != 0 || stderr != "" || out != "> " {
		t.Fatalf("exit=%d stdout=%q stderr=%q", code, out, stderr)
	}
	code, _, stderr, _ = runEd(t, "", "a", "b")
	if code != 2 || !strings.Contains(stderr, "extra operand") {
		t.Fatalf("exit=%d stderr=%q", code, stderr)
	}
}

type edFailWriter struct {
	err   error
	short bool
}

func (w edFailWriter) Write(p []byte) (int, error) {
	if w.short && len(p) > 0 {
		return len(p) - 1, nil
	}
	return 0, w.err
}

func TestRegisteredToolRejectsOutputAndFileShortWrites(t *testing.T) {
	for _, w := range []io.Writer{
		edFailWriter{err: errors.New("unavailable")},
		edFailWriter{short: true},
	} {
		if err := writeBytes(w, []byte("data")); err == nil {
			t.Fatalf("writeBytes(%T) succeeded", w)
		}
	}

	var errOut bytes.Buffer
	rc := &tool.RunContext{Ctx: context.Background(), Dir: t.TempDir(), FS: tool.NewLocalFS(),
		Stdio: tool.Stdio{In: strings.NewReader("Q\n"), Out: edFailWriter{short: true}, Err: &errOut}}
	if code := tool.Lookup("ed").Run(rc, []string{"-p", "> "}); code != 1 {
		t.Fatalf("short prompt write exit=%d, want 1", code)
	}
	if !strings.Contains(errOut.String(), io.ErrShortWrite.Error()) {
		t.Fatalf("diagnostic=%q", errOut.String())
	}
}

func TestRegisteredToolFileCommandDiagnosticsUseStderr(t *testing.T) {
	code, out, stderr, _ := runEd(t, "r missing\nQ\n", "-s")
	if code != 1 {
		t.Fatalf("exit=%d stdout=%q stderr=%q", code, out, stderr)
	}
	if out != "?\n" {
		t.Fatalf("stdout=%q", out)
	}
	if !strings.Contains(stderr, "missing") {
		t.Fatalf("stderr=%q", stderr)
	}
}

func TestRegisteredToolCTypeClassesUseByteLocale(t *testing.T) {
	var out, errb bytes.Buffer
	rc := &tool.RunContext{
		Ctx: context.Background(), Dir: t.TempDir(), FS: tool.NewLocalFS(),
		Env:   []string{"LC_ALL=C"},
		Stdio: tool.Stdio{In: strings.NewReader("a\nA\né\n.\n1,$g/[[:alpha:]]/p\nQ\n"), Out: &out, Err: &errb},
	}
	code := tool.Lookup("ed").Run(rc, []string{"-s"})
	if code != 0 || errb.String() != "" {
		t.Fatalf("exit=%d stdout=%q stderr=%q", code, out.String(), errb.String())
	}
	if out.String() != "A\n" {
		t.Fatalf("stdout=%q", out.String())
	}
}
