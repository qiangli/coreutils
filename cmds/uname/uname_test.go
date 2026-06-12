package unamecmd

import (
	"bytes"
	"context"
	"os"
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

func wantSysname() string {
	switch runtime.GOOS {
	case "linux":
		return "Linux"
	case "darwin":
		return "Darwin"
	case "windows":
		return "Windows_NT"
	default:
		return ""
	}
}

func TestUnameDefaultIsKernelName(t *testing.T) {
	out, _, code := runTool(t)
	if code != 0 {
		t.Fatalf("code=%d", code)
	}
	if want := wantSysname(); want != "" && out != want+"\n" {
		t.Errorf("uname = %q, want %q", out, want+"\n")
	}
	s, _, _ := runTool(t, "-s")
	if s != out {
		t.Errorf("-s (%q) differs from default (%q)", s, out)
	}
}

func TestUnameFields(t *testing.T) {
	host, err := os.Hostname()
	if err != nil {
		t.Fatal(err)
	}
	out, _, code := runTool(t, "-n")
	if code != 0 || out != host+"\n" {
		t.Errorf("-n = (%q, %d), want %q", out, code, host+"\n")
	}
	for _, flag := range []string{"-r", "-m", "-o"} {
		out, _, code := runTool(t, flag)
		if code != 0 || strings.TrimSpace(out) == "" || strings.Count(out, "\n") != 1 {
			t.Errorf("%s = (%q, %d), want one non-empty line", flag, out, code)
		}
	}
	out, _, _ = runTool(t, "-o")
	switch runtime.GOOS {
	case "linux":
		if out != "GNU/Linux\n" {
			t.Errorf("-o = %q, want GNU/Linux", out)
		}
	case "darwin":
		if out != "Darwin\n" {
			t.Errorf("-o = %q, want Darwin", out)
		}
	case "windows":
		if out != "Windows_NT\n" {
			t.Errorf("-o = %q, want Windows_NT", out)
		}
	}
}

func TestUnameCombinedAndAll(t *testing.T) {
	// Output order is fixed regardless of flag order: s n r m o.
	a, _, _ := runTool(t, "-s", "-n")
	b, _, _ := runTool(t, "-n", "-s")
	if a != b {
		t.Errorf("flag order changed output: %q vs %q", a, b)
	}
	s, _, _ := runTool(t, "-s")
	n, _, _ := runTool(t, "-n")
	want := strings.TrimSuffix(s, "\n") + " " + n
	if a != want {
		t.Errorf("-s -n = %q, want %q", a, want)
	}

	all, _, code := runTool(t, "-a")
	if code != 0 {
		t.Fatalf("-a: code=%d", code)
	}
	host, _ := os.Hostname()
	for _, part := range []string{strings.TrimSpace(s), host} {
		if !strings.Contains(all, part) {
			t.Errorf("-a output %q missing %q", all, part)
		}
	}
	if strings.Count(all, "\n") != 1 {
		t.Errorf("-a output is not a single line: %q", all)
	}
}

func TestUnameErrors(t *testing.T) {
	_, errb, code := runTool(t, "extra")
	if code != 2 || !strings.Contains(errb, "extra operand") {
		t.Errorf("operand: code=%d err=%q", code, errb)
	}
	_, errb, code = runTool(t, "-p")
	if code != 2 || !strings.Contains(errb, "p") {
		t.Errorf("-p (not implemented): code=%d err=%q", code, errb)
	}
	_, errb, code = runTool(t, "--frobnicate")
	if code != 2 || !strings.Contains(errb, "frobnicate") {
		t.Errorf("unknown flag: code=%d err=%q", code, errb)
	}
}

func TestUnameHelp(t *testing.T) {
	out, _, code := runTool(t, "--help")
	if code != 0 || !strings.Contains(out, "Usage: uname") {
		t.Errorf("--help: code=%d out=%q", code, out)
	}
}
