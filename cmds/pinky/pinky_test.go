package pinkycmd

import (
	"bytes"
	"context"
	"testing"

	"github.com/qiangli/coreutils/tool"
)

func TestPinkyNoUsers(t *testing.T) {
	var out, errb bytes.Buffer
	code := run(&tool.RunContext{Ctx: context.Background(), Stdio: tool.Stdio{Out: &out, Err: &errb}}, nil)
	if code != 0 {
		t.Fatalf("code=%d err=%q", code, errb.String())
	}
}
