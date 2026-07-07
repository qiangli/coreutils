package pathchkcmd

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/qiangli/coreutils/tool"
)

func TestPathchkPortable(t *testing.T) {
	var out, err bytes.Buffer
	code := run(&tool.RunContext{Ctx: context.Background(), Dir: t.TempDir(), Stdio: tool.Stdio{Out: &out, Err: &err, In: strings.NewReader("")}}, []string{"-p", "abc/def"})
	if code != 0 {
		t.Fatalf("code=%d err=%s", code, err.String())
	}
}

func TestPathchkRejectsLeadingHyphen(t *testing.T) {
	var out, err bytes.Buffer
	code := run(&tool.RunContext{Ctx: context.Background(), Dir: t.TempDir(), Stdio: tool.Stdio{Out: &out, Err: &err, In: strings.NewReader("")}}, []string{"-P", "./-bad"})
	if code != 1 {
		t.Fatalf("code=%d out=%s err=%s", code, out.String(), err.String())
	}
}
