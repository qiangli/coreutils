package atcmd

import (
	"bytes"
	"context"
	"os"
	"strings"
	"testing"

	"github.com/qiangli/coreutils/tool"
)

func runTool(tb testing.TB, ctx context.Context, stdin string, args ...string) (string, string, int) {
	tb.Helper()
	var out, errb bytes.Buffer
	rc := &tool.RunContext{
		Ctx:   ctx,
		Dir:   tb.TempDir(),
		Env:   []string{"BASHY_SCHEDULE_STATE=" + os.Getenv("BASHY_SCHEDULE_STATE")},
		Stdio: tool.Stdio{In: strings.NewReader(stdin), Out: &out, Err: &errb},
	}
	code := cmd.Run(rc, args)
	return out.String(), errb.String(), code
}
