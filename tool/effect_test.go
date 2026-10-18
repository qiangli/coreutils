package tool

import (
	"bytes"
	"errors"
	"testing"
)

type effectBadWriter struct{}

func (effectBadWriter) Write([]byte) (int, error) { return 0, errors.New("broken writer") }
func TestEffectSeam(t *testing.T) {
	var out, errb bytes.Buffer
	rc := &RunContext{Stdio: Stdio{Out: &out, Err: &errb}}
	if !rc.Effect("remove", "file") || out.Len() != 0 || len(rc.Effects) != 0 {
		t.Fatal("normal path changed")
	}
	rc.dryRun = true
	if rc.Effect("remove", "a\nb") || out.String() != "planned (local-preview, conditional): remove \"a\\nb\"\n" || len(rc.Effects) != 1 {
		t.Fatalf("bad preview %q", &out)
	}
	rc.Env = []string{"VSC_PROFILE=cert"}
	out.Reset()
	if !rc.Effect("remove", "file") || out.Len() != 0 {
		t.Fatal("cert path changed")
	}
}
func TestEffectWriteFailureAndInvocationReset(t *testing.T) {
	cmd := &Tool{Name: "effect-test", PreviewLevel: "local-preview"}
	acted := false
	cmd.Run = func(rc *RunContext, args []string) int {
		_, code := Parse(rc, cmd, NewFlags(cmd.Name), args)
		if code >= 0 {
			return code
		}
		if rc.Effect("test", "file") {
			acted = true
		}
		return 0
	}
	Register(cmd)
	rc := &RunContext{Stdio: Stdio{Out: effectBadWriter{}, Err: &bytes.Buffer{}}}
	if code := cmd.Run(rc, []string{"--dry-run"}); code != 1 || acted {
		t.Fatalf("code=%d acted=%t", code, acted)
	}
	rc.Out = &bytes.Buffer{}
	if code := cmd.Run(rc, nil); code != 0 || !acted || len(rc.Effects) != 0 {
		t.Fatalf("state leaked: code=%d acted=%t", code, acted)
	}
}
