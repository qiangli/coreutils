package pscmd

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"reflect"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/qiangli/coreutils/pkg/locale"
	"github.com/qiangli/coreutils/tool"
)

func TestPSOwnPIDAndFormat(t *testing.T) {
	var out, errOut bytes.Buffer
	rc := &tool.RunContext{Ctx: context.Background(), Stdio: tool.Stdio{Out: &out, Err: &errOut}}
	code := run(rc, []string{"-p", "1", "-o", "pid="})
	if code != 0 {
		t.Fatalf("code=%d stderr=%s", code, errOut.String())
	}
	if !strings.Contains(out.String(), "1") {
		t.Fatalf("output=%q", out.String())
	}

	out.Reset()
	errOut.Reset()
	code = run(rc, []string{"-A", "-o", "pid=Process ID"})
	if code != 0 || !strings.HasPrefix(out.String(), "Process ID\n") {
		t.Fatalf("spaced header = (code %d, stdout %q, stderr %q)", code, out.String(), errOut.String())
	}
}

func TestPSExplicitEmptyHeadings(t *testing.T) {
	tests := []struct {
		name string
		cols []column
		ps   []process
		want string
	}{
		{
			name: "single empty heading",
			cols: []column{{name: "pid", header: "", minWidth: len(defaultHeader("pid"))}},
			ps:   []process{{pid: 42}},
			want: " 42\n",
		},
		{
			name: "two empty headings",
			cols: []column{{name: "pid", header: "", minWidth: len(defaultHeader("pid"))}, {name: "ppid", header: "", minWidth: len(defaultHeader("ppid"))}},
			ps:   []process{{pid: 42, ppid: 7}},
			want: " 42    7\n",
		},
		{
			name: "nonempty heading",
			cols: []column{{name: "pid", header: "PID"}},
			ps:   []process{{pid: 42}},
			want: "PID\n42\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var out bytes.Buffer
			rc := &tool.RunContext{Ctx: context.Background(), Stdio: tool.Stdio{Out: &out}}
			if err := printTable(rc, tt.ps, tt.cols); err != nil {
				t.Fatal(err)
			}
			if got := out.String(); got != tt.want {
				t.Fatalf("output=%q, want %q", got, tt.want)
			}
		})
	}
}

func TestPSPOSIXSelectionUnionAndDefaults(t *testing.T) {
	base := process{pid: 10, sid: 1, pgid: 5, ruid: 20, euid: 21, rgid: 30, egid: 31, tty: "pts/2"}
	tests := []struct {
		name string
		p    process
		o    options
		want bool
	}{
		{"default same effective uid and tty", base, options{invokerUID: 21, invokerTTY: "/dev/pts/2"}, true},
		{"default rejects another tty", base, options{invokerUID: 21, invokerTTY: "pts/3"}, false},
		{"default rejects another effective uid", base, options{invokerUID: 22, invokerTTY: "pts/2"}, false},
		{"A unions with an unmatched pid", base, options{all: true, pids: map[int]bool{99: true}}, true},
		{"a includes terminal nonleader", base, options{withTerminals: true}, true},
		{"a uses permitted session leader omission", process{pid: 10, sid: 10, tty: "pts/2"}, options{withTerminals: true}, false},
		{"d includes nonleader without terminal", process{pid: 10, sid: 1}, options{descendants: true}, true},
		{"g selects by session ID", base, options{sids: map[int]bool{1: true}}, true},
		{"g does not select by process group ID", base, options{sids: map[int]bool{5: true}}, false},
		{"real group selection", base, options{gids: map[int]bool{30: true}}, true},
		{"effective user selection", base, options{eusers: map[int]bool{21: true}}, true},
		{"real user selection", base, options{rusers: map[int]bool{20: true}}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := selected(tt.p, tt.o); got != tt.want {
				t.Fatalf("selected=%v, want %v", got, tt.want)
			}
		})
	}
}

func TestPSPOSIXListSeparatorsAndFormatHeaders(t *testing.T) {
	numbers, err := numberSet([]string{"1 2", "3,4\t5"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	for i := 1; i <= 5; i++ {
		if !numbers[i] {
			t.Errorf("blank/comma-separated numeric list omitted %d: %v", i, numbers)
		}
	}
	ttys := stringSet([]string{"tty1 tty2", "pts/3,pts/4\ttty5"})
	for _, name := range []string{"1", "2", "pts/3", "pts/4", "5"} {
		if !ttys[name] {
			t.Errorf("blank/comma-separated terminal list omitted %q: %v", name, ttys)
		}
	}

	cols, err := parseFormat([]string{"pid tty", "args=Full Command"}, options{})
	if err != nil {
		t.Fatal(err)
	}
	want := []column{
		{name: "pid", header: "PID"},
		{name: "tty", header: "TT"},
		{name: "args", header: "Full Command"},
	}
	if !reflect.DeepEqual(cols, want) {
		t.Fatalf("parsed format=%#v, want %#v", cols, want)
	}
	cols, err = parseFormat([]string{"pid=", "args"}, options{})
	if err != nil {
		t.Fatal(err)
	}
	if cols[0].minWidth != len("PID") || cols[0].header != "" || cols[1].header != "COMMAND" {
		t.Fatalf("null/default headers parsed incorrectly: %#v", cols)
	}
}

type failingWriter struct{ err error }

func (w failingWriter) Write([]byte) (int, error) { return 0, w.err }

func TestPSPOSIXNameListAcceptedAndOutputErrorsFail(t *testing.T) {
	var out, errOut bytes.Buffer
	rc := &tool.RunContext{Ctx: context.Background(), Stdio: tool.Stdio{Out: &out, Err: &errOut}}
	if code := run(rc, []string{"-n", "kernel.names", "-p", "1", "-o", "pid="}); code != 0 {
		t.Fatalf("-n must be accepted with a proc-backed enumerator: code=%d stderr=%q", code, errOut.String())
	}

	errOut.Reset()
	rc.Out = failingWriter{err: context.Canceled}
	if err := printTable(rc, []process{{pid: 1}}, []column{{name: "pid"}}); !errors.Is(err, context.Canceled) {
		t.Fatalf("printTable error=%v, want context.Canceled", err)
	}
	errOut.Reset()
	if code := run(rc, []string{"-p", "1", "-o", "pid"}); code != 1 || !strings.Contains(errOut.String(), "write error") {
		t.Fatalf("run output failure=(code %d, stderr %q), want (1, write error)", code, errOut.String())
	}
}

func TestPSRejectsUnknownFormat(t *testing.T) {
	var out, errOut bytes.Buffer
	rc := &tool.RunContext{Ctx: context.Background(), Stdio: tool.Stdio{Out: &out, Err: &errOut}}
	if code := run(rc, []string{"-o", "not-a-field"}); code != 2 {
		t.Fatalf("code=%d stderr=%s", code, errOut.String())
	}
}

type fakeProcessSource struct {
	ps     []process
	uid    int
	tty    string
	err    error
	called *bool
}

func (s fakeProcessSource) identity() (int, string) { return s.uid, s.tty }
func (s fakeProcessSource) processes() ([]process, error) {
	if s.called != nil {
		*s.called = true
	}
	return append([]process(nil), s.ps...), s.err
}

func TestPSHermeticEnumeratorSelectionOrderingAndFields(t *testing.T) {
	now := time.Date(2026, time.March, 4, 12, 0, 0, 0, time.UTC)
	ps := []process{
		{pid: 20, ppid: 2, pgid: 7, ruid: 10, euid: 11, tty: "pts/3", command: "ignored", args: "ignored", start: now.Add(-90 * time.Second)},
		{pid: 10, ppid: 1, pgid: 6, pgidKnown: true, ruid: 11, euid: 10, tty: "pts/2", command: "custom-argv0", args: "custom-argv0 one two", start: now.Add(-2 * time.Hour), cpu: 90 * time.Second, cpuKnown: true, flags: 5, flagsKnown: true, state: "S", priority: 20, priorityKnown: true, addr: 4096, addrKnown: true, wchan: "futex_wait"},
	}
	var out, errOut bytes.Buffer
	rc := &tool.RunContext{Ctx: context.Background(), Env: []string{"LC_ALL=C", "TZ=UTC"}, Stdio: tool.Stdio{Out: &out, Err: &errOut}}
	code := runWithSource(rc, []string{"-p", "10", "-o", "pid=,ppid=,pgid=,f=,s=,pri=,addr=,wchan=,pcpu=,etime=,time=,comm=,args="}, fakeProcessSource{ps: ps, uid: 10, tty: "pts/2"}, func() time.Time { return now })
	if code != 0 {
		t.Fatalf("code=%d stderr=%q", code, errOut.String())
	}
	want := " 10    1    6 5 S  20 1000 futex_wait  1.2 02:00:00 00:01:30 custom-argv0 custom-argv0 one two\n"
	if got := out.String(); got != want {
		t.Fatalf("output=%q, want %q", got, want)
	}
}

func TestPSPOSIXDurationShapes(t *testing.T) {
	tests := []struct {
		d                    time.Duration
		wantElapsed, wantCPU string
	}{
		{5*time.Minute + 6*time.Second, "05:06", "00:05:06"},
		{2*time.Hour + 3*time.Minute + 4*time.Second, "02:03:04", "02:03:04"},
		{49*time.Hour + 2*time.Minute + 3*time.Second, "2-01:02:03", "2-01:02:03"},
	}
	for _, tt := range tests {
		if got := elapsed(tt.d); got != tt.wantElapsed {
			t.Errorf("elapsed(%v)=%q, want %q", tt.d, got, tt.wantElapsed)
		}
		if got := cpuTime(tt.d); got != tt.wantCPU {
			t.Errorf("cpuTime(%v)=%q, want %q", tt.d, got, tt.wantCPU)
		}
	}
}

func TestPSXSIStandardDefaultLayouts(t *testing.T) {
	tests := []struct {
		name    string
		opts    options
		want    []string
		headers []string
	}{
		{"default", options{}, []string{"pid", "tty", "time", "comm"}, []string{"PID", "TT", "TIME", "CMD"}},
		{"full", options{full: true}, []string{"uid", "pid", "ppid", "c", "start", "tty", "time", "args"}, []string{"UID", "PID", "PPID", "C", "STIME", "TT", "TIME", "CMD"}},
		{"long", options{long: true}, []string{"f", "s", "uid", "pid", "ppid", "c", "pri", "nice", "addr", "sz", "wchan", "tty", "time", "comm"}, []string{"F", "S", "UID", "PID", "PPID", "C", "PRI", "NI", "ADDR", "SZ", "WCHAN", "TT", "TIME", "CMD"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cols, err := parseFormat(nil, tt.opts)
			if err != nil {
				t.Fatal(err)
			}
			if len(cols) != len(tt.want) {
				t.Fatalf("columns=%#v", cols)
			}
			for i := range cols {
				if cols[i].name != tt.want[i] || cols[i].header != tt.headers[i] {
					t.Fatalf("column[%d]=%#v, want %s/%s", i, cols[i], tt.want[i], tt.headers[i])
				}
			}
		})
	}
}

func TestPSPOSIXRequiredFormatNamesAndHeaders(t *testing.T) {
	want := map[string]string{
		"ruser": "RUSER", "user": "USER", "rgroup": "RGROUP", "group": "GROUP",
		"pid": "PID", "ppid": "PPID", "pgid": "PGID", "pcpu": "%CPU",
		"vsz": "VSZ", "nice": "NI", "etime": "ELAPSED", "time": "TIME",
		"tty": "TT", "comm": "COMMAND", "args": "COMMAND",
	}
	for name, header := range want {
		if !knownColumn(name) {
			t.Errorf("required format name %q is not recognized", name)
		}
		if got := defaultHeader(name); got != header {
			t.Errorf("defaultHeader(%q)=%q, want %q", name, got, header)
		}
	}
	p := process{}
	tf, _ := locale.ResolveTime([]string{"LC_ALL=C"})
	render := renderContext{now: time.Now(), tf: tf, loc: time.UTC}
	for _, name := range []string{"pgid", "pcpu", "vsz", "nice", "etime"} {
		if got := valueAt(p, name, render); got != "-" {
			t.Errorf("unavailable %s=%q, want permitted hyphen", name, got)
		}
	}
}

func TestPSEnvironmentIsInvocationLocal(t *testing.T) {
	now := time.Date(2026, time.March, 4, 12, 0, 0, 0, time.UTC)
	p := process{pid: 7, start: time.Date(2026, time.March, 4, 10, 0, 0, 0, time.UTC), args: "abcdefghijklmno"}
	older := p
	older.start = time.Date(2026, time.March, 1, 10, 0, 0, 0, time.UTC)
	tests := []struct {
		name string
		env  []string
		args []string
		p    process
		want string
	}{
		{"COLUMNS bounds every line", []string{"LC_ALL=C", "COLUMNS=10"}, []string{"-A", "-o", "pid,args"}, p, " PID COMMA\n   7 abcde\n"},
		{"TZ controls recent start", []string{"LC_ALL=C", "TZ=EST5"}, []string{"-A", "-o", "start="}, p, "05:00:00\n"},
		{"LC_TIME controls old start", []string{"LC_ALL=", "LANG=C", "LC_TIME=de_DE.UTF-8", "TZ=UTC"}, []string{"-A", "-o", "start="}, older, "Mär  1\n"},
		{"LC_ALL overrides LC_TIME", []string{"LC_ALL=C", "LC_TIME=fr_FR.UTF-8", "TZ=UTC"}, []string{"-A", "-o", "start="}, older, "Mar  1\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var out, errOut bytes.Buffer
			rc := &tool.RunContext{Ctx: context.Background(), Env: tt.env, Stdio: tool.Stdio{Out: &out, Err: &errOut}}
			if code := runWithSource(rc, tt.args, fakeProcessSource{ps: []process{tt.p}}, func() time.Time { return now }); code != 0 {
				t.Fatalf("code=%d stderr=%q", code, errOut.String())
			}
			if got := out.String(); got != tt.want {
				t.Fatalf("output=%q, want %q", got, tt.want)
			}
		})
	}

	called := false
	var out, errOut bytes.Buffer
	rc := &tool.RunContext{Ctx: context.Background(), Env: []string{"LC_TIME=fr_FR.UTF-8"}, Stdio: tool.Stdio{Out: &out, Err: &errOut}}
	if code := runWithSource(rc, []string{"-A"}, fakeProcessSource{called: &called}, time.Now); code != 1 || called || !strings.Contains(errOut.String(), "LC_TIME") {
		t.Fatalf("unsupported locale=(code %d, called %v, stderr %q)", code, called, errOut.String())
	}

	errOut.Reset()
	rc.Env = []string{"LC_ALL=C"}
	if code := runWithSource(rc, []string{"-A"}, fakeProcessSource{err: errors.New("enumerator failed")}, time.Now); code != 1 || !strings.Contains(errOut.String(), "enumerator failed") {
		t.Fatalf("enumerator failure=(code %d, stderr %q)", code, errOut.String())
	}
}

func TestPSDisplayColumnsPresenceAndValidation(t *testing.T) {
	for _, tt := range []struct {
		env  []string
		want int
	}{{nil, 0}, {[]string{"COLUMNS="}, 0}, {[]string{"COLUMNS=no"}, 0}, {[]string{"COLUMNS=-2"}, 0}, {[]string{"COLUMNS=3", "COLUMNS=12"}, 12}} {
		if got := displayColumns(tt.env); got != tt.want {
			t.Errorf("displayColumns(%q)=%s, want %d", tt.env, strconv.Itoa(got), tt.want)
		}
	}
}

func TestPSEnrichLinuxProcFixture(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("Linux procfs evidence")
	}
	const pid = 4242
	files := map[string]string{
		"/proc/4242/stat":   "4242 (name with ) char) S 1 2 3 34817 0 4194560 0 0 0 0 120 30 0 0 20 5 1 0 100 8192 2 0 4096",
		"/proc/4242/status": "Name:\tfixture\nUid:\t101\t102\t103\t104\nGid:\t201\t202\t203\t204\n",
		"/proc/4242/wchan":  "futex_wait_queue\n",
		"/proc/4242/statm":  "2 1 0 1 0 0 0\n",
	}
	readFile := func(name string) ([]byte, error) {
		if value, ok := files[name]; ok {
			return []byte(value), nil
		}
		return nil, os.ErrNotExist
	}
	p := process{pid: pid}
	enrichWithReader(&p, readFile)
	if p.state != "S" || p.ppid != 1 || p.pgid != 2 || p.sid != 3 || !p.pgidKnown {
		t.Fatalf("identity/state not parsed: %#v", p)
	}
	if p.tty != "pts/1" {
		t.Fatalf("tty=%q, want pts/1", p.tty)
	}
	if p.ruid != 101 || p.euid != 102 || p.rgid != 201 || p.egid != 202 {
		t.Fatalf("real/effective IDs not parsed: %#v", p)
	}
	if !p.flagsKnown || p.flags != 4194560 || !p.priorityKnown || p.priority != 20 || !p.niceKnown || p.nice != 5 {
		t.Fatalf("flags/priority/nice not parsed: %#v", p)
	}
	if !p.cpuKnown || p.cpu.Seconds() != 1.5 || !p.vszKnown || p.vsz != 8192 || !p.szKnown || p.sz != 2 || !p.addrKnown || p.addr != 4096 || p.wchan != "futex_wait_queue" {
		t.Fatalf("resource fields not parsed: %#v", p)
	}
}

func TestPSLiveLinuxEnumeratorOwnProcess(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("Linux procfs evidence")
	}
	ps, err := (liveProcessSource{}).processes()
	if err != nil {
		t.Fatal(err)
	}
	pid := os.Getpid()
	for _, p := range ps {
		if p.pid != pid {
			continue
		}
		if p.ppid != os.Getppid() || p.pgid <= 0 || p.sid <= 0 {
			t.Fatalf("live process hierarchy invalid: %#v", p)
		}
		if p.ruid != os.Getuid() || p.euid != os.Geteuid() || p.rgid != os.Getgid() || p.egid != os.Getegid() {
			t.Fatalf("live identity invalid: %#v", p)
		}
		if p.command == "" || p.args == "" || p.state == "" || !p.flagsKnown || !p.priorityKnown || !p.niceKnown || !p.cpuKnown || !p.vszKnown || p.vsz == 0 || !p.szKnown || p.sz == 0 || p.start.IsZero() {
			t.Fatalf("live Linux fields incomplete: %#v", p)
		}
		return
	}
	t.Fatalf("own pid %s absent from %s", strconv.Itoa(pid), summarizePIDs(ps))
}

func summarizePIDs(ps []process) string {
	var ids []string
	for i, p := range ps {
		if i == 8 {
			ids = append(ids, "...")
			break
		}
		ids = append(ids, fmt.Sprint(p.pid))
	}
	return "[" + strings.Join(ids, ",") + "]"
}
