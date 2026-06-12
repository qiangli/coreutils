package whoamicmd

import (
	"bytes"
	"context"
	"os/user"
	"runtime"
	"strings"
	"testing"

	"github.com/qiangli/coreutils/tool"
)

func runTool(t *testing.T, args ...string) (stdout, stderr string, code int) {
	t.Helper()
	var out, errb bytes.Buffer
	rc := &tool.RunContext{
		Ctx:   context.Background(),
		Dir:   t.TempDir(),
		Stdio: tool.Stdio{In: strings.NewReader(""), Out: &out, Err: &errb},
	}
	code = cmd.Run(rc, args)
	return out.String(), errb.String(), code
}

func TestWhoami(t *testing.T) {
	u, err := user.Current()
	if err != nil {
		t.Skipf("user.Current: %v", err)
	}
	want := u.Username
	if runtime.GOOS == "windows" {
		if i := strings.LastIndexByte(want, '\\'); i >= 0 {
			want = want[i+1:]
		}
	}
	out, errb, code := runTool(t)
	if code != 0 || out != want+"\n" || errb != "" {
		t.Errorf("whoami = (%q, %q, %d), want (%q, \"\", 0)", out, errb, code, want+"\n")
	}
}

func TestWhoamiErrors(t *testing.T) {
	_, errb, code := runTool(t, "extra")
	if code != 2 || !strings.Contains(errb, "extra operand") {
		t.Errorf("operand: code=%d err=%q", code, errb)
	}
	_, errb, code = runTool(t, "--frobnicate")
	if code != 2 || !strings.Contains(errb, "frobnicate") {
		t.Errorf("unknown flag: code=%d err=%q", code, errb)
	}
}

func TestWhoamiHelp(t *testing.T) {
	out, _, code := runTool(t, "--help")
	if code != 0 || !strings.Contains(out, "Usage: whoami") {
		t.Errorf("--help: code=%d out=%q", code, out)
	}
}
