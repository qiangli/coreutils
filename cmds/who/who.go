package whocmd

import (
	"fmt"
	"os/user"
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
	short := fs.BoolP("short", "s", false, "short format")
	usersOnly := fs.BoolP("users", "u", false, "list users logged in")
	mesg := fs.BoolP("mesg", "T", false, "add user's message status")
	fs.BoolP("boot", "b", false, "time of last system boot")
	fs.BoolP("dead", "d", false, "print dead processes")
	fs.BoolP("login", "l", false, "print system login processes")
	fs.Bool("lookup", false, "attempt to canonicalize hostnames")
	fs.BoolP("process", "p", false, "print active processes spawned by init")
	fs.BoolP("runlevel", "r", false, "print current runlevel")
	fs.BoolP("time", "t", false, "print last system clock change")
	fs.BoolP("message", "w", false, "same as -T")
	onlyMe := fs.BoolP("same-host", "m", false, "only hostname and user associated with stdin")
	writable := fs.Bool("writable", false, "same as -T")
	operands, code := tool.Parse(rc, cmd, fs, tool.AliasHelpVersion(args))
	if code >= 0 {
		return code
	}
	if len(operands) == 2 {
		fmt.Fprintf(rc.Out, "%s %s\n", operands[0], operands[1])
		return 0
	}
	if len(operands) > 2 {
		return tool.UsageError(rc, cmd, "extra operand %q", operands[2])
	}
	path := ""
	if len(operands) == 1 {
		path = rc.Path(operands[0])
	}
	records, err := session.Read(path)
	if err != nil {
		fmt.Fprintf(rc.Err, "who: %v\n", err)
		return 1
	}
	var live []session.Record
	for _, r := range records {
		if session.IsUser(r) {
			live = append(live, r)
		}
	}
	showMesg := *mesg || *writable

	if *onlyMe {
		u, err := user.Current()
		if err != nil {
			fmt.Fprintf(rc.Err, "who: cannot get current user: %v\n", err)
			return 1
		}
		live = nil
		for _, r := range records {
			if session.IsUser(r) && r.User == u.Username {
				live = append(live, r)
			}
		}
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
		fmt.Fprintln(rc.Out, "NAME     LINE         TIME             COMMENT")
	}
	_ = short
	_ = usersOnly
	for _, r := range live {
		prefix := ""
		if showMesg || *all {
			prefix = "+ "
		}
		fmt.Fprintf(rc.Out, "%-8s %s%-12s %-16s", r.User, prefix, r.TTY, formatTime(r.Time))
		if r.Host != "" {
			fmt.Fprintf(rc.Out, " (%s)", r.Host)
		}
		fmt.Fprintln(rc.Out)
	}
	return 0
}

func formatTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.Local().Format("2006-01-02 15:04")
}
