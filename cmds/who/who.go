package whocmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/qiangli/coreutils/cmds/internal/session"
	"github.com/qiangli/coreutils/tool"
)

var cmd = &tool.Tool{Name: "who", Synopsis: "Show who is logged on.", Usage: "who [OPTION]... [FILE | ARG1 ARG2]"}

func init() { cmd.Run = run; tool.Register(cmd) }

func run(rc *tool.RunContext, args []string) int {
	fs := tool.NewFlags(cmd.Name)
	all := fs.BoolP("all", "a", false, "same as -b -d --login -p -r -t -T -u")
	heading := fs.BoolP("heading", "H", false, "print line of column headings")
	count := fs.BoolP("count", "q", false, "list login names and count")
	_ = fs.BoolP("short", "s", false, "short format")
	usersOnly := fs.BoolP("users", "u", false, "list users logged in")
	mesg := fs.BoolP("mesg", "T", false, "add user's message status")
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
	var live []session.Record
	needBoot := *boot || *all
	needDead := *dead || *all
	needLogin := *login || *all
	needProcess := *process || *all
	needRunlevel := *runlevel || *all
	needTime := *timeChange || *all
	needUsers := *usersOnly || *all || !(needBoot || needDead || needLogin || needProcess || needRunlevel || needTime)

	for _, r := range records {
		if session.IsUser(r) {
			if needUsers {
				live = append(live, r)
			}
			continue
		}
		
		switch r.Type {
		case "BOOT_TIME", "boot", "2":
			if needBoot { live = append(live, r) }
		case "DEAD_PROCESS", "dead", "8":
			if needDead { live = append(live, r) }
		case "LOGIN_PROCESS", "login", "6":
			if needLogin { live = append(live, r) }
		case "INIT_PROCESS", "init", "5":
			if needProcess { live = append(live, r) }
		case "RUN_LVL", "runlevel", "1":
			if needRunlevel { live = append(live, r) }
		case "NEW_TIME", "time", "3", "OLD_TIME", "4":
			if needTime { live = append(live, r) }
		default:
			if *all { live = append(live, r) }
		}
	}
	showMesg := *mesg || *writable || *all || *message
	showIdle := *usersOnly || *all

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

	if *count {
		names := make([]string, 0, len(live))
		for _, r := range live {
			names = append(names, r.User)
		}
		if len(names) > 0 {
			fmt.Fprintln(rc.Out, strings.Join(names, " "))
		}
		fmt.Fprintf(rc.Out, "# users=%d\n", len(names))
		return 0
	}

	if *heading || *all {
		if showMesg {
			fmt.Fprintln(rc.Out, "NAME     STATE LINE         TIME             IDLE   PID  COMMENT")
		} else if showIdle {
			fmt.Fprintln(rc.Out, "NAME     LINE         TIME             IDLE   PID  COMMENT")
		} else {
			fmt.Fprintln(rc.Out, "NAME     LINE         TIME             COMMENT")
		}
	}

	for _, r := range live {
		state := byte(' ')
		if showMesg {
			state = messageStatus(r.TTY)
		}
		idle := ""
		if showIdle {
			idle = formatIdle(r.TTY, r.Time)
		}
		comment := r.Host
		if *onlyMe && comment == "" {
			if h, err := os.Hostname(); err == nil {
				comment = h
			}
		}

		if showMesg {
			fmt.Fprintf(rc.Out, "%-8s %c   %-12s %-16s", r.User, state, r.TTY, formatTime(r.Time))
		} else {
			fmt.Fprintf(rc.Out, "%-8s %-12s %-16s", r.User, r.TTY, formatTime(r.Time))
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
			fmt.Fprintf(rc.Out, " (%s)", comment)
		}
		fmt.Fprintln(rc.Out)
	}
	return 0
}

func parseOperands(operands []string) (file string, sameHost bool, errMsg string) {
	switch len(operands) {
	case 0:
		return "", false, ""
	case 1:
		return operands[0], false, ""
	case 2:
		return "", true, ""
	case 3:
		return operands[0], true, ""
	default:
		return "", false, fmt.Sprintf("extra operand %q", operands[3])
	}
}

func ttyMatch(recordTTY, stdinTTY string) bool {
	record := filepath.Base(recordTTY)
	stdin := filepath.Base(stdinTTY)
	return record != "" && stdin != "" && record == stdin
}

func formatTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.Local().Format("Jan _2 15:04")
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
