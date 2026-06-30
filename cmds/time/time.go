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
// (JSON under DHNT_AGENT, prose otherwise) carrying the instruction/context, so
// an agent learns "that took too long; do X next" without the command being
// killed. It is advisory only.
package timecmd

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

	// Resolve + run the program (external, like the GNU binary), searching the
	// invocation's PATH (rc.Env), not the host process's.
	path := lookCommand(rc, command[0])
	if path == "" {
		fmt.Fprintf(rc.Err, "time: %s: command not found\n", command[0])
		return 127
	}
	c := exec.Command(path, command[1:]...)
	c.Dir = rc.Dir
	c.Env = rc.Env
	c.Stdin = rc.In
	c.Stdout = rc.Out
	c.Stderr = rc.Err

	start := time.Now()
	err := c.Start()
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
	status := ps.ExitCode()
	if status < 0 {
		status = 128 // killed by signal; GNU encodes 128+sig, best-effort
	}

	// Report to -o FILE or stderr (GNU writes the report to stderr by default).
	w := rc.Err
	if o.outfile != "" {
		flags := os.O_CREATE | os.O_WRONLY | os.O_TRUNC
		if o.appendF {
			flags = os.O_CREATE | os.O_WRONLY | os.O_APPEND
		}
		if f, ferr := os.OpenFile(rc.Path(o.outfile), flags, 0o644); ferr == nil {
			defer f.Close()
			w = f
		}
	}
	if !o.quiet {
		fmt.Fprint(w, report(o, command, elapsed, userT, sysT, maxRSS, haveRSS, status))
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
func report(o opts, command []string, real, user, sys time.Duration, rss int64, haveRSS bool, status int) string {
	switch {
	case o.format != "":
		return expandFormat(o.format, command, real, user, sys, rss, status) + "\n"
	case o.posix: // -p: POSIX three-line form
		return fmt.Sprintf("real %.2f\nuser %.2f\nsys %.2f\n", real.Seconds(), user.Seconds(), sys.Seconds())
	case o.verbose:
		var b strings.Builder
		fmt.Fprintf(&b, "\tCommand being timed: %q\n", strings.Join(command, " "))
		fmt.Fprintf(&b, "\tUser time (seconds): %.2f\n", user.Seconds())
		fmt.Fprintf(&b, "\tSystem time (seconds): %.2f\n", sys.Seconds())
		fmt.Fprintf(&b, "\tPercent of CPU this job got: %d%%\n", cpuPct(user, sys, real))
		fmt.Fprintf(&b, "\tElapsed (wall clock) time: %s\n", elapsedHMS(real))
		if haveRSS {
			fmt.Fprintf(&b, "\tMaximum resident set size (kbytes): %d\n", rss)
		}
		fmt.Fprintf(&b, "\tExit status: %d\n", status)
		return b.String()
	default: // GNU default one-liner
		s := fmt.Sprintf("%.2fuser %.2fsystem %selapsed %d%%CPU", user.Seconds(), sys.Seconds(), elapsedHMS(real), cpuPct(user, sys, real))
		if haveRSS {
			s += fmt.Sprintf(" (%dmaxresident)k", rss)
		}
		return s + "\n"
	}
}

// expandFormat supports the common GNU -f specifiers; unknown ones pass through.
func expandFormat(f string, command []string, real, user, sys time.Duration, rss int64, status int) string {
	r := strings.NewReplacer(
		"%e", fmt.Sprintf("%.2f", real.Seconds()),
		"%U", fmt.Sprintf("%.2f", user.Seconds()),
		"%S", fmt.Sprintf("%.2f", sys.Seconds()),
		"%P", fmt.Sprintf("%d%%", cpuPct(user, sys, real)),
		"%M", strconv.FormatInt(rss, 10),
		"%x", strconv.Itoa(status),
		"%C", strings.Join(command, " "),
		"\\n", "\n",
		"\\t", "\t",
	)
	return r.Replace(f)
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

// lookCommand resolves a program name against the invocation's PATH (rc.Env).
// A name containing a separator is resolved against the working directory.
func lookCommand(rc *tool.RunContext, name string) string {
	if strings.ContainsAny(name, `/\`) {
		return rc.Path(name)
	}

	pathVal := rc.Getenv("PATH")
	isWindows := runtime.GOOS == "windows"
	var exts []string
	if isWindows {
		pathExt := rc.Getenv("PATHEXT")
		if pathExt == "" {
			exts = []string{".com", ".exe", ".bat", ".cmd"}
		} else {
			for _, e := range filepath.SplitList(pathExt) {
				if e != "" {
					exts = append(exts, strings.ToLower(e))
				}
			}
		}
	}

	hasExt := func(file string) bool {
		if !isWindows {
			return true
		}
		fileLower := strings.ToLower(file)
		for _, ext := range exts {
			if strings.HasSuffix(fileLower, ext) {
				return true
			}
		}
		return false
	}

	checkFile := func(path string) bool {
		fi, err := os.Stat(path)
		if err != nil || fi.IsDir() {
			return false
		}
		if isWindows {
			return true
		}
		return fi.Mode()&0o111 != 0
	}

	for _, dir := range filepath.SplitList(pathVal) {
		if dir == "" {
			continue
		}
		cand := filepath.Join(dir, name)
		if hasExt(name) && checkFile(cand) {
			return cand
		}
		if isWindows {
			for _, ext := range exts {
				candWithExt := cand + ext
				if checkFile(candWithExt) {
					return candWithExt
				}
			}
		}
	}
	return ""
}

// isAgentMode reports whether the invocation env requests agent (JSON) output.
func isAgentMode(rc *tool.RunContext) bool {
	for _, k := range []string{"DHNT_AGENT", "YCODE_AGENT"} {
		switch strings.ToLower(rc.Getenv(k)) {
		case "", "0", "false", "off", "no":
		default:
			return true
		}
	}
	return false
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
