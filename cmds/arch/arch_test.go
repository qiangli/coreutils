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

func TestArchHelpVersionAliases(t *testing.T) {
	for _, tt := range []struct {
		args []string
		want string
	}{
		{[]string{"-h"}, "Usage: arch"},
		{[]string{"-V"}, "arch"},
	} {
		var out, err bytes.Buffer
		code := run(&tool.RunContext{Ctx: context.Background(), Stdio: tool.Stdio{Out: &out, Err: &err}}, tt.args)
		if code != 0 || !strings.Contains(out.String(), tt.want) || err.Len() != 0 {
			t.Errorf("arch %v: code=%d out=%q err=%q", tt.args, code, out.String(), err.String())
		}
	}
}
