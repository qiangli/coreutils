//go:build !linux && !darwin

package pathchkcmd

import (
	"strings"
	"testing"
)

// The default checks require the containing directory's real limits; on
// platforms without a pathconf/statfs seam the query fails closed with a
// clear diagnostic instead of inventing limits.
func TestIssue741PlatformWithoutLimitsQueryFailsClosed(t *testing.T) {
	if _, _, err := filesystemLimits(t.TempDir()); err == nil ||
		!strings.Contains(err.Error(), "unsupported on this platform") {
		t.Fatalf("filesystemLimits err=%v, want unsupported-platform failure", err)
	}

	code, errText := runPathchk(t, t.TempDir(), "any/name")
	if code != 1 || !strings.Contains(errText, "cannot determine filesystem limits") {
		t.Fatalf("code=%d stderr=%q", code, errText)
	}
}
