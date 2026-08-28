package diffcmd

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

// POSIX.1-2016 diff evidence (Issue 7, XCU:diff). This file pins mandatory
// operand and error-status clauses that the broader format tests do not isolate.

func TestIssue7DirectoryFileOperandUsesBasename(t *testing.T) {
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "left"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, dir, "left/name", "old\n")
	writeFile(t, dir, "name", "new\n")

	out, errb, code := runIn(t, dir, "", "left", "name")
	want := "1c1\n< old\n---\n> new\n"
	if code != 1 || errb != "" || out != want {
		t.Fatalf("diff dir file = (%q, %q, %d), want basename comparison %q", out, errb, code, want)
	}
}

func TestIssue7OperandArityAndTroubleStatuses(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "same", "x\n")
	for _, tc := range []struct {
		name    string
		args    []string
		code    int
		errPart string
	}{
		{"missing both", nil, 2, "missing operand"},
		{"missing second", []string{"same"}, 2, "missing operand after 'same'"},
		{"extra", []string{"same", "same", "extra"}, 2, "extra operand 'extra'"},
		{"missing file", []string{"same", "missing"}, 2, "No such file or directory"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, errb, code := runIn(t, dir, "", tc.args...)
			if code != tc.code || out != "" || !strings.Contains(errb, tc.errPart) {
				t.Fatalf("diff %v = (%q, %q, %d), want empty stdout, stderr containing %q, code %d", tc.args, out, errb, code, tc.errPart, tc.code)
			}
		})
	}
}

func TestIssue7StopsOptionParsingAtFirstOperand(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "first", "same\n")
	for _, name := range []string{
		"-u", "-U", "-C", "-q", "--unified", "--brief", "--help", "--",
	} {
		writeFile(t, dir, name, "same\n")
		out, errb, code := runIn(t, dir, "", "first", name)
		if code != 0 || out != "" || errb != "" {
			t.Errorf("diff first %s = (%q, %q, %d), want identical operands", name, out, errb, code)
		}
	}
}

type issue7DiffErrorReader struct{}

func (issue7DiffErrorReader) Read([]byte) (int, error) {
	return 0, errors.New("injected stdin failure")
}

func TestIssue7StdinReadErrorIsTrouble(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "f", "x\n")
	out, errb, code := runInReader(t, dir, issue7DiffErrorReader{}, "-", "f")
	if code != 2 || out != "" || !strings.Contains(strings.ToLower(errb), "injected stdin failure") {
		t.Fatalf("diff stdin read error = (%q, %q, %d), want trouble diagnostic and status >1", out, errb, code)
	}
}

func runInReader(t *testing.T, dir string, in io.Reader, args ...string) (stdout, stderr string, code int) {
	t.Helper()
	var out, errb bytes.Buffer
	rc := &tool.RunContext{
		Ctx:   context.Background(),
		Dir:   dir,
		Stdio: tool.Stdio{In: in, Out: &out, Err: &errb},
	}
	code = cmd.Run(rc, args)
	return out.String(), errb.String(), code
}
