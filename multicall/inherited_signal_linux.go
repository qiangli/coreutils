//go:build linux

package multicall

import (
	"debug/elf"
	"encoding/binary"
	"os"
	"os/signal"
	"strconv"
	"sync"
	"syscall"
)

const linuxNSIG = 65

var originalSignals struct {
	sync.Once
	handlers [linuxNSIG]uintptr
	ok       bool
}

// preserveInheritedSignalDispositions repairs the observable execve boundary
// before a standalone utility runs. The Go runtime snapshots every inherited
// disposition in runtime.fwdSig, then replaces many SIG_IGN/SIG_DFL actions
// with its own handler. That is appropriate for a long-lived Go application,
// but not for a multicall utility: POSIX requires ignored signals to remain
// ignored and default core-producing signals to retain their wait status.
//
// Recent Go linkers intentionally reject go:linkname access to runtime.fwdSig.
// Linux Go executables retain the ordinary ELF data symbol, so read its address
// from our own executable and copy the live snapshot through /proc/self/mem
// before changing dispositions.
// This is confined to Main; embedded Tool.Run/Dispatch callers never invoke it.
func preserveInheritedSignalDispositions() {
	originalSignals.Do(loadOriginalSignals)
	if !originalSignals.ok {
		return
	}
	for _, sig := range []syscall.Signal{
		syscall.SIGABRT, syscall.SIGALRM, syscall.SIGBUS, syscall.SIGFPE,
		syscall.SIGILL, syscall.SIGPIPE, syscall.SIGQUIT, syscall.SIGSEGV,
		syscall.SIGUSR1, syscall.SIGUSR2, syscall.SIGTERM,
	} {
		switch originalSignals.handlers[sig] {
		case 1: // SIG_IGN
			// Use os/signal, not only sigaction: Go's stdout/stderr EPIPE
			// path consults the runtime's internal ignored bit before it
			// deliberately raises SIGPIPE.
			signal.Ignore(sig)
		case 0: // SIG_DFL
			// Preserve the mask inherited across exec; only the action was
			// replaced by runtime startup.
			_ = setLinuxSignalHandler(sig, 0)
		}
	}
}

func loadOriginalSignals() {
	path, err := os.Executable()
	if err != nil {
		return
	}
	f, err := elf.Open(path)
	if err != nil {
		return
	}
	defer f.Close()
	// sym.Value is an absolute virtual address only for the non-PIE static
	// executable used by the certification and release builders.
	if f.Type != elf.ET_EXEC {
		return
	}
	syms, err := f.Symbols()
	if err != nil {
		return
	}
	var address, size uint64
	for _, sym := range syms {
		if sym.Name == "runtime.fwdSig" {
			address, size = sym.Value, sym.Size
			break
		}
	}
	wordSize := strconv.IntSize / 8
	need := uint64(linuxNSIG * wordSize)
	if address == 0 || size < need {
		return
	}
	mem, err := os.Open("/proc/self/mem")
	if err != nil {
		return
	}
	defer mem.Close()
	snapshot := make([]byte, need)
	if n, err := mem.ReadAt(snapshot, int64(address)); err != nil || n != len(snapshot) {
		return
	}
	for i := range originalSignals.handlers {
		word := snapshot[i*wordSize : (i+1)*wordSize]
		if wordSize == 8 {
			originalSignals.handlers[i] = uintptr(binary.NativeEndian.Uint64(word))
		} else {
			originalSignals.handlers[i] = uintptr(binary.NativeEndian.Uint32(word))
		}
	}
	originalSignals.ok = true
}
