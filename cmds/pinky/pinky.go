package pinkycmd

import (
	"fmt"
	"os/user"
	"time"

	"github.com/qiangli/coreutils/cmds/internal/session"
	"github.com/qiangli/coreutils/tool"
)

var cmd = &tool.Tool{Name: "pinky", Synopsis: "Lightweight finger.", Usage: "pinky [OPTION]... [USER]..."}

func init() { cmd.Run = run; tool.Register(cmd) }

func run(rc *tool.RunContext, args []string) int {
	fs := tool.NewFlags(cmd.Name)
	short := fs.BoolP("short", "s", false, "short format")
	long := fs.BoolP("long", "l", false, "long format")
	heading := fs.BoolP("heading", "f", false, "omit short-format headings")
	fs.BoolP("no-name", "w", false, "omit full name in short format")
	fs.BoolP("no-home", "b", false, "omit home directory in long format")
	fs.BoolP("no-plan", "h", false, "omit project/plan in long format")
	fs.BoolP("no-project", "p", false, "omit project in long format")
	operands, code := tool.Parse(rc, cmd, fs, args)
	if code >= 0 {
		return code
	}
	_ = long
	if len(operands) == 0 {
		recs, err := session.Read("")
		if err != nil {
			fmt.Fprintf(rc.Err, "pinky: %v\n", err)
			return 1
		}
		if !*heading {
			fmt.Fprintln(rc.Out, "Login    Name                 TTY      Idle   When             Where")
		}
		for _, r := range recs {
			if session.IsUser(r) {
				fmt.Fprintf(rc.Out, "%-8s %-20s %-8s        %-16s %s\n", r.User, r.User, r.TTY, formatTime(r.Time), r.Host)
			}
		}
		return 0
	}
	for _, name := range operands {
		u, err := user.Lookup(name)
		if err != nil {
			fmt.Fprintf(rc.Err, "pinky: %s: no such user\n", name)
			continue
		}
		if *short {
			fmt.Fprintf(rc.Out, "%-8s %-20s\n", name, u.Name)
		} else {
			fmt.Fprintf(rc.Out, "Login name: %s\nDirectory: %s\nShell: \n", name, u.HomeDir)
		}
	}
	return 0
}

func formatTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.Local().Format("2006-01-02 15:04")
}
