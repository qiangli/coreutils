package killcmd

import (
	"bytes"
	"context"
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
