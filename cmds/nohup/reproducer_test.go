package nohupcmd

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/qiangli/coreutils/tool"
)

func TestNohupDiagnosticMessage(t *testing.T) {
	var errb bytes.Buffer
	rc := &tool.RunContext{
		Ctx:   context.Background(),
		Dir:   t.TempDir(),
		Env:   []string{"PATH=/bin:/usr/bin"},
		Stdio: tool.Stdio{In: strings.NewReader(""), Out: nil, Err: &errb},
	}
	run(rc, []string{"sh", "-c", "exit 0"})
	if !strings.Contains(errb.String(), "appending output") {
		t.Fatalf("expected diagnostic message in stderr, got: %q", errb.String())
	}
}
