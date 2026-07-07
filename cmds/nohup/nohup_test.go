package nohupcmd

import (
	"bytes"
	"context"
	"runtime"
	"strings"
	"testing"

	"github.com/qiangli/coreutils/tool"
)

func TestNohupMissing(t *testing.T) {
	var out, errb bytes.Buffer
	code := run(&tool.RunContext{Ctx: context.Background(), Dir: t.TempDir(), Stdio: tool.Stdio{Out: &out, Err: &errb, In: strings.NewReader("")}}, nil)
	if code != 2 {
		t.Fatalf("code=%d", code)
	}
}

func TestNohupRunsCommand(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell command differs on windows")
	}
	var out, errb bytes.Buffer
	code := run(&tool.RunContext{Ctx: context.Background(), Dir: t.TempDir(), Env: []string{"PATH=/bin:/usr/bin"}, Stdio: tool.Stdio{Out: &out, Err: &errb, In: strings.NewReader("")}}, []string{"sh", "-c", "printf ok"})
	if code != 0 || out.String() != "ok" {
		t.Fatalf("code=%d out=%q err=%q", code, out.String(), errb.String())
	}
}
