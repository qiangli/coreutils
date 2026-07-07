package falsecmd

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/qiangli/coreutils/tool"
)

func TestFalseHelpVersionAliases(t *testing.T) {
	for _, args := range [][]string{{"--help"}, {"-h"}, {"--version"}, {"-V"}} {
		var out, err bytes.Buffer
		code := cmdRun(&tool.RunContext{Ctx: context.Background(), Stdio: tool.Stdio{Out: &out, Err: &err, In: strings.NewReader("")}}, args)
		if code != 0 || err.String() != "" || out.String() == "" {
			t.Fatalf("false %v: code=%d out=%q err=%q", args, code, out.String(), err.String())
		}
	}
}

func TestFalseDefaultStatus(t *testing.T) {
	var out, err bytes.Buffer
	code := cmdRun(&tool.RunContext{Ctx: context.Background(), Stdio: tool.Stdio{Out: &out, Err: &err, In: strings.NewReader("")}}, nil)
	if code != 1 || out.String() != "" || err.String() != "" {
		t.Fatalf("false: code=%d out=%q err=%q", code, out.String(), err.String())
	}
}

func cmdRun(rc *tool.RunContext, args []string) int {
	if t := tool.Lookup("false"); t != nil {
		return t.Run(rc, args)
	}
	return 127
}
