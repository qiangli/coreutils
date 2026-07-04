// Package timeoutcmd implements a pure-Go drop-in for the GNU `timeout` utility:
// run COMMAND, and if it is still running after DURATION, send it a signal
// (SIGTERM by default). Like the GNU binary it is a command wrapper (runs an
// external program, not shell builtins) and follows GNU's exit-code convention.
//
//	timeout [OPTION]... DURATION COMMAND [ARG]...
//
// DURATION is a number with an optional unit suffix s/m/h/d (default seconds).
// Exit status: 124 if the command timed out; 125 if timeout itself fails; 126 if
// COMMAND is found but cannot be run; 127 if COMMAND is not found; 137 if the
// command was killed by SIGKILL (128+9); otherwise COMMAND's own exit status.
package timeoutcmd

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/qiangli/coreutils/tool"
)

var cmd = &tool.Tool{
	Name:     "timeout",
	Synopsis: "Run a command with a time limit; signal it if it runs too long (GNU timeout drop-in).",
	Usage:    "timeout [-s SIGNAL] [-k DURATION] [--preserve-status] [--foreground] DURATION command [arguments...]",
}

func init() { cmd.Run = run; tool.Register(cmd) }

type opts struct {
	signal         string // -s/--signal (name or number); default TERM
	killAfter      string // -k/--kill-after DURATION
	preserveStatus bool   // --preserve-status
	foreground     bool   // --foreground (do not create a child process group)
	verbose        bool   // -v/--verbose
}

func run(rc *tool.RunContext, args []string) int {
	var o opts
	o.signal = "TERM"
	i := 0
	need := func() (string, bool) {
		if i+1 >= len(args) {
			return "", false
		}
		i++
		return args[i], true
	}
	for i = 0; i < len(args); i++ {
		a := args[i]
		if a == "--" {
			i++
			break
		}
		if a == "" || a[0] != '-' || a == "-" {
			break
		}
		switch {
		case a == "-s" || a == "--signal":
			if v, ok := need(); ok {
				o.signal = v
			}
		case strings.HasPrefix(a, "--signal="):
			o.signal = a[len("--signal="):]
		case strings.HasPrefix(a, "-s"):
			o.signal = a[2:] // -sKILL
		case a == "-k" || a == "--kill-after":
			if v, ok := need(); ok {
				o.killAfter = v
			}
		case strings.HasPrefix(a, "--kill-after="):
			o.killAfter = a[len("--kill-after="):]
		case strings.HasPrefix(a, "-k"):
			o.killAfter = a[2:]
		case a == "--preserve-status":
			o.preserveStatus = true
		case a == "--foreground":
			o.foreground = true
		case a == "-v" || a == "--verbose":
			o.verbose = true
		default:
			return usage(rc, "unknown option %q", a)
		}
	}
	rest := args[i:]
	if len(rest) < 1 {
		return usage(rc, "missing DURATION")
	}
	dur, err := parseDuration(rest[0])
	if err != nil || dur < 0 {
		return usage(rc, "invalid time interval %q", rest[0])
	}
	command := rest[1:]
	if len(command) == 0 {
		return usage(rc, "missing command")
	}

	sig := signalByName(o.signal)
	if sig == nil {
		return usage(rc, "%s: invalid signal", o.signal)
	}
	var killAfter time.Duration
	if o.killAfter != "" {
		if killAfter, err = parseDuration(o.killAfter); err != nil {
			return usage(rc, "invalid time interval %q", o.killAfter)
		}
	}

	path := lookCommand(rc, command[0])
	if path == "" {
		fmt.Fprintf(rc.Err, "timeout: failed to run command %q: No such file or directory\n", command[0])
		return 127
	}
	c := exec.Command(path, command[1:]...)
	c.Dir = rc.Dir
	c.Env = rc.Env
	c.Stdin = rc.In
	c.Stdout = rc.Out
	c.Stderr = rc.Err
	if !o.foreground {
		setProcGroup(c) // unix: own process group so the whole child tree is signalled
	}

	if err := c.Start(); err != nil {
		fmt.Fprintf(rc.Err, "timeout: failed to run command %q: %v\n", command[0], err)
		if os.IsNotExist(err) {
			return 127
		}
		return 126
	}

	done := make(chan error, 1)
	go func() { done <- c.Wait() }()

	timedOut := false
	primary := time.NewTimer(dur)
	defer primary.Stop()
	for {
		select {
		case werr := <-done:
			return exitStatus(werr, timedOut, o.preserveStatus)
		case <-primary.C:
			timedOut = true
			if o.verbose {
				fmt.Fprintf(rc.Err, "timeout: sending signal %s to command %q\n", o.signal, command[0])
			}
			signalCmd(c, sig, o.foreground)
			if killAfter > 0 {
				kt := time.NewTimer(killAfter)
				select {
				case werr := <-done:
					kt.Stop()
					return exitStatus(werr, timedOut, o.preserveStatus)
				case <-kt.C:
					if o.verbose {
						fmt.Fprintf(rc.Err, "timeout: sending signal KILL to command %q\n", command[0])
					}
					signalCmd(c, killSignal(), o.foreground)
					<-done
					return 137
				}
			}
			// No kill-after: wait for the command to die from the signal.
			<-done
			return exitStatus(nil, true, o.preserveStatus)
		}
	}
}

// exitStatus maps the command's wait result to GNU timeout's exit convention.
func exitStatus(werr error, timedOut, preserve bool) int {
	if timedOut && !preserve {
		return 124
	}
	if werr == nil {
		return 0
	}
	if ee, ok := werr.(*exec.ExitError); ok {
		code := ee.ExitCode()
		if code < 0 {
			return 137 // killed by signal (best-effort; SIGKILL = 128+9)
		}
		return code
	}
	return 125
}

func usage(rc *tool.RunContext, format string, a ...any) int {
	return tool.UsageError(rc, cmd, format, a...)
}

// parseDuration accepts GNU forms: a bare number (seconds) or NUMBER[smhd].
func parseDuration(s string) (time.Duration, error) {
	if s == "" {
		return 0, strconv.ErrSyntax
	}
	mult := time.Second
	switch s[len(s)-1] {
	case 's':
		s = s[:len(s)-1]
	case 'm':
		mult, s = time.Minute, s[:len(s)-1]
	case 'h':
		mult, s = time.Hour, s[:len(s)-1]
	case 'd':
		mult, s = 24*time.Hour, s[:len(s)-1]
	}
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0, err
	}
	return time.Duration(v * float64(mult)), nil
}

// lookCommand resolves a program name against the invocation's PATH (rc.Env); a
// name with a path separator is resolved against the working directory.
func lookCommand(rc *tool.RunContext, name string) string {
	if strings.ContainsAny(name, `/\`) {
		p := rc.Path(name)
		if isExecFile(p) {
			return p
		}
		return ""
	}
	for _, dir := range filepath.SplitList(rc.Getenv("PATH")) {
		if dir == "" {
			continue
		}
		cand := filepath.Join(dir, name)
		if isExecFile(cand) {
			return cand
		}
		if runtime.GOOS == "windows" {
			for _, ext := range []string{".exe", ".bat", ".cmd", ".com"} {
				if isExecFile(cand + ext) {
					return cand + ext
				}
			}
		}
	}
	return ""
}

func isExecFile(path string) bool {
	fi, err := os.Stat(path)
	if err != nil || fi.IsDir() {
		return false
	}
	if runtime.GOOS == "windows" {
		return true
	}
	return fi.Mode()&0o111 != 0
}
