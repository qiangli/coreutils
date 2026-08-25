//go:build windows

package unamecmd

import (
	"strings"
	"testing"
)

func TestWindowsProbeHasPOSIXVersion(t *testing.T) {
	info, err := probe()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(info.version, "Build ") || strings.TrimSpace(info.version) == "Build" {
		t.Fatalf("Windows -v symbol = %q, want non-empty build string", info.version)
	}
}
