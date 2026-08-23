//go:build unix

package nohupcmd

import (
	"os"
	"os/signal"
	"syscall"
)

// ignoreHangup makes the nohup invocation itself immune to SIGHUP while it
// waits for the requested utility.
//
// GNU nohup exec()s the utility, so the pid the caller launched *becomes*
// the utility and no wrapper is left behind. This implementation spawns a
// shell and waits, so the invocation keeps a pid of its own. Without this,
// a hangup kills that wrapper while the utility carries on -- and a caller
// watching the pid it launched observes the nohup invocation being killed
// by SIGHUP, which is precisely what nohup promises will not happen.
//
// Notify rather than signal.Ignore: this runs inside a multicall binary that
// may be an interactive shell, and Ignore would have to be undone with
// Reset, clobbering any SIGHUP handling the embedding process installed.
// Notify suppresses the default terminate action for the duration and Stop
// restores the prior disposition without disturbing other handlers.
func ignoreHangup() func() {
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, syscall.SIGHUP)
	return func() { signal.Stop(ch) }
}
