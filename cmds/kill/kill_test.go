package killcmd

import (
	"bytes"
	"context"
	"errors"
	"strconv"
	"strings"
	"testing"

	"github.com/qiangli/coreutils/tool"
)

func invoke(args ...string) (int, string, string) {
	var out, err bytes.Buffer
	rc := &tool.RunContext{Ctx: context.Background(), Stdio: tool.Stdio{Out: &out, Err: &err}}
	return run(rc, args), out.String(), err.String()
}

func TestKillListAndTranslateSignals(t *testing.T) {
	code, out, errOut := invoke("-l")
	if code != 0 || !strings.Contains(out, "TERM") || errOut != "" {
		t.Fatalf("kill -l: code=%d out=%q err=%q", code, out, errOut)
	}
	term := signalNumber(signalByName("TERM"))
	code, out, errOut = invoke("-l", strconv.Itoa(128+term))
	if code != 0 || out != "TERM\n" || errOut != "" {
		t.Fatalf("kill -l exit status: code=%d out=%q err=%q", code, out, errOut)
	}
	code, out, errOut = invoke("-l", "SIGTERM")
	if code != 0 || strings.TrimSpace(out) != strconv.Itoa(term) || errOut != "" {
		t.Fatalf("kill -l SIGTERM: code=%d out=%q err=%q", code, out, errOut)
	}
}

func TestKillSignalZeroCurrentProcess(t *testing.T) {
	pid := strconv.Itoa(currentPID())
	for _, args := range [][]string{{"-s", "0", pid}, {"-s0", "--", pid}, {"-n0", pid}} {
		code, out, errOut := invoke(args...)
		if code != 0 || out != "" || errOut != "" {
			t.Fatalf("kill %v self: code=%d out=%q err=%q", args, code, out, errOut)
		}
	}
}

func TestKillContinuesAfterBadPID(t *testing.T) {
	code, out, errOut := invoke("-0", "not-a-pid", strconv.Itoa(currentPID()))
	if code != 1 || out != "" || !strings.Contains(errOut, "not-a-pid") {
		t.Fatalf("kill operands: code=%d out=%q err=%q", code, out, errOut)
	}
}

func TestKillErrors(t *testing.T) {
	for _, args := range [][]string{nil, {"-s"}, {"-s", "BOGUS", "1"}, {"-l", "1", "2"}} {
		code, _, errOut := invoke(args...)
		if code == 0 || errOut == "" {
			t.Fatalf("kill %v: code=%d err=%q", args, code, errOut)
		}
	}
}

// errOutWriter fails every write, like a full device.
type errOutWriter struct{}

var errNoSpace = errors.New("no space left on device")

func (errOutWriter) Write(p []byte) (int, error) { return 0, errNoSpace }

// TestKillListWriteError: a failing stdout makes kill -l exit 1 with a
// write-error diagnostic (POSIX: an error occurred).
func TestKillListWriteError(t *testing.T) {
	var e bytes.Buffer
	rc := &tool.RunContext{Ctx: context.Background(), Stdio: tool.Stdio{Out: errOutWriter{}, Err: &e}}
	if code := run(rc, []string{"-l"}); code != 1 {
		t.Fatalf("kill -l write error: exit=%d, want 1", code)
	}
	if !strings.Contains(e.String(), "kill: write error:") {
		t.Fatalf("stderr=%q, want a write-error diagnostic", e.String())
	}
	if code := run(rc, []string{"-l", "TERM"}); code != 1 {
		t.Fatalf("kill -l TERM write error: exit=%d, want 1", code)
	}
}
