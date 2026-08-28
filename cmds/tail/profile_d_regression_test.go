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
