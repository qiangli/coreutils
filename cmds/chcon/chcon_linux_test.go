//go:build linux

package chconcmd

import (
	"strings"
	"testing"
)

func TestChconLinuxReportsSetxattrErrors(t *testing.T) {
	dir := t.TempDir()
	_, errb, code := runTool(t, dir, "system_u:object_r:tmp_t:s0", "missing")
	if code != 1 || !strings.Contains(errb, "missing") || !strings.Contains(errb, "system_u:object_r:tmp_t:s0") {
		t.Fatalf("missing file: code=%d err=%q", code, errb)
	}
}
