//go:build unix

package loggercmd

import (
	"fmt"
	"log/syslog"

	"github.com/qiangli/coreutils/tool"
)

// dialSystemLog connects to the local syslog daemon.
//
// log/syslog is frozen and marked deprecated in the standard library, but the
// deprecation is about the API's design, not about the transport: the local
// AF_UNIX datagram to /dev/log (Linux) or /var/run/syslog (macOS) IS how a
// POSIX utility hands a record to the system log, and reimplementing that
// framing here would add a wire format to maintain for no behavioural gain.
//
// KNOWN FIDELITY GAP: log/syslog's writer stamps "tag[pid]" into every record
// unconditionally, so -i cannot be honoured on the syslog side — the pid is
// always present, whether or not it was requested. The flag is therefore
// observable only on the -s copy, which this package formats itself. This is
// under- rather than over-reporting (the record carries more than was asked
// for, never less), so it is documented rather than made an error; a caller who
// needs the pid suppressed needs a transport this package does not implement.
func dialSystemLog(rc *tool.RunContext, prio priority, tag string) (sink, error) {
	w, err := syslog.Dial("", "", syslog.Priority(prio), tag)
	if err != nil {
		return nil, fmt.Errorf("cannot connect to the system log: %w", err)
	}
	return &syslogSink{w: w, dialed: prio, tag: tag}, nil
}

type syslogSink struct {
	w *syslog.Writer
	// dialed is the priority the writer was opened with; a record at that
	// priority goes through the plain Write path, anything else through the
	// per-severity helpers, so the wire priority always matches the record.
	dialed priority
	tag    string
}

func (s *syslogSink) Send(r record) error {
	if r.Priority == s.dialed {
		_, err := s.w.Write([]byte(r.Message))
		return err
	}
	// A record whose priority differs from the dial-time default (nothing in
	// this command produces one today, but the sink contract allows it) is
	// routed by severity. The facility cannot be changed after Dial, which is
	// exactly why -p is resolved BEFORE the sink is opened.
	switch r.Priority.severity() {
	case 0:
		return s.w.Emerg(r.Message)
	case 1:
		return s.w.Alert(r.Message)
	case 2:
		return s.w.Crit(r.Message)
	case 3:
		return s.w.Err(r.Message)
	case 4:
		return s.w.Warning(r.Message)
	case 5:
		return s.w.Notice(r.Message)
	case 6:
		return s.w.Info(r.Message)
	default:
		return s.w.Debug(r.Message)
	}
}

func (s *syslogSink) Close() error { return s.w.Close() }
