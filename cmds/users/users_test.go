package userscmd

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/qiangli/coreutils/tool"
)

func TestUsersFile(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "utmp"), []byte("bob pts/1 1 host\nalice tty1 2 host\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	var out, errb bytes.Buffer
	code := run(&tool.RunContext{Ctx: context.Background(), Dir: dir, Stdio: tool.Stdio{Out: &out, Err: &errb}}, []string{"utmp"})
	if code != 0 || out.String() != "alice bob\n" {
		t.Fatalf("code=%d out=%q err=%q", code, out.String(), errb.String())
	}
}
