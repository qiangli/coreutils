package hostnamecmd

import (
	"bytes"
	"context"
	"os"
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

func TestHostname(t *testing.T) {
	want, err := os.Hostname()
	if err != nil {
		t.Skipf("os.Hostname: %v", err)
	}
	out, errb, code := runTool(t)
	if code != 0 || out != want+"\n" || errb != "" {
		t.Errorf("hostname = (%q, %q, %d), want (%q, \"\", 0)", out, errb, code, want+"\n")
	}
}

func TestHostnameSetNotSupported(t *testing.T) {
	_, errb, code := runTool(t, "newname")
	if code != 2 || !strings.Contains(errb, "not supported") {
		t.Errorf("set mode: code=%d err=%q, want contract error", code, errb)
	}
}

func TestHostnameFlags(t *testing.T) {
	out, _, code := runTool(t, "--help")
	if code != 0 || !strings.Contains(out, "Usage: hostname") {
		t.Errorf("--help: code=%d out=%q", code, out)
	}
	_, errb, code := runTool(t, "--frobnicate")
	if code != 2 || !strings.Contains(errb, "frobnicate") {
		t.Errorf("unknown flag: code=%d err=%q", code, errb)
	}
}

func TestHostnameUutilsAliases(t *testing.T) {
	want, err := os.Hostname()
	if err != nil {
		t.Skipf("os.Hostname: %v", err)
	}
	out, errb, code := runTool(t, "-s")
	if code != 0 || errb != "" {
		t.Fatalf("hostname -s = (%q, %q, %d)", out, errb, code)
	}
	short := want
	if i := strings.IndexByte(short, '.'); i >= 0 {
		short = short[:i]
	}
	if out != short+"\n" {
		t.Fatalf("hostname -s = %q, want %q", out, short+"\n")
	}
	out, _, code = runTool(t, "-h")
	if code != 0 || !strings.Contains(out, "Usage: hostname") {
		t.Fatalf("hostname -h = (%q, %d)", out, code)
	}
}
