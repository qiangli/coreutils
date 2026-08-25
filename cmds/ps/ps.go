// Package pscmd implements the POSIX ps(1) utility.
//
// Process enumeration is provided by github.com/tklauser/ps (BSD-3-Clause).
// Selection and formatting are owned here so their semantics remain testable
// independently of any host ps binary.
package pscmd

import (
	"bytes"
	"fmt"
	"io"
	"os/user"
	"sort"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"
	"unicode/utf8"

	portableps "github.com/tklauser/ps"

	"github.com/qiangli/coreutils/cmds/internal/tzenv"
	"github.com/qiangli/coreutils/pkg/locale"
	"github.com/qiangli/coreutils/tool"
)

var cmd = &tool.Tool{
	Name:     "ps",
	Synopsis: "Report process status.",
	Usage:    "ps [-Aadefl] [-g grouplist] [-G grouplist] [-n namelist] [-o format] [-p proclist] [-t termlist] [-u userlist] [-U userlist]",
}

type options struct {
	all, withTerminals, descendants, full, long bool
	pids, sids, gids, eusers, rusers            map[int]bool
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
	vsz, sz                                            uint64
	flags, addr                                        uint64
	state, wchan                                       string
	priority                                           int
	flagsKnown, addrKnown, priorityKnown               bool
	pgidKnown, niceKnown, cpuKnown, vszKnown, szKnown  bool
}

type processSource interface {
	processes() ([]process, error)
	identity() (uid int, tty string)
}

type liveProcessSource struct{}

func (liveProcessSource) identity() (int, string) { return currentUID(), currentTTY() }

func (liveProcessSource) processes() ([]process, error) {
	base, err := portableps.Processes()
	if err != nil {
		return nil, err
	}
	procs := make([]process, 0, len(base))
	for _, p := range base {
		argv := p.ExecutableArgs()
		q := process{pid: p.PID(), ppid: p.PPID(), ruid: p.UID(), euid: p.UID(), rgid: p.GID(), egid: p.GID(), command: p.Command(), start: p.CreationTime()}
		if len(argv) != 0 {
			q.command = argv[0] // POSIX comm is argv[0], not the executable basename.
			q.args = strings.Join(argv, " ")
		}
		if q.args == "" {
			q.args = q.command
		}
		enrich(&q)
		procs = append(procs, q)
	}
	return procs, nil
}

type renderContext struct {
	now time.Time
	tf  locale.TimeFormatter
	loc *time.Location
}

type column struct {
	name, header string
	minWidth     int
}

func init() { cmd.Run = run; tool.Register(cmd) }

func run(rc *tool.RunContext, args []string) int {
	return runWithSource(rc, args, liveProcessSource{}, time.Now)
}

func runWithSource(rc *tool.RunContext, args []string, source processSource, now func() time.Time) int {
	fs := tool.NewFlags(cmd.Name)
	allA := fs.BoolP("all", "A", false, "select all processes")
	alla := fs.BoolP("terminal-processes", "a", false, "include terminal processes (session leaders may be omitted)")
	desc := fs.BoolP("no-leaders", "d", false, "select all processes except session leaders")
	alle := fs.BoolP("every", "e", false, "select all processes")
	full := fs.BoolP("full", "f", false, "show a full listing")
	long := fs.BoolP("long", "l", false, "show a long listing")
	sids := fs.StringSliceP("group", "g", nil, "select processes by session-leader ID LIST")
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
	tf, err := locale.ResolveTime(rc.Env)
	if err != nil {
		fmt.Fprintf(rc.Err, "ps: %v\n", err)
		return 1
	}
	uid, tty := source.identity()
	o := options{all: *allA || *alle, withTerminals: *alla, descendants: *desc, full: *full, long: *long,
		invokerUID: uid, invokerTTY: tty}
	// err is reused below for all independently validated option lists.
	if o.pids, err = numberSet(*pids, nil); err != nil {
		return usage(rc, err.Error())
	}
	if o.sids, err = numberSet(*sids, nil); err != nil {
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

	procs, err := source.processes()
	if err != nil {
		fmt.Fprintf(rc.Err, "ps: %v\n", err)
		return 1
	}
	selectedProcs := procs[:0]
	for _, p := range procs {
		if selected(p, o) {
			selectedProcs = append(selectedProcs, p)
		}
	}
	procs = selectedProcs
	sort.Slice(procs, func(i, j int) bool { return procs[i].pid < procs[j].pid })
	render := renderContext{now: now(), tf: tf, loc: tzenv.Location(rc.Env)}
	if err := printTableWithContext(rc, procs, o.format, render, displayColumns(rc.Env)); err != nil {
		fmt.Fprintf(rc.Err, "ps: write error: %v\n", err)
		return 1
	}
	return 0
}

func selected(p process, o options) bool {
	explicit := o.all || o.withTerminals || o.descendants ||
		len(o.pids)+len(o.sids)+len(o.gids)+len(o.eusers)+len(o.rusers)+len(o.ttys) > 0
	if o.all {
		return true
	}
	if o.withTerminals && p.pid != p.sid && p.tty != "" && p.tty != "?" {
		return true
	}
	if o.descendants && p.pid != p.sid {
		return true
	}
	if o.pids[p.pid] || o.sids[p.sid] || o.gids[p.rgid] || o.eusers[p.euid] ||
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
			specs = []string{"f,s,uid,pid,ppid,c,pri,nice,addr,sz,wchan,tty,time,comm=CMD"}
		} else if o.full {
			specs = []string{"uid,pid,ppid,c,start,tty,time,args=CMD"}
		} else {
			specs = []string{"pid,tty,time,comm=CMD"}
		}
	}
	var out []column
	for _, spec := range specs {
		for _, commaPart := range strings.Split(spec, ",") {
			rest := strings.TrimLeft(commaPart, " \t")
			for rest != "" {
				blank := strings.IndexAny(rest, " \t")
				eq := strings.IndexByte(rest, '=')
				var item string
				if eq >= 0 && (blank < 0 || eq < blank) {
					// Once '=' starts a header override, every remaining byte in
					// this comma-delimited component is header text. In particular,
					// blanks in "pid=Process ID" are not field separators.
					item, rest = rest, ""
				} else if blank >= 0 {
					item, rest = rest[:blank], strings.TrimLeft(rest[blank:], " \t")
				} else {
					item, rest = rest, ""
				}
				name, header, override := strings.Cut(item, "=")
				name = strings.ToLower(name)
				if !knownColumn(name) {
					return nil, fmt.Errorf("unknown output format %q", name)
				}
				col := column{name: name}
				if override {
					col.header = header
					if header == "" {
						col.minWidth = len(defaultHeader(name))
					}
				} else {
					col.header = defaultHeader(name)
				}
				out = append(out, col)
			}
		}
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("empty output format")
	}
	return out, nil
}

func knownColumn(s string) bool {
	switch s {
	case "f", "s", "uid", "user", "ruser", "gid", "group", "rgroup", "pid", "ppid", "pgid", "c", "pcpu", "pri", "nice", "addr", "vsz", "sz", "wchan", "etime", "start", "time", "tty", "comm", "args":
		return true
	}
	return false
}

func defaultHeader(s string) string {
	m := map[string]string{"f": "F", "s": "S", "uid": "UID", "user": "USER", "ruser": "RUSER", "gid": "GID", "group": "GROUP", "rgroup": "RGROUP", "pid": "PID", "ppid": "PPID", "pgid": "PGID", "c": "C", "pcpu": "%CPU", "pri": "PRI", "nice": "NI", "addr": "ADDR", "vsz": "VSZ", "sz": "SZ", "wchan": "WCHAN", "etime": "ELAPSED", "start": "STIME", "time": "TIME", "tty": "TT", "comm": "COMMAND", "args": "COMMAND"}
	return m[s]
}

func printTable(rc *tool.RunContext, ps []process, cols []column) error {
	tf, _ := locale.ResolveTime([]string{"LC_ALL=C"})
	return printTableWithContext(rc, ps, cols, renderContext{now: time.Now(), tf: tf, loc: time.Local}, 0)
}

func printTableWithContext(rc *tool.RunContext, ps []process, cols []column, render renderContext, columns int) error {
	var output bytes.Buffer
	w := tabwriter.NewWriter(&output, 0, 1, 1, ' ', tabwriter.AlignRight)
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
				// tabwriter right-aligns by padding on the left. Preserve an
				// explicit inter-field blank as POSIX requires as well.
				fmt.Fprint(w, " \t")
			}
			fmt.Fprint(w, c.header)
		}
		fmt.Fprintln(w)
	}
	for _, p := range ps {
		for i, c := range cols {
			if i > 0 {
				if showHeader {
					fmt.Fprint(w, " \t")
				} else {
					fmt.Fprint(w, " ")
				}
			}
			v := valueAt(p, c.name, render)
			if c.minWidth > len(v) {
				v = strings.Repeat(" ", c.minWidth-len(v)) + v
			}
			fmt.Fprint(w, v)
		}
		fmt.Fprintln(w)
	}
	if err := w.Flush(); err != nil {
		return err
	}
	return writeLimited(rc.Out, output.Bytes(), columns)
}

func value(p process, name string) string {
	tf, _ := locale.ResolveTime([]string{"LC_ALL=C"})
	return valueAt(p, name, renderContext{now: time.Now(), tf: tf, loc: time.Local})
}

func valueAt(p process, name string, render renderContext) string {
	switch name {
	case "f":
		if p.flagsKnown {
			return strconv.FormatUint(p.flags, 8)
		}
		return "-"
	case "s":
		if p.state != "" {
			return p.state
		}
		return "-"
	case "pri":
		if p.priorityKnown {
			return strconv.Itoa(p.priority)
		}
		return "-"
	case "addr":
		if p.addrKnown {
			return strconv.FormatUint(p.addr, 16)
		}
		return "-"
	case "wchan":
		if p.wchan != "" && p.wchan != "0" {
			return p.wchan
		}
		return "-"
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
		if !p.pgidKnown {
			return "-"
		}
		return strconv.Itoa(p.pgid)
	case "pcpu":
		if !p.cpuKnown {
			return "-"
		}
		age := render.now.Sub(p.start)
		if p.start.IsZero() || age <= 0 {
			return "0.0"
		}
		return fmt.Sprintf("%.1f", 100*float64(p.cpu)/float64(age))
	case "c":
		if !p.cpuKnown {
			return "-"
		}
		age := render.now.Sub(p.start)
		if p.start.IsZero() || age <= 0 {
			return "0"
		}
		return strconv.FormatInt(int64(100*float64(p.cpu)/float64(age)), 10)
	case "nice":
		if !p.niceKnown {
			return "-"
		}
		return strconv.Itoa(p.nice)
	case "vsz":
		if !p.vszKnown {
			return "-"
		}
		return strconv.FormatUint(p.vsz/1024, 10)
	case "sz":
		if !p.szKnown {
			return "-"
		}
		return strconv.FormatUint(p.sz, 10)
	case "etime":
		if p.start.IsZero() {
			return "-"
		}
		return elapsed(render.now.Sub(p.start))
	case "start":
		if !p.start.IsZero() {
			return startTime(p.start.In(render.loc), render.now.In(render.loc), render.tf)
		}
		return "-"
	case "time":
		if !p.cpuKnown {
			return "-"
		}
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
		for _, s := range strings.FieldsFunc(raw, func(r rune) bool { return r == ',' || r == ' ' || r == '\t' }) {
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
		for _, s := range strings.FieldsFunc(raw, func(r rune) bool { return r == ',' || r == ' ' || r == '\t' }) {
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
	if s >= 86400 {
		return fmt.Sprintf("%d-%02d:%02d:%02d", s/86400, s/3600%24, s/60%60, s%60)
	}
	if s >= 3600 {
		return fmt.Sprintf("%02d:%02d:%02d", s/3600, s/60%60, s%60)
	}
	return fmt.Sprintf("%02d:%02d", s/60, s%60)
}
func cpuTime(d time.Duration) string {
	if d < 0 {
		d = 0
	}
	s := int64(d.Seconds())
	if s >= 86400 {
		return fmt.Sprintf("%d-%02d:%02d:%02d", s/86400, s/3600%24, s/60%60, s%60)
	}
	return fmt.Sprintf("%02d:%02d:%02d", s/3600, s/60%60, s%60)
}

func startTime(start, now time.Time, tf locale.TimeFormatter) string {
	age := now.Sub(start)
	if age >= 0 && age < 24*time.Hour {
		return start.Format("15:04:05")
	}
	// The bounded locale formatter owns month spelling and output encoding.
	// Its final clock component is not part of ps's older-than-24h STIME.
	formatted := tf.FormatMonthDayTime(start)
	if i := strings.LastIndexByte(formatted, ' '); i >= 0 {
		return strings.TrimRight(formatted[:i], " ")
	}
	return formatted
}

func displayColumns(env []string) int {
	for i := len(env) - 1; i >= 0; i-- {
		if v, ok := strings.CutPrefix(env[i], "COLUMNS="); ok {
			n, err := strconv.Atoi(v)
			if err == nil && n > 0 {
				return n
			}
			return 0
		}
	}
	return 0
}

func writeAll(out io.Writer, p []byte) error {
	for len(p) > 0 {
		n, err := out.Write(p)
		if err != nil {
			return err
		}
		if n <= 0 {
			return io.ErrShortWrite
		}
		p = p[n:]
	}
	return nil
}

func writeLimited(out io.Writer, data []byte, columns int) error {
	if columns <= 0 {
		return writeAll(out, data)
	}
	for len(data) > 0 {
		i := bytes.IndexByte(data, '\n')
		var line []byte
		if i < 0 {
			line, data = data, nil
		} else {
			line, data = data[:i], data[i+1:]
		}
		limited := line
		for n, at := 0, 0; at < len(line); n++ {
			if n == columns {
				limited = line[:at]
				break
			}
			_, size := utf8.DecodeRune(line[at:])
			at += size
		}
		if err := writeAll(out, limited); err != nil {
			return err
		}
		if i >= 0 {
			if err := writeAll(out, []byte{'\n'}); err != nil {
				return err
			}
		}
	}
	return nil
}
func usage(rc *tool.RunContext, msg string) int {
	fmt.Fprintf(rc.Err, "ps: %s\nUsage: %s\n", msg, cmd.Usage)
	return 2
}
