package pscmd

import (
	"bytes"
	"context"
	"errors"
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
			cols: []column{{name: "pid", header: ""}},
			ps:   []process{{pid: 42}},
			want: "42\n",
		},
		{
			name: "two empty headings",
			cols: []column{{name: "pid", header: ""}, {name: "ppid", header: ""}},
			ps:   []process{{pid: 42, ppid: 7}},
			want: "42 7\n",
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
		{"a excludes session leader", process{pid: 10, sid: 10, tty: "pts/2"}, options{withTerminals: true}, false},
		{"d includes nonleader without terminal", process{pid: 10, sid: 1}, options{descendants: true}, true},
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
