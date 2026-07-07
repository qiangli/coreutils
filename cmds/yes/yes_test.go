package yescmd

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/qiangli/coreutils/tool"
)

// limitWriter accepts up to n bytes, then fails (a stand-in for a
// closed pipe).
type limitWriter struct {
	buf bytes.Buffer
	n   int
}

func (w *limitWriter) Write(p []byte) (int, error) {
	if w.buf.Len() >= w.n {
		return 0, errors.New("broken pipe")
	}
	return w.buf.Write(p)
}

func runYes(t *testing.T, ctx context.Context, w interface{ Write([]byte) (int, error) }, args ...string) (stderr string, code int) {
	t.Helper()
	var errb bytes.Buffer
	rc := &tool.RunContext{
		Ctx:   ctx,
		Dir:   t.TempDir(),
		Stdio: tool.Stdio{In: strings.NewReader(""), Out: w, Err: &errb},
	}
	code = cmd.Run(rc, args)
	return errb.String(), code
}

func TestYesDefault(t *testing.T) {
	w := &limitWriter{n: 1}
	stderr, code := runYes(t, context.Background(), w)
	if code != 1 || stderr != "" {
		t.Fatalf("code=%d stderr=%q, want quiet exit 1 on write error", code, stderr)
	}
	out := w.buf.String()
	if !strings.HasPrefix(out, "y\ny\n") {
		t.Errorf("output %q does not repeat \"y\\n\"", out[:min(20, len(out))])
	}
	for _, line := range strings.Split(strings.TrimSuffix(out, "\n"), "\n") {
		if line != "y" {
			t.Fatalf("unexpected line %q", line)
		}
	}
}

func TestYesOperands(t *testing.T) {
	w := &limitWriter{n: 1}
	_, code := runYes(t, context.Background(), w, "hello", "world")
	if code != 1 {
		t.Fatalf("code=%d, want 1", code)
	}
	if !strings.HasPrefix(w.buf.String(), "hello world\nhello world\n") {
		t.Errorf("output %q does not repeat \"hello world\"", w.buf.String()[:min(40, w.buf.Len())])
	}
}

func TestYesContextCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	var out bytes.Buffer
	stderr, code := runYes(t, ctx, &out)
	if code != 1 || stderr != "" || out.Len() != 0 {
		t.Errorf("pre-cancelled ctx: code=%d stderr=%q outlen=%d, want quiet immediate exit 1", code, stderr, out.Len())
	}
}

func TestYesFlags(t *testing.T) {
	var out bytes.Buffer
	_, code := runYes(t, context.Background(), &out, "--help")
	if code != 0 || !strings.Contains(out.String(), "Usage: yes") {
		t.Errorf("--help: code=%d out=%q", code, out.String())
	}
	out.Reset()
	_, code = runYes(t, context.Background(), &out, "-h")
	if code != 0 || !strings.Contains(out.String(), "Usage: yes") {
		t.Errorf("-h: code=%d out=%q", code, out.String())
	}
	out.Reset()
	_, code = runYes(t, context.Background(), &out, "-V")
	if code != 0 || !strings.Contains(out.String(), "yes") {
		t.Errorf("-V: code=%d out=%q", code, out.String())
	}
	out.Reset()
	stderr, code := runYes(t, context.Background(), &out, "--frobnicate")
	if code != 2 || !strings.Contains(stderr, "frobnicate") {
		t.Errorf("unknown flag: code=%d stderr=%q", code, stderr)
	}
}
