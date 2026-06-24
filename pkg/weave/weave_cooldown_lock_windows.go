//go:build windows

package weave

// withWeaveCooldownLock on Windows is best-effort (no OS-level mutex),
// mirroring withWeaveQueueLock. Cooldown writes are themselves best-effort,
// so a lost concurrent update only loses a cooldown record, never run state.
func withWeaveCooldownLock(dir string, fn func(*toolCooldowns) error) error {
	tc := loadToolCooldowns(dir)
	if err := fn(&tc); err != nil {
		return err
	}
	return saveToolCooldowns(dir, tc)
}
