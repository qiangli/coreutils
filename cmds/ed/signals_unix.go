//go:build unix

package edcmd

import (
	"os"
	"os/signal"
	"syscall"
)

type edSignals struct {
	raw    chan os.Signal
	events chan string
	done   chan struct{}
}

// edSignalSet preserves dispositions which were ignored when ed was entered.
// POSIX utility startup keeps inherited ignores in force; calling
// signal.Notify for one of them would silently turn that ignore into ed's
// command-mode action.  Signals with the default disposition get ed's
// specified behavior: HUP saves, INT returns to command mode, and QUIT is
// ignored for the duration of the invocation.
func edSignalSet(ignored func(os.Signal) bool) []os.Signal {
	set := make([]os.Signal, 0, 3)
	for _, sig := range []os.Signal{syscall.SIGHUP, syscall.SIGINT, syscall.SIGQUIT} {
		if !ignored(sig) {
			set = append(set, sig)
		}
	}
	return set
}

func startEdSignals() *edSignals {
	s := &edSignals{raw: make(chan os.Signal, 4), events: make(chan string, 4), done: make(chan struct{})}
	if set := edSignalSet(signal.Ignored); len(set) > 0 {
		signal.Notify(s.raw, set...)
	}
	go func() {
		for {
			select {
			case sig := <-s.raw:
				var event string
				switch sig {
				case syscall.SIGHUP:
					event = "hangup"
				case syscall.SIGINT:
					event = "interrupt"
				case syscall.SIGQUIT:
					continue
				}
				select {
				case s.events <- event:
				default:
				}
			case <-s.done:
				return
			}
		}
	}()
	return s
}

func (s *edSignals) stop() { signal.Stop(s.raw); close(s.done) }
func (s *edSignals) poll() string {
	select {
	case event := <-s.events:
		return event
	default:
		return ""
	}
}
func (s *edSignals) channel() <-chan string { return s.events }
