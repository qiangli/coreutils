//go:build !unix

package timecmd

import "os"

// maxRSSKB is unavailable off unix (no rusage); the resident-memory field is
// simply omitted from the report there.
func maxRSSKB(ps *os.ProcessState) (int64, bool) { return 0, false }

// exitStatus mirrors the unix helper without the WaitStatus signal decoding
// (Windows has no POSIX signals); ExitCode is authoritative when non-negative.
func exitStatus(ps *os.ProcessState) int {
	if code := ps.ExitCode(); code >= 0 {
		return code
	}
	return 128
}
