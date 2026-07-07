package runconcmd

import (
	"bytes"
	"context"
	"testing"

	"github.com/qiangli/coreutils/tool"
)

func TestRunconHelp(t *testing.T) {
	var out, errb bytes.Buffer
	code := run(&tool.RunContext{Ctx: context.Background(), Stdio: tool.Stdio{Out: &out, Err: &errb}}, []string{"--help"})
	if code != 0 || out.Len() == 0 {
		t.Fatalf("code=%d out=%q err=%q", code, out.String(), errb.String())
	}
}
