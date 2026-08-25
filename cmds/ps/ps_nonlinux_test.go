//go:build !linux

package pscmd

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/qiangli/coreutils/tool"
)

func TestPSNonLinuxLiveSourceFailsExplicitly(t *testing.T) {
	var out, errOut bytes.Buffer
	rc := &tool.RunContext{
		Ctx:   context.Background(),
		Env:   []string{"LC_ALL=C"},
		Stdio: tool.Stdio{Out: &out, Err: &errOut},
	}
	if code := run(rc, []string{"-A", "-o", "pid"}); code != 1 || out.Len() != 0 ||
		!strings.Contains(errOut.String(), "supported only on Linux") {
		t.Fatalf("non-Linux live ps=(code %d, stdout %q, stderr %q)", code, out.String(), errOut.String())
	}
}
