package uptimecmd

import (
	"bytes"
	"context"
	"regexp"
	"runtime"
	"strings"
	"testing"
	"time"

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

var shape = regexp.MustCompile(`^ \d{2}:\d{2}:\d{2} up (\d+ days?, )?(\d+:\d{2}|\d+ min)(,  load average: [\d.]+, [\d.]+, [\d.]+)?\n$`)

func TestUptimeShape(t *testing.T) {
	out, errb, code := runTool(t)
	if code != 0 || errb != "" {
		t.Fatalf("uptime: code=%d err=%q", code, errb)
	}
	if !shape.MatchString(out) {
		t.Errorf("output %q does not match the procps shape", out)
	}
	if runtime.GOOS == "linux" && !strings.Contains(out, "load average:") {
		t.Errorf("linux output %q missing load averages", out)
	}
	if runtime.GOOS != "linux" && strings.Contains(out, "load average:") {
		t.Errorf("non-linux output %q should omit load averages", out)
	}
}

func TestFormatUptime(t *testing.T) {
	cases := []struct {
		d    time.Duration
		want string
	}{
		{45 * time.Second, "0 min"},
		{5 * time.Minute, "5 min"},
		{59 * time.Minute, "59 min"},
		{time.Hour, "1:00"},
		{3*time.Hour + 4*time.Minute, "3:04"},
		{24 * time.Hour, "1 day, 0 min"},
		{25*time.Hour + 7*time.Minute, "1 day, 1:07"},
		{49*time.Hour + 30*time.Minute, "2 days, 1:30"},
	}
	for _, c := range cases {
		if got := formatUptime(c.d); got != c.want {
			t.Errorf("formatUptime(%v) = %q, want %q", c.d, got, c.want)
		}
	}
}

func TestUptimeErrors(t *testing.T) {
	_, errb, code := runTool(t, "extra")
	if code != 2 || !strings.Contains(errb, "extra operand") {
		t.Errorf("operand: code=%d err=%q", code, errb)
	}
	_, errb, code = runTool(t, "--frobnicate")
	if code != 2 || !strings.Contains(errb, "frobnicate") {
		t.Errorf("unknown flag: code=%d err=%q", code, errb)
	}
}

func TestUptimeHelp(t *testing.T) {
	out, _, code := runTool(t, "--help")
	if code != 0 || !strings.Contains(out, "Usage: uptime") {
		t.Errorf("--help: code=%d out=%q", code, out)
	}
}
