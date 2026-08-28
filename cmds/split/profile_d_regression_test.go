package splitcmd

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/qiangli/coreutils/tool"
)

func TestPOSIXStopsOptionParsingAtInputOperand(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "input"), []byte("one\ntwo\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	var out, errOut bytes.Buffer
	rc := &tool.RunContext{Ctx: context.Background(), Dir: dir, Env: []string{"POSIXLY_CORRECT=1"}, Stdio: tool.Stdio{In: strings.NewReader(""), Out: &out, Err: &errOut}}
	if code := cmd.Run(rc, []string{"input", "--lin"}); code != 0 || errOut.Len() != 0 {
		t.Fatalf("split post-operand -l = (%q, %d)", errOut.String(), code)
	}
	got, err := os.ReadFile(filepath.Join(dir, "--linaa"))
	if err != nil || string(got) != "one\ntwo\n" {
		t.Fatalf("split --linaa = (%q, %v)", got, err)
	}
}
