// Package loggercmd implements logger(1): write a message to the system log.
//
// POSIX specifies only `logger string...`; -i, -s, -t and -p are the universal
// historical extensions and are implemented here because the certification
// environment and every real script uses them.
//
// Two things about this command are worth stating up front.
//
// First, the message is the OPERANDS JOINED BY SINGLE SPACES, not the argv
// vector verbatim. `logger a   b` logs "a b": the shell already collapsed the
// run of blanks, and logger must not try to reconstruct it. With no operands
// the standard input is read and EACH LINE becomes its own record — one call,
// many records — which is why the sink is opened once and reused.
//
// Second, the record is delivered through an injectable sink rather than
// straight to log/syslog. That is not only for tests: log/syslog does not exist
// on Windows, and a package that referenced it unconditionally would not
// COMPILE there. The seam is the portability boundary as much as the test seam.
package loggercmd

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/qiangli/coreutils/tool"
)

var cmd = &tool.Tool{
	Name:     "logger",
	Synopsis: "Write messages to the system log.",
	Usage:    "logger [-i] [-s] [-t tag] [-p priority] [string ...]",
}

func init() { cmd.Run = run; tool.Register(cmd) }

// record is one submission to the system log, already fully resolved: the
// priority is encoded, the tag is chosen, and the message is assembled. Tests
// assert on this value, so it is the whole contract between logger's parsing
// and whatever transport carries the record.
type record struct {
	Priority priority
	Tag      string
	Message  string
}

// sink is the system-log transport.
type sink interface {
	Send(record) error
	Close() error
}

// openSink is the seam tests replace. The default is platform-specific:
// log/syslog on unix, a loud refusal on Windows.
var openSink = dialSystemLog

// pid is a seam too, so an asserted -i record is deterministic.
var pid = os.Getpid

func run(rc *tool.RunContext, args []string) int {
	// NOT tool.AliasHelpVersion: that helper rewrites any clustered short
	// option containing an 'h' into --help, and logger's shorthands TAKE
	// VALUES — `logger -tmyhost msg` and `-p daemon.nosuch` both carry an h
	// inside the value and would silently print help and exit 0 instead of
	// logging. tool.Parse registers the -h/-V aliases itself (they are unused
	// here), so nothing is lost by leaving the pre-pass out.
	fs := tool.NewFlags(cmd.Name)
	withPID := fs.BoolP("id", "i", false, "log the process ID with each line")
	toStderr := fs.BoolP("stderr", "s", false, "also write the message to standard error")
	tagFlag := fs.StringP("tag", "t", "", "mark every line with this tag")
	prioFlag := fs.StringP("priority", "p", "", "log at facility.level (default user.notice)")
	operands, code := tool.Parse(rc, cmd, fs, args)
	if code >= 0 {
		return code
	}

	prio := priority(defaultPriority)
	// fs.Changed, not a non-empty value: `-p ""` is a mistyped priority, and
	// treating it as "unset" would silently log at user.notice instead.
	if fs.Changed("priority") {
		p, err := parsePriority(*prioFlag)
		if err != nil {
			return tool.UsageError(rc, cmd, "%v", err)
		}
		prio = p
	}

	tag := *tagFlag
	if tag == "" {
		tag = defaultTag(rc)
	}

	s, err := openSink(rc, prio, tag)
	if err != nil {
		fmt.Fprintf(rc.Err, "logger: %v\n", err)
		return 1
	}
	defer s.Close()

	emit := func(msg string) error {
		if *toStderr {
			// The -s copy is logger's OWN rendering, so it is the one place -i
			// is observable: the unix transport stamps the pid into every
			// record whether or not it was asked for (see sink_unix.go), but
			// nothing forces this copy to.
			fmt.Fprintf(rc.Err, "%s: %s\n", stderrTag(tag, *withPID), msg)
		}
		return s.Send(record{Priority: prio, Tag: tag, Message: msg})
	}

	if len(operands) > 0 {
		if err := emit(strings.Join(operands, " ")); err != nil {
			fmt.Fprintf(rc.Err, "logger: %v\n", err)
			return 1
		}
		return 0
	}

	// No operands: standard input, one message per line. A read error is
	// reported and is a failure, but the records already sent stay sent —
	// there is no way to retract a syslog write and pretending otherwise
	// would be a lie about what the log now contains.
	if rc.In == nil {
		return 0
	}
	status := 0
	sc := bufio.NewScanner(rc.In)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		if err := emit(sc.Text()); err != nil {
			fmt.Fprintf(rc.Err, "logger: %v\n", err)
			return 1
		}
	}
	if err := sc.Err(); err != nil {
		fmt.Fprintf(rc.Err, "logger: %v\n", err)
		status = 1
	}
	return status
}

// stderrTag renders the tag for the -s copy: tag[pid] under -i, bare tag
// otherwise.
func stderrTag(tag string, withPID bool) string {
	if withPID {
		return fmt.Sprintf("%s[%d]", tag, pid())
	}
	return tag
}

// defaultTag is the invoking user's login name, which is what logger has always
// used when -t is absent. The environment is read through the RunContext, never
// os.Getenv: an embedding shell's environment is not the process's.
func defaultTag(rc *tool.RunContext) string {
	for _, key := range []string{"LOGNAME", "USER"} {
		if v := rc.Getenv(key); v != "" {
			return v
		}
	}
	if name := currentUserName(); name != "" {
		return name
	}
	return "logger"
}
