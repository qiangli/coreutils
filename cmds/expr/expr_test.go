package exprcmd

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/qiangli/coreutils/tool"
)

func TestExprArithmetic(t *testing.T) {
	var out, err bytes.Buffer
	code := run(&tool.RunContext{Ctx: context.Background(), Stdio: tool.Stdio{Out: &out, Err: &err, In: strings.NewReader("")}}, []string{"1", "+", "2", "*", "3"})
	if code != 0 || out.String() != "7\n" {
		t.Fatalf("code=%d out=%q err=%s", code, out.String(), err.String())
	}
}

func TestExprMatch(t *testing.T) {
	var out, err bytes.Buffer
	code := run(&tool.RunContext{Ctx: context.Background(), Stdio: tool.Stdio{Out: &out, Err: &err, In: strings.NewReader("")}}, []string{"abc123", ":", "[a-z]*\\([0-9]*\\)"})
	if code != 0 || out.String() != "123\n" {
		t.Fatalf("code=%d out=%q err=%s", code, out.String(), err.String())
	}
}

func TestExprHelpVersionAliases(t *testing.T) {
	for _, args := range [][]string{{"--help"}, {"-h"}, {"--version"}, {"-V"}} {
		var out, err bytes.Buffer
		code := run(&tool.RunContext{Ctx: context.Background(), Stdio: tool.Stdio{Out: &out, Err: &err, In: strings.NewReader("")}}, args)
		if code != 0 || err.String() != "" || out.String() == "" {
			t.Fatalf("expr %v: code=%d out=%q err=%q", args, code, out.String(), err.String())
		}
	}
}
