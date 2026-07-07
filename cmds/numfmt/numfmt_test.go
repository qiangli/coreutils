package numfmtcmd

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/qiangli/coreutils/tool"
)

func runTool(t *testing.T, stdin string, args ...string) (stdout, stderr string, code int) {
	t.Helper()
	var out, errb bytes.Buffer
	rc := &tool.RunContext{
		Ctx:   context.Background(),
		Stdio: tool.Stdio{In: strings.NewReader(stdin), Out: &out, Err: &errb},
	}
	code = cmd.Run(rc, args)
	return out.String(), errb.String(), code
}

func TestNumfmtToSI(t *testing.T) {
	out, errb, code := runTool(t, "", "--to=si", "--format=%.1f", "1500")
	if code != 0 || errb != "" {
		t.Fatalf("code=%d err=%q", code, errb)
	}
	if out != "1.5K\n" {
		t.Fatalf("out=%q", out)
	}
}

func TestNumfmtFromIEC(t *testing.T) {
	out, errb, code := runTool(t, "", "--from=iec", "--format=%.0f", "2K")
	if code != 0 || errb != "" {
		t.Fatalf("code=%d err=%q", code, errb)
	}
	if out != "2048\n" {
		t.Fatalf("out=%q", out)
	}
}

func TestNumfmtFromAutoAndToIECI(t *testing.T) {
	out, errb, code := runTool(t, "", "--from=auto", "--to=iec-i", "--format=%.1f", "1536")
	if code != 0 || errb != "" {
		t.Fatalf("code=%d err=%q", code, errb)
	}
	if out != "1.5Ki\n" {
		t.Fatalf("out=%q", out)
	}

	out, errb, code = runTool(t, "", "--from=auto", "--format=%.0f", "2KiB")
	if code != 0 || errb != "" {
		t.Fatalf("code=%d err=%q", code, errb)
	}
	if out != "2048\n" {
		t.Fatalf("out=%q", out)
	}
}

func TestNumfmtStdinFields(t *testing.T) {
	out, errb, code := runTool(t, "1K 2K\n", "--from=si", "--format=%.0f")
	if code != 0 || errb != "" {
		t.Fatalf("code=%d err=%q", code, errb)
	}
	if out != "1000 2000\n" {
		t.Fatalf("out=%q", out)
	}
}

func TestNumfmtErrors(t *testing.T) {
	_, errb, code := runTool(t, "", "--to=bad", "1")
	if code != 2 || !strings.Contains(errb, "not supported") {
		t.Fatalf("code=%d err=%q", code, errb)
	}
	_, errb, code = runTool(t, "", "x")
	if code != 1 || !strings.Contains(errb, "invalid number") {
		t.Fatalf("code=%d err=%q", code, errb)
	}
	_, errb, code = runTool(t, "", "--from=si", "1Ki")
	if code != 1 || !strings.Contains(errb, "invalid number") {
		t.Fatalf("code=%d err=%q", code, errb)
	}
	_, errb, code = runTool(t, "", "--to=auto", "1")
	if code != 2 || !strings.Contains(errb, "not supported") {
		t.Fatalf("code=%d err=%q", code, errb)
	}
}
