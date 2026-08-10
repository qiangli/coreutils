//go:build linux

package multicall

import (
	"bufio"
	"debug/elf"
	"encoding/binary"
	"io"
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
	// Go leaves an inherited SIG_IGN in place for the terminal-stop signals,
	// but installs its own handler when their inherited action was SIG_DFL.
	// They therefore do not reliably appear in runtime.fwdSig: inspect the
	// live kernel action before replacing only the runtime-owned case.
	for _, sig := range []syscall.Signal{syscall.SIGTSTP, syscall.SIGTTIN, syscall.SIGTTOU} {
		handler, err := linuxSignalHandler(sig)
		if err != nil || handler == 1 { // preserve inherited SIG_IGN
			continue
		}
		_ = setLinuxSignalHandler(sig, 0)
	}
}

// inheritedSIGPIPEWasIgnored reports whether the parent process ignored
// SIGPIPE across execve — the fact preserveInheritedSignalDispositions
// recovers but, until now, discarded before processRunContext built the
// RunContext. When the runtime.fwdSig snapshot is available (static ET_EXEC
// executables — the certification and release builds), the answer comes
// directly from the snapshot. For PIE test binaries where that snapshot is
// unavailable, it queries the live kernel disposition as a fallback: after
// preserveInheritedSignalDispositions has run (or was a no-op on PIE), the
// disposition faithfully reflects the inherited state — SIG_IGN stays
// SIG_IGN, and SIG_DFL was restored to SIG_DFL (ET_EXEC) or replaced by
// the Go runtime's own handler (PIE), neither of which is SIG_IGN.
func inheritedSIGPIPEWasIgnored() bool {
	if originalSignals.ok {
		return originalSignals.handlers[syscall.SIGPIPE] == 1
	}
	handler, err := linuxSignalHandler(syscall.SIGPIPE)
	return err == nil && handler == 1
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
	address, size, ok := lookupSymtabSymbol(f, "runtime.fwdSig")
	if !ok {
		return
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

// lookupSymtabSymbol resolves a single symbol's virtual address and size from
// the executable's .symtab without materializing every elf.Symbol. debug/elf's
// File.Symbols reads the whole table, allocating a Go string and a Symbol
// struct per entry (tens of thousands for this binary) via a reflection-based
// binary.Read — the dominant cost on every standalone-utility startup. Here the
// string table is loaded once and the symbol entries are streamed through a
// bounded buffer with direct field decoding, so lookup allocates a constant
// amount regardless of table size and stops as soon as the name matches.
func lookupSymtabSymbol(f *elf.File, name string) (address, size uint64, ok bool) {
	symtab := f.SectionByType(elf.SHT_SYMTAB)
	if symtab == nil || symtab.Type == elf.SHT_NOBITS {
		return 0, 0, false
	}
	if int(symtab.Link) >= len(f.Sections) {
		return 0, 0, false
	}
	strtab, err := f.Sections[symtab.Link].Data()
	if err != nil {
		return 0, 0, false
	}
	// Elf{64,32}_Sym field offsets: the name index is first in both; Value and
	// Size follow the name on 64-bit and precede Info on 32-bit.
	var entSize, valueOff, sizeOff int
	switch f.Class {
	case elf.ELFCLASS64:
		entSize, valueOff, sizeOff = 24, 8, 16
	case elf.ELFCLASS32:
		entSize, valueOff, sizeOff = 16, 4, 8
	default:
		return 0, 0, false
	}
	r := bufio.NewReaderSize(symtab.Open(), 1<<16)
	buf := make([]byte, entSize)
	target := []byte(name)
	for {
		if _, err := io.ReadFull(r, buf); err != nil {
			return 0, 0, false
		}
		nameOff := f.ByteOrder.Uint32(buf[0:4])
		if nameOff == 0 || !symbolNameMatches(strtab, nameOff, target) {
			continue
		}
		if f.Class == elf.ELFCLASS64 {
			address = f.ByteOrder.Uint64(buf[valueOff : valueOff+8])
			size = f.ByteOrder.Uint64(buf[sizeOff : sizeOff+8])
		} else {
			address = uint64(f.ByteOrder.Uint32(buf[valueOff : valueOff+4]))
			size = uint64(f.ByteOrder.Uint32(buf[sizeOff : sizeOff+4]))
		}
		return address, size, true
	}
}

// symbolNameMatches reports whether the NUL-terminated string at off in strtab
// equals target, comparing bytes in place without allocating a Go string.
func symbolNameMatches(strtab []byte, off uint32, target []byte) bool {
	start := int(off)
	end := start + len(target)
	if start < 0 || end >= len(strtab) {
		return false
	}
	for i := range target {
		if strtab[start+i] != target[i] {
			return false
		}
	}
	return strtab[end] == 0
}
