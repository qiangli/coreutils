package pwdcmd

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/qiangli/coreutils/tool"
)

type failingWriter struct{ err error }

func (w failingWriter) Write([]byte) (int, error) { return 0, w.err }

func TestPwdReportsStandardOutputFailure(t *testing.T) {
	var errOut bytes.Buffer
	dir := t.TempDir()
	rc := &tool.RunContext{Ctx: context.Background(), Dir: dir, Env: []string{"PWD=" + dir, "POSIXLY_CORRECT=1"}, Stdio: tool.Stdio{In: strings.NewReader(""), Out: failingWriter{errors.New("closed")}, Err: &errOut}}
	if code := cmd.Run(rc, nil); code == 0 || !strings.Contains(errOut.String(), "pwd: write error:") {
		t.Fatalf("pwd output failure = (%q, %d)", errOut.String(), code)
	}
}

func TestPwdNativeCurrentDirectoryAvoidsPATHMAXLookup(t *testing.T) {
	long := "/" + strings.Repeat("segment/", 700) + "leaf"

	var out, errOut bytes.Buffer
	rc := &tool.RunContext{Ctx: context.Background(), Dir: long, DirIsProcessCwd: true, Env: []string{"PWD=" + long, "POSIXLY_CORRECT=1"}, Stdio: tool.Stdio{In: strings.NewReader(""), Out: &out, Err: &errOut}}
	if code := cmd.Run(rc, []string{"-L"}); code != 0 || out.String() != long+"\n" || errOut.Len() != 0 {
		t.Fatalf("pwd -L long = (%q, %q, %d)", out.String(), errOut.String(), code)
	}

	oldGetwd := processGetwd
	processGetwd = func() (string, error) { return long, nil }
	t.Cleanup(func() { processGetwd = oldGetwd })
	out.Reset()
	if code := cmd.Run(rc, []string{"-P"}); code != 0 || out.String() != long+"\n" || errOut.Len() != 0 {
		t.Fatalf("pwd -P long = (%q, %q, %d)", out.String(), errOut.String(), code)
	}
}
