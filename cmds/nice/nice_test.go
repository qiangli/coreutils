package nicecmd

import (
	"bytes"
	"context"
	"strconv"
	"strings"
	"testing"

	"github.com/qiangli/coreutils/tool"
)

func TestNicePrintsCurrent(t *testing.T) {
	var out, errb bytes.Buffer
	code := run(&tool.RunContext{Ctx: context.Background(), Stdio: tool.Stdio{Out: &out, Err: &errb}}, nil)
	if code != 0 {
		t.Fatalf("code=%d err=%q", code, errb.String())
	}
	if _, err := strconv.Atoi(strings.TrimSpace(out.String())); err != nil {
		t.Fatalf("out=%q", out.String())
	}
}
