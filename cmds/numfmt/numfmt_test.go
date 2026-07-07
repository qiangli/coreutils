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

func TestNumfmtFieldDelimiterSuffixAndSeparator(t *testing.T) {
	out, errb, code := runTool(t, "a,1000,x\nb,2000,y\n", "-d", ",", "--field=2", "--to=si", "--format=%.1f", "--suffix=B", "--unit-separator= ")
	if code != 0 || errb != "" {
		t.Fatalf("code=%d err=%q", code, errb)
	}
	if out != "a,1 KB,x\nb,2 KB,y\n" {
		t.Fatalf("out=%q", out)
	}
}

func TestNumfmtUnitsPaddingAndInvalidFail(t *testing.T) {
	out, errb, code := runTool(t, "", "--from-unit=2", "--to-unit=4", "--padding=4", "--format=%.0f", "10")
	if code != 0 || errb != "" {
		t.Fatalf("code=%d err=%q", code, errb)
	}
	if out != "   5\n" {
		t.Fatalf("out=%q", out)
	}

	out, errb, code = runTool(t, "x 2\n", "--field=-", "--invalid=fail", "--format=%.0f")
	if code != 1 || !strings.Contains(errb, "invalid number") {
		t.Fatalf("code=%d err=%q", code, errb)
	}
	if out != "x 2\n" {
		t.Fatalf("out=%q", out)
	}
}

func TestNumfmtHeaderZeroRoundAndGrouping(t *testing.T) {
	out, errb, code := runTool(t, "name\x0012345.67\x00", "-z", "--header=1", "--grouping", "--round=down", "--format=%.1f")
	if code != 0 || errb != "" {
		t.Fatalf("code=%d err=%q", code, errb)
	}
	if out != "name\x0012,345.6\x00" {
		t.Fatalf("out=%q", out)
	}

	out, errb, code = runTool(t, "", "--round=towards-zero", "--format=%.0f", "--", "-12.9")
	if code != 0 || errb != "" {
		t.Fatalf("code=%d err=%q", code, errb)
	}
	if out != "-12\n" {
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
	_, errb, code = runTool(t, "", "--round=sideways", "1")
	if code != 2 || !strings.Contains(errb, "invalid --round") {
		t.Fatalf("code=%d err=%q", code, errb)
	}
}
