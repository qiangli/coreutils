package pinkycmd

import (
	"bytes"
	"fmt"
	"os"
	"os/user"
	"path/filepath"
	"runtime"
	"strings"
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
	noHome := fs.BoolP("no-home", "b", false, "omit home directory in long format")
	noPlan := fs.BoolP("no-plan", "h", false, "omit project/plan in long format")
	noProject := fs.BoolP("no-project", "p", false, "omit project in long format")
	doLookup := fs.BoolP("lookup", "i", false, "do a full name, shell, and home lookup for each user")
	quick := fs.BoolP("quick", "q", false, "quick format: only login name and full name")

	args = aliasV(args)
	operands, code := tool.Parse(rc, cmd, fs, args)
	if code >= 0 {
		return code
	}
	if len(operands) == 0 {
		recs, err := session.ReadEnv("", rc.Env)
		if err != nil {
			fmt.Fprintf(rc.Err, "pinky: %v\n", err)
			return 1
		}
		if !*heading {
			fmt.Fprintln(rc.Out, "Login    Name                 TTY      Idle   When             Where")
		}
		for _, r := range recs {
			if session.IsUser(r) {
				displayName := r.User
				if *doLookup {
					if u, err := user.Lookup(r.User); err == nil {
						displayName = u.Name
					}
				}
				if *quick {
					fmt.Fprintf(rc.Out, "%-8s %s\n", r.User, displayName)
				} else {
					fmt.Fprintf(rc.Out, "%-8s %-20s %-8s        %-16s %s\n", r.User, displayName, r.TTY, formatTime(r.Time), r.Host)
				}
			}
		}
		return 0
	}
	status := 0
	for _, name := range operands {
		a, ok := lookupAccount(rc.Env, name)
		if !ok {
			fmt.Fprintf(rc.Err, "pinky: %s: no such user\n", name)
			status = 1
			continue
		}
		if *quick {
			fmt.Fprintf(rc.Out, "%-8s %s\n", name, a.realName)
		} else if *short || !*long {
			if *doLookup {
				fmt.Fprintf(rc.Out, "%-8s %-20s %s\n", name, a.realName, a.home)
			} else {
				fmt.Fprintf(rc.Out, "%-8s %-20s\n", name, a.realName)
			}
		} else {
			writeLong(rc, a, *noHome, *noProject, *noPlan)
		}
	}
	return status
}

type account struct {
	login, realName, home, shell string
}

func lookupAccount(env []string, name string) (account, bool) {
	if u, err := user.Lookup(name); err == nil {
		realName := u.Name
		if realName == "" {
			realName = u.Username
		}
		return account{login: name, realName: realName, home: u.HomeDir, shell: passwdShell(u.Username)}, true
	}
	if dir, ok := session.AgentUserDir(env, name); ok {
		return account{login: name, realName: name, home: dir, shell: "bashy"}, true
	}
	return account{}, false
}

func writeLong(rc *tool.RunContext, a account, noHome, noProject, noPlan bool) {
	fmt.Fprintf(rc.Out, "Login name: %-28s In real life: %s\n", a.login, a.realName)
	if !noHome {
		fmt.Fprintf(rc.Out, "Directory: %-29s Shell: %s\n", a.home, a.shell)
	}
	if !noProject {
		if project, err := os.ReadFile(filepath.Join(a.home, ".project")); err == nil {
			project = firstLine(project)
			fmt.Fprintf(rc.Out, "Project: %s\n", project)
		}
	}
	if !noPlan {
		if plan, err := os.ReadFile(filepath.Join(a.home, ".plan")); err == nil {
			fmt.Fprintln(rc.Out, "Plan:")
			_, _ = rc.Out.Write(plan)
			if len(plan) == 0 || plan[len(plan)-1] != '\n' {
				fmt.Fprintln(rc.Out)
			}
		}
	}
}

func firstLine(data []byte) []byte {
	if i := bytes.IndexByte(data, '\n'); i >= 0 {
		data = data[:i]
	}
	return bytes.TrimSuffix(data, []byte{'\r'})
}

func passwdShell(name string) string {
	if runtime.GOOS == "windows" {
		return ""
	}
	b, err := os.ReadFile("/etc/passwd")
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(b), "\n") {
		fields := strings.Split(line, ":")
		if len(fields) >= 7 && fields[0] == name {
			return fields[6]
		}
	}
	return ""
}

func aliasV(args []string) []string {
	out := make([]string, 0, len(args))
	for _, arg := range args {
		switch arg {
		case "-V":
			out = append(out, "--version")
		default:
			if len(arg) > 2 && arg[0] == '-' && arg[1] != '-' {
				var kept []byte
				kept = append(kept, '-')
				hasV := false
				for _, r := range arg[1:] {
					if r == 'V' {
						hasV = true
					} else {
						kept = append(kept, byte(r))
					}
				}
				if hasV {
					out = append(out, "--version")
				}
				if len(kept) > 1 {
					out = append(out, string(kept))
				}
			} else {
				out = append(out, arg)
			}
		}
	}
	return out
}

func formatTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.Local().Format("2006-01-02 15:04")
}
