package pscmd

import (
	"bytes"
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

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
		{name: "tty", header: "TTY"},
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
