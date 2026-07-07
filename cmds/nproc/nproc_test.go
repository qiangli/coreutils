package nproccmd

import (
	"bytes"
	"context"
	"strconv"
	"strings"
	"testing"

	"github.com/qiangli/coreutils/tool"
)

func TestNprocDefault(t *testing.T) {
	var out, err bytes.Buffer
	code := run(&tool.RunContext{Ctx: context.Background(), Stdio: tool.Stdio{Out: &out, Err: &err}}, nil)
	if code != 0 {
		t.Fatalf("code=%d err=%s", code, err.String())
	}
	n, e := strconv.Atoi(strings.TrimSpace(out.String()))
	if e != nil || n < 1 {
		t.Fatalf("bad nproc output %q", out.String())
	}
}

func TestNprocIgnoreClampsToOne(t *testing.T) {
	var out, err bytes.Buffer
	code := run(&tool.RunContext{Ctx: context.Background(), Stdio: tool.Stdio{Out: &out, Err: &err}}, []string{"--ignore=999999"})
	if code != 0 {
		t.Fatalf("code=%d err=%s", code, err.String())
	}
	if strings.TrimSpace(out.String()) != "1" {
		t.Fatalf("out=%q", out.String())
	}
}
