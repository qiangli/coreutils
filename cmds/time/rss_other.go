//go:build !unix

package timecmd

import "os"

// maxRSSKB is unavailable off unix (no rusage); the resident-memory field is
// simply omitted from the report there.
func maxRSSKB(ps *os.ProcessState) (int64, bool) { return 0, false }
