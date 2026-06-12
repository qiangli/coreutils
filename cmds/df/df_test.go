package dfcmd

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/qiangli/coreutils/tool"
)

// runToolAt is the canonical test harness shape for cmds packages,
// with an explicit working directory.
func runToolAt(t *testing.T, dir string, args ...string) (stdout, stderr string, code int) {
	t.Helper()
	var out, errb bytes.Buffer
	rc := &tool.RunContext{
		Ctx:   context.Background(),
		Dir:   dir,
		Stdio: tool.Stdio{In: strings.NewReader(""), Out: &out, Err: &errb},
	}
	code = cmd.Run(rc, args)
	return out.String(), errb.String(), code
}

func runTool(t *testing.T, args ...string) (string, string, int) {
	t.Helper()
	return runToolAt(t, t.TempDir(), args...)
}

func TestDefaultListing(t *testing.T) {
	out, errb, code := runTool(t)
	if code != 0 {
		t.Fatalf("df code = %d, stderr = %q", code, errb)
	}
	lines := strings.Split(strings.TrimSuffix(out, "\n"), "\n")
	if len(lines) < 2 {
		t.Fatalf("df output = %q, want header + at least one mount", out)
	}
	hdr := lines[0]
	for _, col := range []string{"Filesystem", "1K-blocks", "Used", "Available", "Use%", "Mounted on"} {
		if !strings.Contains(hdr, col) {
			t.Errorf("header %q missing column %q", hdr, col)
		}
	}
}

func TestKFlagSameUnits(t *testing.T) {
	out, _, code := runTool(t, "-k")
	if code != 0 || !strings.Contains(out, "1K-blocks") {
		t.Errorf("df -k: code=%d out lacks 1K-blocks header: %q", code, firstLine(out))
	}
}

func TestHumanHeader(t *testing.T) {
	out, _, code := runTool(t, "-h")
	if code != 0 {
		t.Fatalf("df -h code = %d", code)
	}
	hdr := firstLine(out)
	for _, col := range []string{"Filesystem", "Size", "Used", "Avail", "Use%", "Mounted on"} {
		if !strings.Contains(hdr, col) {
			t.Errorf("df -h header %q missing %q", hdr, col)
		}
	}
	if strings.Contains(hdr, "1K-blocks") {
		t.Errorf("df -h header still shows 1K-blocks: %q", hdr)
	}
}

func TestFileOperand(t *testing.T) {
	out, errb, code := runTool(t, ".")
	if code != 0 {
		t.Fatalf("df . code = %d, stderr = %q", code, errb)
	}
	lines := strings.Split(strings.TrimSuffix(out, "\n"), "\n")
	if len(lines) != 2 {
		t.Errorf("df . = %q, want header + exactly one mount line", out)
	}
}

func TestNonexistentOperand(t *testing.T) {
	out, errb, code := runTool(t, "definitely-not-here")
	if code != 1 || !strings.Contains(errb, "definitely-not-here") {
		t.Errorf("df missing file: code=%d err=%q", code, errb)
	}
	if !strings.Contains(errb, "no file systems processed") {
		t.Errorf("df missing file stderr = %q, want trailing summary error", errb)
	}
	if out != "" {
		t.Errorf("df missing file stdout = %q, want empty", out)
	}
}

func TestUsePct(t *testing.T) {
	cases := []struct {
		used, avail uint64
		want        string
	}{
		{0, 0, "-"},
		{0, 100, "0%"},
		{50, 50, "50%"},
		{1, 99, "1%"},  // 1.0 -> 1, exact
		{1, 199, "1%"}, // 0.5 rounds up
		{99, 1, "99%"},
		{100, 0, "100%"},
	}
	for _, c := range cases {
		if got := usePct(c.used, c.avail); got != c.want {
			t.Errorf("usePct(%d, %d) = %q, want %q", c.used, c.avail, got, c.want)
		}
	}
}

func TestUnknownFlag(t *testing.T) {
	_, errb, code := runTool(t, "--frobnicate")
	if code != 2 || !strings.Contains(errb, "frobnicate") || !strings.Contains(errb, "pure-Go") {
		t.Errorf("unknown flag: code=%d err=%q", code, errb)
	}
}

func TestHelpAndVersion(t *testing.T) {
	out, _, code := runTool(t, "--help")
	if code != 0 || !strings.Contains(out, "Usage: df") {
		t.Errorf("--help: code=%d out=%q", code, out)
	}
	out, _, code = runTool(t, "--version")
	if code != 0 || !strings.Contains(out, "df") {
		t.Errorf("--version: code=%d out=%q", code, out)
	}
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}
