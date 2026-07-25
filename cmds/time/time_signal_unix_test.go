//go:build unix

package timecmd

import (
	"os"
	"testing"
)

func TestTimeSignalExitStatus(t *testing.T) {
	// A command terminated by a signal has no ordinary exit code; time must
	// report 128+signum (SIGTERM=15 → 143), matching the shell and GNU/BSD time,
	// not a flat 128.
	_, _, code := runTime(t, os.Environ(), "sh", "-c", "kill -TERM $$")
	if code != 143 {
		t.Errorf("signal exit status = %d, want 143 (128+SIGTERM)", code)
	}
}
