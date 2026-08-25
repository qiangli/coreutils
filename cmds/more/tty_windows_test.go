//go:build windows

package morecmd

import (
	"context"
	"strings"
	"testing"

	"github.com/qiangli/coreutils/tool"
)

// This test is type-checked by crossvet. It pins Windows terminal behavior to
// an explicit refusal until a cancellable console implementation exists.
func TestWindowsControllingTerminalIsExplicitlyUnsupported(t *testing.T) {
	_, err := openControllingTTY(&tool.RunContext{Ctx: context.Background()})
	if err == nil || !strings.Contains(err.Error(), "not supported on Windows") {
		t.Fatalf("openControllingTTY error = %v", err)
	}
}
