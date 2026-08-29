//go:build unix

package edcmd

import (
	"os"
	"os/signal"
	"syscall"
)

type edSignals struct {
	raw       chan os.Signal
	events    chan string
	done      chan struct{}
	resetQuit bool
}

// edSignalSet preserves dispositions which were ignored when ed was entered.
// POSIX utility startup keeps inherited ignores in force; calling
// signal.Notify for one of them would silently turn that ignore into ed's
// command-mode action.  Signals with the default disposition get ed's
// specified behavior: HUP saves and INT returns to command mode. QUIT is
// installed as SIG_IGN separately, rather than caught through Notify.
func edSignalSet(ignored func(os.Signal) bool) []os.Signal {
	set := make([]os.Signal, 0, 2)
	for _, sig := range []os.Signal{syscall.SIGHUP, syscall.SIGINT} {
		if !ignored(sig) {
			set = append(set, sig)
		}
	}
	return set
}

func startEdSignals() *edSignals {
	s := &edSignals{raw: make(chan os.Signal, 4), events: make(chan string, 4), done: make(chan struct{})}
	// POSIX requires ed to ignore SIGQUIT. Installing SIG_IGN at the process
	// boundary also covers the small windows in which Go's Notify dispatcher
	// has not yet received a caught signal.
	s.resetQuit = !signal.Ignored(syscall.SIGQUIT)
	signal.Ignore(syscall.SIGQUIT)
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

func (s *edSignals) stop() {
	signal.Stop(s.raw)
	if s.resetQuit {
		signal.Reset(syscall.SIGQUIT)
	}
	close(s.done)
}
func (s *edSignals) poll() string {
	select {
	case event := <-s.events:
		return event
	default:
		return ""
	}
}
func (s *edSignals) channel() <-chan string { return s.events }
