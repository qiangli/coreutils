package tailcmd

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/qiangli/coreutils/tool"
)

func TestPOSIXDoubleDashProtectsOptionLikeOperand(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "-n"), []byte("one\ntwo\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	var out, errOut bytes.Buffer
	rc := &tool.RunContext{Ctx: context.Background(), Dir: dir, Env: []string{"POSIXLY_CORRECT=1"}, Stdio: tool.Stdio{In: strings.NewReader(""), Out: &out, Err: &errOut}}
	if code := cmd.Run(rc, []string{"-n", "1", "--", "-n"}); code != 0 || out.String() != "two\n" || errOut.Len() != 0 {
		t.Fatalf("tail -- -n = (%q, %q, %d)", out.String(), errOut.String(), code)
	}
}

// TestPOSIXDoubleDashKeepsFollowingFileOperand pins Utility Syntax Guideline
// 10 at the applet boundary: -- is consumed as an option delimiter and the
// following pathname is the FILE operand.  This deliberately uses an ordinary
// pathname as well as the option-looking pathname above; neither the delimiter
// nor the operand may be lost while argv is normalized.
func TestPOSIXDoubleDashKeepsFollowingFileOperand(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "input.txt", twelveLines())

	var out, errOut bytes.Buffer
	rc := &tool.RunContext{
		Ctx: context.Background(),
		Dir: dir,
		Env: []string{"POSIXLY_CORRECT=1"},
		Stdio: tool.Stdio{
			In:  strings.NewReader(""),
			Out: &out,
			Err: &errOut,
		},
	}
	if code := cmd.Run(rc, []string{"--", "input.txt"}); code != 0 {
		t.Fatalf("tail -- input.txt: code=%d stderr=%q", code, errOut.String())
	}
	want := "line3\nline4\nline5\nline6\nline7\nline8\nline9\nline10\nline11\nline12\n"
	if out.String() != want || errOut.Len() != 0 {
		t.Fatalf("tail -- input.txt = (%q, %q), want (%q, empty stderr)", out.String(), errOut.String(), want)
	}
}

// TestPOSIXDoubleDashAfterHistoricalByteCount pins the complete argv shape
// used by the certification boundary: the retained historical first-argument
// byte-count spelling is normalized before -- is consumed, and the pathname
// following the delimiter remains the FILE operand.
func TestPOSIXDoubleDashAfterHistoricalByteCount(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "input.txt", "prefix\nlast-7\n")

	var out, errOut bytes.Buffer
	rc := &tool.RunContext{
		Ctx: context.Background(),
		Dir: dir,
		Env: []string{"POSIXLY_CORRECT=1"},
		Stdio: tool.Stdio{
			In:  strings.NewReader(""),
			Out: &out,
			Err: &errOut,
		},
	}
	if code := cmd.Run(rc, []string{"-7c", "--", "input.txt"}); code != 0 {
		t.Fatalf("tail historical-byte-count -- input.txt: code=%d stderr=%q", code, errOut.String())
	}
	if out.String() != "last-7\n" || errOut.Len() != 0 {
		t.Fatalf("tail historical-byte-count -- input.txt = (%q, %q), want (%q, empty stderr)", out.String(), errOut.String(), "last-7\n")
	}
}

// TestPOSIXParentDirectoryOperand pins ordinary pathname resolution when the
// applet's logical working directory differs from the process working
// directory, as it does after an embedded shell performs cd.  The operand must
// be resolved against RunContext.Dir without altering the visible pathname or
// requiring the host process itself to chdir.
func TestPOSIXParentDirectoryOperand(t *testing.T) {
	root := t.TempDir()
	child := filepath.Join(root, "child")
	if err := os.Mkdir(child, 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, root, "parent-input.txt", twelveLines())

	var out, errOut bytes.Buffer
	rc := &tool.RunContext{
		Ctx: context.Background(),
		Dir: child,
		Env: []string{"POSIXLY_CORRECT=1"},
		Stdio: tool.Stdio{
			In:  strings.NewReader(""),
			Out: &out,
			Err: &errOut,
		},
	}
	if code := cmd.Run(rc, []string{"../parent-input.txt"}); code != 0 {
		t.Fatalf("tail ../parent-input.txt: code=%d stderr=%q", code, errOut.String())
	}
	want := "line3\nline4\nline5\nline6\nline7\nline8\nline9\nline10\nline11\nline12\n"
	if out.String() != want || errOut.Len() != 0 {
		t.Fatalf("tail ../parent-input.txt = (%q, %q), want (%q, empty stderr)", out.String(), errOut.String(), want)
	}
}

func TestPOSIXStopsOptionParsingAtFirstOperand(t *testing.T) {
	dir := t.TempDir()
	for name, data := range map[string]string{"first": "one\ntwo\n", "--lin": "three\nfour\n"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(data), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	var out, errOut bytes.Buffer
	rc := &tool.RunContext{Ctx: context.Background(), Dir: dir, Env: []string{"POSIXLY_CORRECT=1"}, Stdio: tool.Stdio{In: strings.NewReader(""), Out: &out, Err: &errOut}}
	if code := cmd.Run(rc, []string{"-n", "1", "first", "--lin"}); code != 0 || !strings.Contains(out.String(), "two\n") || !strings.Contains(out.String(), "four\n") || errOut.Len() != 0 {
		t.Fatalf("tail post-operand --lin = (%q, %q, %d)", out.String(), errOut.String(), code)
	}
}
