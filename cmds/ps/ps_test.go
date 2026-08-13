package pscmd

import (
	"bytes"
	"context"
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
			printTable(rc, tt.ps, tt.cols)
			if got := out.String(); got != tt.want {
				t.Fatalf("output=%q, want %q", got, tt.want)
			}
		})
	}
}

func TestPSRejectsUnknownFormat(t *testing.T) {
	var out, errOut bytes.Buffer
	rc := &tool.RunContext{Ctx: context.Background(), Stdio: tool.Stdio{Out: &out, Err: &errOut}}
	if code := run(rc, []string{"-o", "not-a-field"}); code != 2 {
		t.Fatalf("code=%d stderr=%s", code, errOut.String())
	}
}
