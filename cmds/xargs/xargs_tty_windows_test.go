//go:build windows

package xargscmd

import (
	"strings"
	"testing"
)

// This test is type-checked by crossvet. It pins Windows -p behavior to an
// explicit refusal, matching cmds/more's tty_windows.go precedent, rather
// than an unverified guess at reading a Windows console device.
func TestWindowsInteractiveModeIsExplicitlyUnsupported(t *testing.T) {
	_, err := defaultTTYOpener()
	if err == nil || !strings.Contains(err.Error(), "not supported on Windows") {
		t.Fatalf("defaultTTYOpener error = %v", err)
	}
}
