//go:build !windows

package weave

import (
	"fmt"
	"os"
	"path/filepath"
	"syscall"
)

// withWeaveCooldownLock takes an exclusive flock on <dir>/cooldown.lock,
// loads the cooldown store, hands it to fn for mutation, saves it back,
// then releases the lock. A dedicated lock (not queue.lock) so a
// best-effort cooldown write never contends with a queue state transition.
func withWeaveCooldownLock(dir string, fn func(*toolCooldowns) error) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("cooldown lock: ensure dir: %w", err)
	}
	lockPath := filepath.Join(dir, "cooldown.lock")
	lf, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return fmt.Errorf("cooldown lock: open: %w", err)
	}
	defer lf.Close()
	if err := syscall.Flock(int(lf.Fd()), syscall.LOCK_EX); err != nil {
		return fmt.Errorf("cooldown lock: flock: %w", err)
	}
	defer func() { _ = syscall.Flock(int(lf.Fd()), syscall.LOCK_UN) }()

	tc := loadToolCooldowns(dir)
	if err := fn(&tc); err != nil {
		return err
	}
	return saveToolCooldowns(dir, tc)
}
