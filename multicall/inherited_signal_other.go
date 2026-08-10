//go:build !linux

package multicall

// Non-Linux platforms retain their existing Go runtime boundary. The VSC
// signal-inheritance carrier and its ELF snapshot are Linux-specific; this
// no-op keeps embedded and cross-platform builds unchanged.
func preserveInheritedSignalDispositions() {}

// inheritedSIGPIPEWasIgnored is always false on non-Linux: the ELF snapshot
// and runtime.fwdSig recovery are Linux-specific, so non-Linux standalone
// binaries cannot detect an inherited SIG_IGN for SIGPIPE. processRunContext
// therefore leaves SIGPIPEIgnored at its zero value (false), preserving the
// existing non-Linux behavior.
func inheritedSIGPIPEWasIgnored() bool { return false }
