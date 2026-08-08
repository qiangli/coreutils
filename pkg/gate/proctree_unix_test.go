//go:build aix || darwin || dragonfly || freebsd || linux || netbsd || openbsd || solaris

package gate

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestRunLocalTimeoutReapsGateDescendants(t *testing.T) {
	dir := t.TempDir()
	marker := filepath.Join(dir, "leaked")
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	out := RunLocal(ctx, dir, "(sleep 0.3; echo leaked > leaked) & wait", "")
	if out.Passed {
		t.Fatal("timed-out gate must not pass")
	}
	time.Sleep(500 * time.Millisecond)
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("gate descendant survived cancellation: %v", err)
	}
}
