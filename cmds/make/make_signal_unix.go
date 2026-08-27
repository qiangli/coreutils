//go:build unix

package makecmd

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/qiangli/coreutils/tool"
)

// installMakeSignalContext confines process-global signal interception to the
// standalone multicall process. Embedded shells must never change the host's
// signal dispositions. The multicall boundary re-raises rc.ExitSignal after
// run returns, preserving the original signal wait status.
func installMakeSignalContext(rc *tool.RunContext) func() {
	if !rc.DedicatedProcess {
		return func() {}
	}
	parent := rc.Ctx
	if parent == nil {
		parent = context.Background()
	}
	ctx, cancel := context.WithCancel(parent)
	rc.Ctx = ctx
	ch := make(chan os.Signal, 1)
	done := make(chan struct{})
	exitSignal := 0
	signal.Notify(ch, syscall.SIGHUP, syscall.SIGTERM, syscall.SIGINT, syscall.SIGQUIT)
	go func() {
		defer close(done)
		select {
		case sig := <-ch:
			if value, ok := sig.(syscall.Signal); ok {
				exitSignal = int(value)
			}
			cancel()
		case <-ctx.Done():
		}
	}()
	return func() {
		signal.Stop(ch)
		cancel()
		<-done
		if exitSignal != 0 {
			rc.ExitSignal = exitSignal
		}
	}
}
