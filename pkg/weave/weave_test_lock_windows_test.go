//go:build windows

package weave

import "testing"

func assertPullLockFree(t *testing.T, _ string) {
	t.Helper()
	t.Log("pull.lock availability assertion requires Unix flock; Windows weave locking is currently a no-op")
}
