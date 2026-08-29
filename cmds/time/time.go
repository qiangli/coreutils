// Package timecmd implements a pure-Go drop-in for the GNU `time` utility
// (`/usr/bin/time`): run a program, then report the real (wall), user, and
// system time it consumed (plus peak resident memory where the OS exposes it).
//
// Note on the shell `time` keyword: bash parses a bare `time CMD` as a reserved
// word that times a whole pipeline (including builtins). This tool — like the
// real /usr/bin/time it replaces — is the *program* `time`, which the keyword
// shadows; reach it with `command time …` or `\time …`, exactly as you would the
// GNU binary. It runs an external program, not shell builtins.
//
// Agentic twist: `--budget DUR` + `--todo TEXT` turn it into a soft deadline —
// when the program runs longer than the budget, time emits a one-line TODO
// (JSON under BASHY_AGENTIC, prose otherwise) carrying the instruction/context, so
// an agent learns "that took too long; do X next" without the command being
// killed. It is advisory only.
package timecmd

import (
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/qiangli/coreutils/pkg/locale"
	"github.com/qiangli/coreutils/tool"
)

var cmd = &tool.Tool{
	Name:     "time",
	Synopsis: "Run a program and report the real, user, and system time it used (GNU time drop-in).",
	Usage:    "time [-vpqa] [-f FORMAT] [-o FILE] [--budget DUR --todo TEXT] command [arguments...]",
}

func init() { cmd.Run = run; tool.Register(cmd) }

type opts struct {
	verbose bool
	posix   bool   // -p
	quiet   bool   // -q: suppress the resource line (still runs the command)
	appendF bool   // -a (with -o)
	format  string // -f
	outfile string // -o
	budget  string // --budget DUR (agentic)
	todo    string // --todo TEXT (agentic)
}

func run(rc *tool.RunContext, args []string) int {
	var o opts
	// Hand-parse our own options up to the first non-flag (the command), so the
	// command's own flags are never consumed as ours — the wrapper-command rule
	// the GNU binary follows. `--` ends our options explicitly.
	i := 0
	need := func() (string, bool) { // value for an option that takes one
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
		case a == "-v" || a == "--verbose":
			o.verbose = true
		case a == "-p" || a == "--portability":
			o.posix = true
		case a == "-q" || a == "--quiet":
			o.quiet = true
		case a == "-a" || a == "--append":
			o.appendF = true
		case a == "-f" || a == "--format":
			if v, ok := need(); ok {
				o.format = v
			}
		case strings.HasPrefix(a, "--format="):
			o.format = a[len("--format="):]
		case a == "-o" || a == "--output":
			if v, ok := need(); ok {
				o.outfile = v
			}
		case strings.HasPrefix(a, "--output="):
			o.outfile = a[len("--output="):]
		case a == "--budget":
			if v, ok := need(); ok {
				o.budget = v
			}
		case strings.HasPrefix(a, "--budget="):
			o.budget = a[len("--budget="):]
		case a == "--todo":
			if v, ok := need(); ok {
				o.todo = v
			}
		case strings.HasPrefix(a, "--todo="):
			o.todo = a[len("--todo="):]
		default:
			return tool.UsageError(rc, cmd, "unknown option %q", a)
		}
	}
	command := args[i:]
	if len(command) == 0 {
		return tool.UsageError(rc, cmd, "missing command")
	}
	radix := byte('.')
	if !o.quiet {
		var err error
		radix, err = numericRadix(rc.Env)
		if err != nil {
			fmt.Fprintf(rc.Err, "time: %v\n", err)
			return 1
		}
	}

	// -o FILE is opened before the command runs, as the GNU binary does; an
	// unopenable file is fatal, never a silent fallback to stderr.
	w := io.Writer(rc.Err)
	if o.outfile != "" {
		flags := os.O_CREATE | os.O_WRONLY | os.O_TRUNC
		if o.appendF {
			flags = os.O_CREATE | os.O_WRONLY | os.O_APPEND
		}
		f, ferr := os.OpenFile(rc.Path(o.outfile), flags, 0o644)
		if ferr != nil {
			fmt.Fprintf(rc.Err, "time: %s: %v\n", o.outfile, tool.SysErr(ferr))
			return 1
		}
		defer f.Close()
		w = f
	}

	// Resolve + run the program (external, like the GNU binary), searching the
	// invocation's PATH (rc.Env), not the host process's.
	path := rc.ResolveCommand(command[0])
	if path == "" {
		fmt.Fprintf(rc.Err, "time: %s: command not found\n", command[0])
		return 127
	}
	start := time.Now()
	c, err := rc.StartCommand(path, command[1:], rc.In, rc.Out, rc.Err)
	if err != nil {
		// 127 not found / 126 not executable, like a shell.
		fmt.Fprintf(rc.Err, "time: %s: %v\n", command[0], err)
		if os.IsNotExist(err) || strings.Contains(err.Error(), "not found") {
			return 127
		}
		return 126
	}
	_ = c.Wait()
	elapsed := time.Since(start)

	ps := c.ProcessState
	userT, sysT := ps.UserTime(), ps.SystemTime()
	maxRSS, haveRSS := maxRSSKB(ps) // platform helper
	status := exitStatus(ps)        // 128+signum for signal-terminated commands

	// Report to the -o FILE (opened above) or stderr.
	if !o.quiet {
		fmt.Fprint(w, report(o, command, elapsed, userT, sysT, maxRSS, haveRSS, status, radix))
	}

	// Agentic soft-deadline: over budget ⇒ surface the TODO.
	if o.budget != "" {
		if budget, perr := parseDuration(o.budget); perr == nil && elapsed > budget {
			emitTodo(rc, command, elapsed, budget, o.todo)
		}
	}
	return status
}

// report renders the resource line(s) in the selected GNU time format.
func report(o opts, command []string, real, user, sys time.Duration, rss int64, haveRSS bool, status int, radix byte) string {
	switch {
	case o.format != "":
		return expandFormat(o.format, command, real, user, sys, rss, status, radix) + "\n"
	case o.posix: // -p: POSIX three-line form
		return fmt.Sprintf("real %s\nuser %s\nsys %s\n", formatSeconds(real, radix), formatSeconds(user, radix), formatSeconds(sys, radix))
	case o.verbose:
		var b strings.Builder
		fmt.Fprintf(&b, "\tCommand being timed: %q\n", strings.Join(command, " "))
		fmt.Fprintf(&b, "\tUser time (seconds): %s\n", formatSeconds(user, radix))
		fmt.Fprintf(&b, "\tSystem time (seconds): %s\n", formatSeconds(sys, radix))
		fmt.Fprintf(&b, "\tPercent of CPU this job got: %d%%\n", cpuPct(user, sys, real))
		fmt.Fprintf(&b, "\tElapsed (wall clock) time: %s\n", localizeNumber(elapsedHMS(real), radix))
		if haveRSS {
			fmt.Fprintf(&b, "\tMaximum resident set size (kbytes): %d\n", rss)
		}
		fmt.Fprintf(&b, "\tExit status: %d\n", status)
		return b.String()
	default: // GNU default one-liner
		s := fmt.Sprintf("%suser %ssystem %selapsed %d%%CPU", formatSeconds(user, radix), formatSeconds(sys, radix), localizeNumber(elapsedHMS(real), radix), cpuPct(user, sys, real))
		if haveRSS {
			s += fmt.Sprintf(" (%dmaxresident)k", rss)
		}
		return s + "\n"
	}
}

// expandFormat supports the common GNU -f specifiers; unknown ones pass through.
func expandFormat(f string, command []string, real, user, sys time.Duration, rss int64, status int, radix byte) string {
	r := strings.NewReplacer(
		"%e", formatSeconds(real, radix),
		"%U", formatSeconds(user, radix),
		"%S", formatSeconds(sys, radix),
		"%P", fmt.Sprintf("%d%%", cpuPct(user, sys, real)),
		"%M", strconv.FormatInt(rss, 10),
		"%x", strconv.Itoa(status),
		"%C", strings.Join(command, " "),
		"\\n", "\n",
		"\\t", "\t",
	)
	return r.Replace(f)
}

func numericRadix(env []string) (byte, error) {
	name := locale.Resolve(env, locale.Numeric)
	withoutModifier, _, _ := strings.Cut(name, "@")
	base, codeset, _ := strings.Cut(withoutModifier, ".")
	normalizedCodeset := strings.ToUpper(strings.NewReplacer("-", "", "_", "").Replace(codeset))
	switch {
	case (base == "C" || base == "POSIX") && (normalizedCodeset == "" || normalizedCodeset == "UTF8"):
		return '.', nil
	case strings.EqualFold(base, "de_DE"):
		switch normalizedCodeset {
		case "", "UTF8", "ISO88591", "ISO885915":
			return ',', nil
		}
	}
	return 0, fmt.Errorf("LC_NUMERIC %q is unavailable; supported locales are C/POSIX and de_DE", name)
}

func formatSeconds(d time.Duration, radix byte) string {
	return localizeNumber(strconv.FormatFloat(d.Seconds(), 'f', 2, 64), radix)
}

func localizeNumber(s string, radix byte) string {
	if radix == '.' {
		return s
	}
	return strings.Replace(s, ".", string(radix), 1)
}

func cpuPct(user, sys, real time.Duration) int {
	if real <= 0 {
		return 0
	}
	return int((float64(user+sys) / float64(real)) * 100)
}

// elapsedHMS renders wall time as GNU does: [H:]M:SS.ss
func elapsedHMS(d time.Duration) string {
	total := d.Seconds()
	h := int(total) / 3600
	m := (int(total) % 3600) / 60
	s := total - float64(h*3600+m*60)
	if h > 0 {
		return fmt.Sprintf("%d:%02d:%05.2f", h, m, s)
	}
	return fmt.Sprintf("%d:%05.2f", m, s)
}

// emitTodo surfaces the over-budget agentic note (JSON in agent mode).
func emitTodo(rc *tool.RunContext, command []string, elapsed, budget time.Duration, todo string) {
	if todo == "" {
		todo = "this step exceeded its time budget — consider splitting it or changing approach."
	}
	if isAgentMode(rc) {
		fmt.Fprintf(rc.Err,
			`{"schema_version":"bashy-time-v1","kind":"todo","command":%q,"elapsed_ms":%d,"budget_ms":%d,"over":true,"todo":%q}`+"\n",
			strings.Join(command, " "), elapsed.Milliseconds(), budget.Milliseconds(), todo)
		return
	}
	fmt.Fprintf(rc.Err, "time: ⏱ over budget (%s > %s): %s\n", elapsedHMS(elapsed), elapsedHMS(budget), todo)
}

// isAgentMode reports whether the invocation env requests agent (JSON) output.
func isAgentMode(rc *tool.RunContext) bool {
	switch strings.ToLower(rc.Getenv("BASHY_AGENTIC")) {
	case "", "0", "false", "off", "no":
		return false
	}
	return true
}

// parseDuration accepts Go durations ("90s", "5m") and a bare number (seconds).
func parseDuration(s string) (time.Duration, error) {
	if d, err := time.ParseDuration(s); err == nil {
		return d, nil
	}
	if v, err := strconv.ParseFloat(s, 64); err == nil {
		return time.Duration(v * float64(time.Second)), nil
	}
	return 0, strconv.ErrSyntax
}
