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

func startEdSignals() *edSignals {
	s := &edSignals{raw: make(chan os.Signal, 4), events: make(chan string, 4), done: make(chan struct{})}
	signal.Notify(s.raw, syscall.SIGHUP, syscall.SIGINT, syscall.SIGQUIT)
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
