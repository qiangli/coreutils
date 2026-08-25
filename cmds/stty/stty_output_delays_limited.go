//go:build freebsd || netbsd || openbsd

package sttycmd

// These systems do not expose the historical output-fill and delay masks via
// x/sys/unix. Refuse their setters instead of claiming success without a
// kernel change.
func platformOutputFlag(name string) (uint64, bool)    { return 0, false }
func platformDelay(mode string) (uint64, uint64, bool) { return 0, 0, false }
func platformDelayReport(value uint64) string          { return "" }
