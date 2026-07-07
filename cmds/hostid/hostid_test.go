package hostidcmd

import (
	"bytes"
	"context"
	"regexp"
	"testing"

	"github.com/qiangli/coreutils/tool"
)

func TestHostidFormat(t *testing.T) {
	var out, err bytes.Buffer
	code := run(&tool.RunContext{Ctx: context.Background(), Stdio: tool.Stdio{Out: &out, Err: &err}}, nil)
	if code != 0 {
		t.Fatalf("code=%d err=%s", code, err.String())
	}
	if !regexp.MustCompile(`^[0-9a-f]{8}\n$`).MatchString(out.String()) {
		t.Fatalf("bad hostid %q", out.String())
	}
}
