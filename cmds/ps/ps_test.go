package pscmd

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/qiangli/coreutils/tool"
)

func TestPSOwnPIDAndFormat(t *testing.T) {
	var out, errOut bytes.Buffer
	rc := &tool.RunContext{Ctx: context.Background(), Stdio: tool.Stdio{Out: &out, Err: &errOut}}
	code := run(rc, []string{"-p", "1", "-o", "pid="})
	if code != 0 {
		t.Fatalf("code=%d stderr=%s", code, errOut.String())
	}
	if !strings.Contains(out.String(), "1") {
		t.Fatalf("output=%q", out.String())
	}
}

func TestPSRejectsUnknownFormat(t *testing.T) {
	var out, errOut bytes.Buffer
	rc := &tool.RunContext{Ctx: context.Background(), Stdio: tool.Stdio{Out: &out, Err: &errOut}}
	if code := run(rc, []string{"-o", "not-a-field"}); code != 2 {
		t.Fatalf("code=%d stderr=%s", code, errOut.String())
	}
}
