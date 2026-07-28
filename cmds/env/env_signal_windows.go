//go:build windows

package envcmd

import (
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"

	"github.com/qiangli/coreutils/tool"
)

var commandSignalMu sync.Mutex

func commandSignals(specs []string) ([]os.Signal, error) {
	var out []os.Signal
	for _, spec := range specs {
		for _, part := range strings.Split(spec, ",") {
			part = strings.TrimSpace(part)
			if part == "" {
				continue
			}
			name := strings.TrimPrefix(strings.ToUpper(part), "SIG")
			if name == "ALL" || name == "INT" {
				if len(out) == 0 {
					out = append(out, os.Interrupt)
				}
				continue
			}
			if n, err := strconv.Atoi(part); err == nil && n == int(syscall.SIGINT) {
				if len(out) == 0 {
					out = append(out, os.Interrupt)
				}
				continue
			}
			return nil, fmt.Errorf("unknown signal %q", part)
		}
	}
	return out, nil
}

func ignoreForCommandStart(signals []os.Signal) func() {
	if len(signals) == 0 {
		return func() {}
	}
	commandSignalMu.Lock()
	wasIgnored := signal.Ignored(os.Interrupt)
	signal.Ignore(os.Interrupt)
	return func() {
		if !wasIgnored {
			ch := make(chan os.Signal, 1)
			signal.Notify(ch, os.Interrupt)
			signal.Reset(os.Interrupt)
		}
		commandSignalMu.Unlock()
	}
}

func commandSignalStatus(_ *os.ProcessState) int { return 125 }

func defaultCommandPath() string {
	return os.Getenv("PATH")
}

func resolveCommandCandidate(rc *tool.RunContext, candidate string) string {
	if filepath.Ext(candidate) != "" {
		return candidate
	}
	return rc.ResolveExecutable(candidate)
}
