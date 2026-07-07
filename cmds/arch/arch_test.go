package archcmd

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/qiangli/coreutils/tool"
)

func TestArchPrintsMachine(t *testing.T) {
	var out, err bytes.Buffer
	code := run(&tool.RunContext{Ctx: context.Background(), Stdio: tool.Stdio{Out: &out, Err: &err}}, nil)
	if code != 0 {
		t.Fatalf("code=%d err=%s", code, err.String())
	}
	if strings.TrimSpace(out.String()) == "" {
		t.Fatalf("empty arch output")
	}
}
