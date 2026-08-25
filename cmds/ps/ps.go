// Package pscmd implements the POSIX ps(1) utility.
//
// Process enumeration is provided by github.com/tklauser/ps (BSD-3-Clause).
// Selection and formatting are owned here so their semantics remain testable
// independently of any host ps binary.
package pscmd

import (
	"fmt"
	"os/user"
	"sort"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"

	portableps "github.com/tklauser/ps"

	"github.com/qiangli/coreutils/tool"
)

var cmd = &tool.Tool{
	Name:     "ps",
	Synopsis: "Report process status.",
	Usage:    "ps [-Aadefl] [-g grouplist] [-G grouplist] [-n namelist] [-o format] [-p proclist] [-t termlist] [-u userlist] [-U userlist]",
}

type options struct {
	all, withTerminals, descendants, full, long bool
	pids, pgids, gids, eusers, rusers           map[int]bool
	ttys                                        map[string]bool
	format                                      []column
	invokerUID                                  int
	invokerTTY                                  string
}

type process struct {
	pid, ppid, pgid, sid, ruid, euid, rgid, egid, nice int
	command, args, tty                                 string
	start                                              time.Time
	cpu                                                time.Duration
	vsz                                                uint64
}

type column struct {
	name, header string
}

func init() { cmd.Run = run; tool.Register(cmd) }

func run(rc *tool.RunContext, args []string) int {
	fs := tool.NewFlags(cmd.Name)
	allA := fs.BoolP("all", "A", false, "select all processes")
	alla := fs.BoolP("terminal-processes", "a", false, "include processes associated with terminals")
	desc := fs.BoolP("no-leaders", "d", false, "select all processes except session leaders")
	alle := fs.BoolP("every", "e", false, "select all processes")
	full := fs.BoolP("full", "f", false, "show a full listing")
	long := fs.BoolP("long", "l", false, "show a long listing")
	pgids := fs.StringSliceP("group", "g", nil, "select process-group leaders in LIST")
	gids := fs.StringSliceP("Group", "G", nil, "select by real group LIST")
	fs.StringSliceP("name-list", "n", nil, "alternate name list (not supported)")
	formats := fs.StringSliceP("format", "o", nil, "select output FORMAT")
	pids := fs.StringSliceP("pid", "p", nil, "select processes by PID LIST")
	ttys := fs.StringSliceP("tty", "t", nil, "select processes by terminal LIST")
	eusers := fs.StringSliceP("user", "u", nil, "select by effective user LIST")
	rusers := fs.StringSliceP("User", "U", nil, "select by real user LIST")
	operands, code := tool.Parse(rc, cmd, fs, args)
	if code >= 0 {
		return code
	}
	if len(operands) != 0 {
		return usage(rc, "unexpected operand "+strconv.Quote(operands[0]))
	}
	// -n supplies an alternate kernel name list. The proc-backed enumerator
	// needs no namelist, so accepting it as a no-op has the required observable
	// behavior on systems where process data is available without one.
	o := options{all: *allA || *alle, withTerminals: *alla, descendants: *desc, full: *full, long: *long,
		invokerUID: currentUID(), invokerTTY: currentTTY()}
	var err error
	if o.pids, err = numberSet(*pids, nil); err != nil {
		return usage(rc, err.Error())
	}
	if o.pgids, err = numberSet(*pgids, nil); err != nil {
		return usage(rc, err.Error())
	}
	if o.gids, err = numberSet(*gids, lookupGroup); err != nil {
		return usage(rc, err.Error())
	}
	if o.eusers, err = numberSet(*eusers, lookupUser); err != nil {
		return usage(rc, err.Error())
	}
	if o.rusers, err = numberSet(*rusers, lookupUser); err != nil {
		return usage(rc, err.Error())
	}
	o.ttys = stringSet(*ttys)
	o.format, err = parseFormat(*formats, o)
	if err != nil {
		return usage(rc, err.Error())
	}

	base, err := portableps.Processes()
	if err != nil {
		fmt.Fprintf(rc.Err, "ps: %v\n", err)
		return 1
	}
	procs := make([]process, 0, len(base))
	for _, p := range base {
		q := process{pid: p.PID(), ppid: p.PPID(), ruid: p.UID(), euid: p.UID(), rgid: p.GID(), egid: p.GID(), command: p.Command(), start: p.CreationTime()}
		q.args = strings.Join(p.ExecutableArgs(), " ")
		if q.args == "" {
			q.args = q.command
		}
		enrich(&q)
		if selected(q, o) {
			procs = append(procs, q)
		}
	}
	sort.Slice(procs, func(i, j int) bool { return procs[i].pid < procs[j].pid })
	if err := printTable(rc, procs, o.format); err != nil {
		fmt.Fprintf(rc.Err, "ps: write error: %v\n", err)
		return 1
	}
	return 0
}

func selected(p process, o options) bool {
	explicit := o.all || o.withTerminals || o.descendants ||
		len(o.pids)+len(o.pgids)+len(o.gids)+len(o.eusers)+len(o.rusers)+len(o.ttys) > 0
	if o.all {
		return true
	}
	if o.withTerminals && p.pid != p.sid && p.tty != "" && p.tty != "?" {
		return true
	}
	if o.descendants && p.pid != p.sid {
		return true
	}
	if o.pids[p.pid] || o.pgids[p.pgid] || o.gids[p.rgid] || o.eusers[p.euid] ||
		o.rusers[p.ruid] || o.ttys[cleanTTY(p.tty)] {
		return true
	}
	if explicit {
		return false
	}
	// With no selection option, POSIX selects processes with the same effective
	// user and terminal as the invoker. A missing controlling terminal is a
	// terminal identity too, so two "?" processes compare equal here.
	return (o.invokerUID < 0 || p.euid == o.invokerUID) &&
		cleanTTY(p.tty) == cleanTTY(o.invokerTTY)
}

func parseFormat(specs []string, o options) ([]column, error) {
	if len(specs) == 0 {
		if o.long {
			specs = []string{"f,s,uid,pid,ppid,pcpu,pri,nice,addr,sz,wchan,tty,time,comm"}
		} else if o.full {
			specs = []string{"user,pid,ppid,pcpu,start,tty,time,args"}
		} else {
			specs = []string{"pid,tty,time,comm"}
		}
	}
	var out []column
	for _, spec := range specs {
		for _, item := range strings.FieldsFunc(spec, func(r rune) bool { return r == ',' || r == ' ' || r == '\t' }) {
			name, header, ok := strings.Cut(item, "=")
			name = strings.ToLower(name)
			if !knownColumn(name) {
				return nil, fmt.Errorf("unknown output format %q", name)
			}
			if !ok {
				header = defaultHeader(name)
			}
			out = append(out, column{name: name, header: header})
		}
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("empty output format")
	}
	return out, nil
}

func knownColumn(s string) bool {
	switch s {
	case "f", "s", "uid", "user", "ruser", "gid", "group", "rgroup", "pid", "ppid", "pgid", "pcpu", "pri", "nice", "addr", "vsz", "sz", "wchan", "etime", "start", "time", "tty", "comm", "args":
		return true
	}
	return false
}

func defaultHeader(s string) string {
	m := map[string]string{"f": "F", "s": "S", "uid": "UID", "user": "USER", "ruser": "RUSER", "gid": "GID", "group": "GROUP", "rgroup": "RGROUP", "pid": "PID", "ppid": "PPID", "pgid": "PGID", "pcpu": "%CPU", "pri": "PRI", "nice": "NI", "addr": "ADDR", "vsz": "VSZ", "sz": "SZ", "wchan": "WCHAN", "etime": "ELAPSED", "start": "STIME", "time": "TIME", "tty": "TTY", "comm": "COMMAND", "args": "CMD"}
	return m[s]
}

func printTable(rc *tool.RunContext, ps []process, cols []column) error {
	w := tabwriter.NewWriter(rc.Out, 0, 1, 1, ' ', tabwriter.AlignRight)
	showHeader := false
	for _, c := range cols {
		if c.header != "" {
			showHeader = true
			break
		}
	}
	if showHeader {
		for i, c := range cols {
			if i > 0 {
				fmt.Fprint(w, "\t")
			}
			fmt.Fprint(w, c.header)
		}
		fmt.Fprintln(w)
	}
	for _, p := range ps {
		for i, c := range cols {
			if i > 0 {
				if showHeader {
					fmt.Fprint(w, "\t")
				} else {
					fmt.Fprint(w, " ")
				}
			}
			fmt.Fprint(w, value(p, c.name))
		}
		fmt.Fprintln(w)
	}
	return w.Flush()
}

func value(p process, name string) string {
	switch name {
	case "f", "pri", "addr", "wchan":
		return "-"
	case "s":
		return "?"
	case "uid":
		return strconv.Itoa(p.euid)
	case "gid":
		return strconv.Itoa(p.egid)
	case "user", "ruser":
		if name == "ruser" {
			return userName(p.ruid)
		}
		return userName(p.euid)
	case "group", "rgroup":
		if name == "rgroup" {
			return groupName(p.rgid)
		}
		return groupName(p.egid)
	case "pid":
		return strconv.Itoa(p.pid)
	case "ppid":
		return strconv.Itoa(p.ppid)
	case "pgid":
		return strconv.Itoa(p.pgid)
	case "pcpu":
		return "0.0"
	case "nice":
		return strconv.Itoa(p.nice)
	case "vsz":
		return strconv.FormatUint(p.vsz/1024, 10)
	case "sz":
		return strconv.FormatUint(p.vsz/4096, 10)
	case "etime":
		return elapsed(time.Since(p.start))
	case "start":
		if !p.start.IsZero() {
			return p.start.Local().Format("15:04")
		}
	case "time":
		return cpuTime(p.cpu)
	case "tty":
		if p.tty == "" {
			return "?"
		}
		return cleanTTY(p.tty)
	case "comm":
		return p.command
	case "args":
		return p.args
	}
	return ""
}

func numberSet(values []string, lookup func(string) (int, error)) (map[int]bool, error) {
	out := map[int]bool{}
	for _, raw := range values {
		for _, s := range strings.Split(raw, ",") {
			s = strings.TrimSpace(s)
			if s == "" {
				continue
			}
			n, err := strconv.Atoi(s)
			if err != nil && lookup != nil {
				n, err = lookup(s)
			}
			if err != nil || n < 0 {
				return nil, fmt.Errorf("invalid list value %q", s)
			}
			out[n] = true
		}
	}
	return out, nil
}

func stringSet(values []string) map[string]bool {
	out := map[string]bool{}
	for _, raw := range values {
		for _, s := range strings.Split(raw, ",") {
			if s = cleanTTY(strings.TrimSpace(s)); s != "" {
				out[s] = true
			}
		}
	}
	return out
}
func cleanTTY(s string) string { return strings.TrimPrefix(strings.TrimPrefix(s, "/dev/"), "tty") }
func lookupUser(s string) (int, error) {
	u, err := user.Lookup(s)
	if err != nil {
		return 0, err
	}
	return strconv.Atoi(u.Uid)
}
func lookupGroup(s string) (int, error) {
	g, err := user.LookupGroup(s)
	if err != nil {
		return 0, err
	}
	return strconv.Atoi(g.Gid)
}
func userName(id int) string {
	if u, err := user.LookupId(strconv.Itoa(id)); err == nil {
		return u.Username
	}
	return strconv.Itoa(id)
}
func groupName(id int) string {
	if g, err := user.LookupGroupId(strconv.Itoa(id)); err == nil {
		return g.Name
	}
	return strconv.Itoa(id)
}
func elapsed(d time.Duration) string {
	if d < 0 {
		d = 0
	}
	s := int64(d.Seconds())
	return fmt.Sprintf("%02d:%02d:%02d", s/3600, s/60%60, s%60)
}
func cpuTime(d time.Duration) string {
	s := int64(d.Seconds())
	return fmt.Sprintf("%02d:%02d:%02d", s/3600, s/60%60, s%60)
}
func usage(rc *tool.RunContext, msg string) int {
	fmt.Fprintf(rc.Err, "ps: %s\nUsage: %s\n", msg, cmd.Usage)
	return 2
}
