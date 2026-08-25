//go:build windows

package newgrpcmd

import (
	"strings"
	"testing"

	"github.com/qiangli/coreutils/tool"
)

func TestWindowsSpawnFailsLoudly(t *testing.T) {
	status, err := defaultSpawnShell(&tool.RunContext{}, shellSpec{})
	if status == 0 && err == nil {
		t.Fatal("Windows newgrp must never claim a successful group change")
	}
	if err == nil || !strings.Contains(err.Error(), "not a Windows concept") {
		t.Fatalf("error = %v, want explicit unsupported-platform diagnostic", err)
	}
}
