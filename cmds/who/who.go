package whocmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/qiangli/coreutils/cmds/internal/session"
	"github.com/qiangli/coreutils/cmds/internal/tzenv"
	posixlocale "github.com/qiangli/coreutils/pkg/locale"
	"github.com/qiangli/coreutils/tool"
)

var cmd = &tool.Tool{Name: "who", Synopsis: "Show who is logged on.", Usage: "who [OPTION]... [FILE | am i]"}

func init() { cmd.Run = run; tool.Register(cmd) }

func run(rc *tool.RunContext, args []string) int {
	fs := tool.NewFlags(cmd.Name)
	all := fs.BoolP("all", "a", false, "same as -b -d --login -p -r -t -T -u")
	heading := fs.BoolP("heading", "H", false, "print line of column headings")
	count := fs.BoolP("count", "q", false, "all login names and number of users logged on")
	short := fs.BoolP("short", "s", false, "short format")
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
		fmt.Fprintln(rc.Out, strings.Join(names, " "))
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
		case "NEW_TIME", "time", "3":
			if needTime {
				live = append(live, r)
			}
		case "OLD_TIME", "4":
			// -t reports the completed clock change (NEW_TIME), not both
			// halves of the old/new transition pair.
		}
	}
	showMesg := *mesg || *writable || *all || *message
	shortMode := *short && !*all
	showIdle := (*usersOnly || *all) && !shortMode
	loc := tzenv.Location(rc.Env)
	timeFmt, err := posixlocale.ResolveTime(rc.Env)
	if err != nil {
		fmt.Fprintf(rc.Err, "who: %v\n", err)
		return 1
	}

	for _, r := range live {
		if isDead(r) && !r.ExitKnown {
			fmt.Fprintln(rc.Err, "who: dead-process exit status is unavailable in this platform's session database")
			return 1
		}
	}

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

	haveStateField := false
	for _, r := range live {
		if stateFieldExists(r) {
			haveStateField = true
			break
		}
	}
	if *heading {
		if showMesg && !showIdle && !haveStateField {
			fmt.Fprintln(rc.Out, "NAME     LINE         TIME")
		} else if showMesg && !showIdle {
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
		if isRunLevel(r) {
			writeRunLevel(rc, r, loc, timeFmt)
			continue
		}
		name := displayName(r)
		tty := displayTTY(r)

		// POSIX -T is exactly name, terminal-state, line and time.
		// LOGIN_PROCESS and INIT_PROCESS have no state field. Only the
		// independently mandatory dead-process exit data may follow this form;
		// optional host and process comments do not belong in it.
		if showMesg && !showIdle {
			if stateFieldExists(r) {
				fmt.Fprintf(rc.Out, "%s %c %s %s", name, terminalState(r), tty, formatTime(r.Time, loc, timeFmt))
			} else {
				fmt.Fprintf(rc.Out, "%s %s %s", name, tty, formatTime(r.Time, loc, timeFmt))
			}
			if isDead(r) {
				fmt.Fprintf(rc.Out, " %s", exitStatus(r))
			}
			fmt.Fprintln(rc.Out)
			continue
		}

		idle := ""
		if showIdle && session.IsUser(r) {
			idle = formatIdle(tty, r.Time)
		}
		comment := lineComment(r, *onlyMe)

		if showMesg && stateFieldExists(r) {
			fmt.Fprintf(rc.Out, "%-8s %c   %-12s %-16s", name, terminalState(r), tty, formatTime(r.Time, loc, timeFmt))
		} else if showMesg {
			fmt.Fprintf(rc.Out, "%-8s     %-12s %-16s", name, tty, formatTime(r.Time, loc, timeFmt))
		} else {
			fmt.Fprintf(rc.Out, "%-8s %-12s %-16s", name, tty, formatTime(r.Time, loc, timeFmt))
		}
		if showIdle {
			fmt.Fprintf(rc.Out, " %-5s", idle)
			if r.PID > 0 {
				fmt.Fprintf(rc.Out, " %5d", r.PID)
			} else {
				fmt.Fprintf(rc.Out, "      ")
			}
		}
		if !showIdle && !shortMode && processRecord(r) && r.PID > 0 {
			fmt.Fprintf(rc.Out, " %5d", r.PID)
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
	switch r.Type {
	case "LOGIN_PROCESS", "login", "6":
		return "LOGIN"
	case "BOOT_TIME", "boot", "2":
		return "system boot"
	case "RUN_LVL", "runlevel", "1":
		return "run-level"
	case "NEW_TIME", "time", "3":
		return "clock change"
	}
	return r.User
}

func displayTTY(r session.Record) string {
	switch r.Type {
	case "BOOT_TIME", "boot", "2", "RUN_LVL", "runlevel", "1",
		"NEW_TIME", "time", "3", "OLD_TIME", "4":
		return ""
	}
	return r.TTY
}

// terminalState is the '%c' of the -T format. A live user or login line
// reports the tty's message/writable status ('+', '-', or '?'); a record
// with no live terminal (dead, boot, run-level, clock change) reports the
// POSIX space state.
func terminalState(r session.Record) byte {
	switch r.Type {
	case "DEAD_PROCESS", "dead", "8",
		"BOOT_TIME", "boot", "2",
		"RUN_LVL", "runlevel", "1",
		"NEW_TIME", "time", "3", "OLD_TIME", "4",
		"LOGIN_PROCESS", "login", "6",
		"INIT_PROCESS", "init", "5":
		return ' '
	}
	if r.TTY == "" {
		return '?'
	}
	return messageStatus(r.TTY)
}

func stateFieldExists(r session.Record) bool {
	switch r.Type {
	case "LOGIN_PROCESS", "login", "6", "INIT_PROCESS", "init", "5":
		return false
	}
	return true
}

// lineComment is the trailing COMMENT field: the origin host for a normal
// login, or the mandatory exit status for a dead process.
func lineComment(r session.Record, onlyMe bool) string {
	if isDead(r) {
		return exitStatus(r)
	}
	if processRecord(r) && r.ID != "" {
		return "id=" + r.ID
	}
	if r.Type == "BOOT_TIME" || r.Type == "boot" || r.Type == "2" {
		return ""
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

func processRecord(r session.Record) bool {
	switch r.Type {
	case "LOGIN_PROCESS", "login", "6", "INIT_PROCESS", "init", "5":
		return true
	}
	return false
}

func isRunLevel(r session.Record) bool {
	switch r.Type {
	case "RUN_LVL", "runlevel", "1":
		return true
	}
	return false
}

func runLevel(pid int) (current, previous byte) {
	current, previous = byte(pid&0xff), byte((pid>>8)&0xff)
	if current < 0x20 || current > 0x7e {
		current = '?'
	}
	if previous < 0x20 || previous > 0x7e {
		previous = '?'
	}
	return current, previous
}

func writeRunLevel(rc *tool.RunContext, r session.Record, loc *time.Location, formatter posixlocale.TimeFormatter) {
	current, previous := runLevel(r.PID)
	fmt.Fprintf(rc.Out, "run-level %c  %s", current, formatTime(r.Time, loc, formatter))
	if previous != '?' {
		fmt.Fprintf(rc.Out, " last=%c", previous)
	}
	fmt.Fprintln(rc.Out)
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

// formatTime keeps timezone and locale selection invocation-local: loc comes
// from tzenv and formatter from the bounded LC_TIME provider.
func formatTime(t time.Time, loc *time.Location, formatter posixlocale.TimeFormatter) string {
	if t.IsZero() {
		return ""
	}
	if loc == nil {
		loc = time.Local
	}
	return formatter.FormatMonthDayTime(t.In(loc))
}

func formatIdle(tty string, loginTime time.Time) string {
	path := session.TTYPath(tty)
	if path == "" {
		return "?"
	}
	at, ok := accessTime(path)
	if !ok {
		return "?"
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
