package weave

import (
	"path/filepath"
	"testing"
)

func assertPullLockFree(t *testing.T, dir string) {
	t.Helper()
	release, err := weaveFlock(filepath.Join(dir, "pull.lock"), 0)
	if err != nil {
		t.Fatalf("pull.lock held for adversarial review: %v", err)
	}
	release()
}
