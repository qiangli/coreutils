package truecmd

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/qiangli/coreutils/tool"
)

func TestTrueHelpVersionAliases(t *testing.T) {
	for _, args := range [][]string{{"--help"}, {"-h"}, {"--version"}, {"-V"}} {
		var out, err bytes.Buffer
		code := cmdRun(&tool.RunContext{Ctx: context.Background(), Stdio: tool.Stdio{Out: &out, Err: &err, In: strings.NewReader("")}}, args)
		if code != 0 || err.String() != "" || out.String() == "" {
			t.Fatalf("true %v: code=%d out=%q err=%q", args, code, out.String(), err.String())
		}
		if args[0] == "--help" && (!strings.Contains(out.String(), "--help") || !strings.Contains(out.String(), "--version")) {
			t.Fatalf("true help missing long options: %q", out.String())
		}
	}
}

func cmdRun(rc *tool.RunContext, args []string) int {
	if t := tool.Lookup("true"); t != nil {
		return t.Run(rc, args)
	}
	return 127
}
