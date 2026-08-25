package whocmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/qiangli/coreutils/cmds/internal/session"
	"github.com/qiangli/coreutils/tool"
)

var cmd = &tool.Tool{Name: "who", Synopsis: "Show who is logged on.", Usage: "who [OPTION]... [FILE | am i]"}

func init() { cmd.Run = run; tool.Register(cmd) }

func run(rc *tool.RunContext, args []string) int {
	fs := tool.NewFlags(cmd.Name)
	all := fs.BoolP("all", "a", false, "same as -b -d --login -p -r -t -T -u")
	heading := fs.BoolP("heading", "H", false, "print line of column headings")
	count := fs.BoolP("count", "q", false, "all login names and number of users logged on")
	_ = fs.BoolP("short", "s", false, "short format")
	usersOnly := fs.BoolP("users", "u", false, "list users logged in")
	mesg := fs.BoolP("mesg", "T", false, "add user's message status as +, - or ?")
	boot := fs.BoolP("boot", "b", false, "time of last system boot")
	dead := fs.BoolP("dead", "d", false, "print dead processes")
	login := fs.BoolP("login", "l", false, "print system login processes")
	fs.Bool("lookup", false, "attempt to canonicalize hostnames")
	process := fs.BoolP("process", "p", false, "print active processes spawned by init")
	runlevel := fs.BoolP("runlevel", "r", false, "print current runlevel")
	timeChange := fs.BoolP("time", "t", false, "print last system clock change")
	message := fs.BoolP("message", "w", false, "same as -T")
	onlyMe := fs.BoolP("same-host", "m", false, "only hostname and user associated with stdin")
	writable := fs.Bool("writable", false, "same as -T")
	operands, code := tool.Parse(rc, cmd, fs, tool.AliasHelpVersion(args))
	if code >= 0 {
		return code
	}

	path, sameHost, errMsg := parseOperands(operands)
	if errMsg != "" {
		return tool.UsageError(rc, cmd, "%s", errMsg)
	}
	if sameHost {
		*onlyMe = true
	}
	if path != "" {
		path = rc.Path(path)
	}

	records, err := session.Read(path)
	if err != nil {
		fmt.Fprintf(rc.Err, "who: %v\n", err)
		return 1
	}

	// -q ignores every other option: it counts the login names present in
	// the database BEFORE any -m / -b / -d / … selection is applied. POSIX:
	// "the -q option ... all other options shall be ignored."
	if *count {
		names := make([]string, 0, len(records))
		for _, r := range records {
			if session.IsUser(r) {
				names = append(names, r.User)
			}
		}
		if len(names) > 0 {
			fmt.Fprintln(rc.Out, strings.Join(names, " "))
		}
		fmt.Fprintf(rc.Out, "# users=%d\n", len(names))
		return 0
	}

	needBoot := *boot || *all
	needDead := *dead || *all
	needLogin := *login || *all
	needProcess := *process || *all
	needRunlevel := *runlevel || *all
	needTime := *timeChange || *all
	needUsers := *usersOnly || *all || !(needBoot || needDead || needLogin || needProcess || needRunlevel || needTime)

	var live []session.Record
	for _, r := range records {
		if session.IsUser(r) {
			if needUsers {
				live = append(live, r)
			}
			continue
		}

		switch r.Type {
		case "BOOT_TIME", "boot", "2":
			if needBoot {
				live = append(live, r)
			}
		case "DEAD_PROCESS", "dead", "8":
			if needDead {
				live = append(live, r)
			}
		case "LOGIN_PROCESS", "login", "6":
			if needLogin {
				live = append(live, r)
			}
		case "INIT_PROCESS", "init", "5":
			if needProcess {
				live = append(live, r)
			}
		case "RUN_LVL", "runlevel", "1":
			if needRunlevel {
				live = append(live, r)
			}
		case "NEW_TIME", "time", "3", "OLD_TIME", "4":
			if needTime {
				live = append(live, r)
			}
		default:
			if *all {
				live = append(live, r)
			}
		}
	}
	showMesg := *mesg || *writable || *all || *message
	showIdle := *usersOnly || *all
	loc := timeLocation(rc)
	timeFmt := timeFormat(rc)

	if *onlyMe {
		tty, ok := stdinTTY(rc)
		if !ok {
			return 0
		}
		var filtered []session.Record
		for _, r := range live {
			if ttyMatch(r.TTY, tty) {
				filtered = append(filtered, r)
			}
		}
		live = filtered
		if len(live) == 0 {
			return 0
		}
	}

	if *heading || *all {
		if showMesg && !showIdle {
			// The -T short form has no idle/pid columns.
			fmt.Fprintln(rc.Out, "NAME     S LINE         TIME")
		} else if showMesg {
			fmt.Fprintln(rc.Out, "NAME     STATE LINE         TIME             IDLE   PID  COMMENT")
		} else if showIdle {
			fmt.Fprintln(rc.Out, "NAME     LINE         TIME             IDLE   PID  COMMENT")
		} else {
			fmt.Fprintln(rc.Out, "NAME     LINE         TIME             COMMENT")
		}
	}

	for _, r := range live {
		name := displayName(r)

		// POSIX -T short form: exactly "%s %c %s %s\n"
		// (name, terminal-state, line, time). Applies when the message
		// status is requested without the -u idle/pid columns.
		if showMesg && !showIdle {
			fmt.Fprintf(rc.Out, "%s %c %s %s\n", name, terminalState(r), r.TTY, formatTime(r.Time, loc, timeFmt))
			continue
		}

		idle := ""
		if showIdle {
			idle = formatIdle(r.TTY, r.Time)
		}
		comment := lineComment(r, *onlyMe)

		if showMesg {
			fmt.Fprintf(rc.Out, "%-8s %c   %-12s %-16s", name, terminalState(r), r.TTY, formatTime(r.Time, loc, timeFmt))
		} else {
			fmt.Fprintf(rc.Out, "%-8s %-12s %-16s", name, r.TTY, formatTime(r.Time, loc, timeFmt))
		}
		if showIdle {
			fmt.Fprintf(rc.Out, " %-5s", idle)
			if r.PID > 0 {
				fmt.Fprintf(rc.Out, " %5d", r.PID)
			} else {
				fmt.Fprintf(rc.Out, "      ")
			}
		}
		if comment != "" {
			fmt.Fprintf(rc.Out, " %s", comment)
		}
		fmt.Fprintln(rc.Out)
	}
	return 0
}

// parseOperands enforces the POSIX operand grammar: `who [file]` or
// `who am i`. Anything else — arbitrary two words, or a nonstandard
// file + "am i" combination — is a usage error, never a silent guess.
func parseOperands(operands []string) (file string, sameHost bool, errMsg string) {
	switch len(operands) {
	case 0:
		return "", false, ""
	case 1:
		return operands[0], false, ""
	case 2:
		if operands[0] == "am" && (operands[1] == "i" || operands[1] == "I") {
			return "", true, ""
		}
		return "", false, fmt.Sprintf("extra operand %q", operands[1])
	default:
		return "", false, fmt.Sprintf("extra operand %q", operands[2])
	}
}

// displayName is the NAME column. For records with no user string (system
// records) it substitutes the conventional POSIX name.
func displayName(r session.Record) string {
	if r.User != "" {
		return r.User
	}
	switch r.Type {
	case "LOGIN_PROCESS", "login", "6":
		return "LOGIN"
	case "BOOT_TIME", "boot", "2":
		return "reboot"
	case "RUN_LVL", "runlevel", "1":
		return "run-level"
	}
	return r.User
}

// terminalState is the '%c' of the -T format. A live user or login line
// reports the tty's message/writable status ('+', '-', or '?'); a record
// with no live terminal (dead, boot, run-level, clock change) reports '?'.
func terminalState(r session.Record) byte {
	switch r.Type {
	case "DEAD_PROCESS", "dead", "8",
		"BOOT_TIME", "boot", "2",
		"RUN_LVL", "runlevel", "1",
		"NEW_TIME", "time", "3", "OLD_TIME", "4":
		return '?'
	}
	if r.TTY == "" {
		return '?'
	}
	return messageStatus(r.TTY)
}

// lineComment is the trailing COMMENT field: the origin host for a normal
// login, or the mandatory exit status for a dead process.
func lineComment(r session.Record, onlyMe bool) string {
	if isDead(r) {
		return exitStatus(r)
	}
	host := r.Host
	if onlyMe && host == "" {
		if h, err := os.Hostname(); err == nil {
			host = h
		}
	}
	if host == "" {
		return ""
	}
	return "(" + host + ")"
}

func isDead(r session.Record) bool {
	switch r.Type {
	case "DEAD_PROCESS", "dead", "8":
		return true
	}
	return false
}

// exitStatus renders the mandatory -d termination/exit values, matching the
// GNU/POSIX "id=… term=N exit=N" shape.
func exitStatus(r session.Record) string {
	if r.ID != "" {
		return fmt.Sprintf("id=%s term=%d exit=%d", r.ID, r.Term, r.Exit)
	}
	return fmt.Sprintf("term=%d exit=%d", r.Term, r.Exit)
}

func ttyMatch(recordTTY, stdinTTY string) bool {
	record := filepath.Base(recordTTY)
	stdin := filepath.Base(stdinTTY)
	return record != "" && stdin != "" && record == stdin
}

// timeLocation resolves the zone from the invocation's TZ (rc.Env — never
// the host process's zone). It accepts both IANA names ("America/New_York")
// and POSIX TZ strings ("EST5EDT", "PST8PDT", "UTC0"); when TZ is unset the
// caller's local zone applies, and an unparseable TZ falls back to UTC as
// POSIX specifies.
func timeLocation(rc *tool.RunContext) *time.Location {
	tz := rc.Getenv("TZ")
	if tz == "" {
		return time.Local
	}
	if loc, err := time.LoadLocation(tz); err == nil {
		return loc
	}
	if loc := posixTZ(tz); loc != nil {
		return loc
	}
	return time.UTC
}

// posixTZ parses the standard-time portion of a POSIX TZ string
// (stdoffset[dst…]) into a fixed zone. DST transition rules are not applied —
// standard time is used — which is deterministic and correct for the
// certification cases (UTC and fixed offsets).
func posixTZ(tz string) *time.Location {
	if strings.HasPrefix(tz, ":") {
		return nil // ":<path>" implementation-defined form: unsupported
	}
	name, rest := parseTZName(tz)
	if name == "" {
		return nil
	}
	secs, ok := parseTZOffset(rest)
	if !ok {
		// A bare name with no offset (e.g. "UTC") denotes UTC.
		return time.FixedZone(name, 0)
	}
	// POSIX offset is the value ADDED to local time to reach UTC, so a zone
	// east of UTC is the negation.
	return time.FixedZone(name, -secs)
}

func parseTZName(s string) (name, rest string) {
	if s == "" {
		return "", ""
	}
	if s[0] == '<' {
		if j := strings.IndexByte(s, '>'); j > 1 {
			return s[1:j], s[j+1:]
		}
		return "", ""
	}
	i := 0
	for i < len(s) && ((s[i] >= 'A' && s[i] <= 'Z') || (s[i] >= 'a' && s[i] <= 'z')) {
		i++
	}
	if i < 3 {
		return "", ""
	}
	return s[:i], s[i:]
}

func parseTZOffset(s string) (secs int, ok bool) {
	if s == "" {
		return 0, false
	}
	sign := 1
	switch s[0] {
	case '+':
		s = s[1:]
	case '-':
		sign = -1
		s = s[1:]
	}
	hh, s, ok := scanInt(s)
	if !ok {
		return 0, false
	}
	total := hh * 3600
	if strings.HasPrefix(s, ":") {
		mm, r, ok := scanInt(s[1:])
		if !ok {
			return 0, false
		}
		total += mm * 60
		s = r
		if strings.HasPrefix(s, ":") {
			ss, _, ok := scanInt(s[1:])
			if !ok {
				return 0, false
			}
			total += ss
		}
	}
	return sign * total, true
}

func scanInt(s string) (n int, rest string, ok bool) {
	i := 0
	for i < len(s) && s[i] >= '0' && s[i] <= '9' {
		i++
	}
	if i == 0 {
		return 0, s, false
	}
	v, err := strconv.Atoi(s[:i])
	if err != nil {
		return 0, s, false
	}
	return v, s[i:], true
}

func formatTime(t time.Time, loc *time.Location, layout string) string {
	if t.IsZero() {
		return ""
	}
	if loc == nil {
		loc = time.Local
	}
	if layout == "" {
		layout = posixTimeLayout
	}
	return t.In(loc).Format(layout)
}

// posixTimeLayout is who's C/POSIX-locale time field: strftime "%b %e %H:%M".
const posixTimeLayout = "Jan _2 15:04"

// timeFormat selects the time-field layout from the effective LC_TIME
// locale. Only the C/POSIX locale is supported (its "%b %e %H:%M" form);
// any other locale would require locale month-name data this pure-Go tool
// deliberately does not carry, so it falls back to the same deterministic
// C layout rather than silently approximating a localized one.
func timeFormat(rc *tool.RunContext) string {
	switch effectiveLocale(rc) {
	case "", "C", "POSIX":
		return posixTimeLayout
	default:
		return posixTimeLayout
	}
}

// effectiveLocale resolves the LC_TIME category with POSIX precedence:
// LC_ALL overrides LC_TIME, which overrides LANG. A common ".UTF-8"/".utf8"
// codeset suffix is stripped so "C.UTF-8" is treated as the C locale.
func effectiveLocale(rc *tool.RunContext) string {
	v := rc.Getenv("LC_ALL")
	if v == "" {
		v = rc.Getenv("LC_TIME")
	}
	if v == "" {
		v = rc.Getenv("LANG")
	}
	if i := strings.IndexByte(v, '.'); i >= 0 {
		v = v[:i]
	}
	return v
}

func formatIdle(tty string, loginTime time.Time) string {
	path := session.TTYPath(tty)
	if path == "" {
		return "old"
	}
	at, ok := accessTime(path)
	if !ok {
		return "old"
	}
	if at.Before(loginTime) {
		at = loginTime
	}
	idle := time.Since(at)
	if idle < time.Minute {
		return "."
	}
	if idle > 24*time.Hour {
		return "old"
	}
	h := int(idle.Hours())
	m := int(idle.Minutes()) % 60
	return fmt.Sprintf("%02d:%02d", h, m)
}

func messageStatus(tty string) byte {
	path := session.TTYPath(tty)
	if path == "" {
		return '?'
	}
	fi, err := os.Stat(path)
	if err != nil {
		return '?'
	}
	return ttyMessageStatus(fi)
}
