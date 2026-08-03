//go:build !linux

package multicall

// Non-Linux platforms retain their existing Go runtime boundary. The VSC
// signal-inheritance carrier and its ELF snapshot are Linux-specific; this
// no-op keeps embedded and cross-platform builds unchanged.
func preserveInheritedSignalDispositions() {}
