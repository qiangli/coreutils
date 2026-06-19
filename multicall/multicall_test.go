package multicall

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/qiangli/coreutils/tool"
)

func init() {
	tool.Register(&tool.Tool{
		Name:     "mctest",
		Synopsis: "multicall test tool",
		Usage:    "mctest [args...]",
		Run: func(rc *tool.RunContext, args []string) int {
			rc.Out.Write([]byte("ran:" + strings.Join(args, ",")))
			return 0
		},
	})
}

func TestResolve(t *testing.T) {
	cases := []struct {
		name     string
		argv0    string
		args     []string
		self     []string
		wantName string
		wantArgs []string
		wantList bool
	}{
		{"frontend with tool", "/usr/bin/coreutils", []string{"ls", "-l"}, []string{"coreutils"}, "ls", []string{"-l"}, false},
		{"frontend no operand", "coreutils", nil, []string{"coreutils"}, "", nil, true},
		{"frontend --list", "coreutils", []string{"--list"}, []string{"coreutils"}, "", nil, true},
		{"argv0 dispatch", "/usr/bin/ls", []string{"-a"}, []string{"coreutils"}, "ls", []string{"-a"}, false},
		{"argv0 .exe stripped", "/bin/ls.exe", []string{"-a"}, []string{"coreutils"}, "ls", []string{"-a"}, false},
		{"bashy frontend", "bashy", []string{"grep", "x"}, []string{"coreutils", "bashy"}, "grep", []string{"x"}, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			name, args, list := Resolve(c.argv0, c.args, c.self...)
			if name != c.wantName || list != c.wantList {
				t.Fatalf("Resolve = (%q,%v,%v), want (%q,_,%v)", name, args, list, c.wantName, c.wantList)
			}
			if strings.Join(args, ",") != strings.Join(c.wantArgs, ",") {
				t.Fatalf("args = %v, want %v", args, c.wantArgs)
			}
		})
	}
}

func TestDispatch(t *testing.T) {
	var out, errb bytes.Buffer
	rc := &tool.RunContext{Ctx: context.Background(), Stdio: tool.Stdio{Out: &out, Err: &errb}}
	if code := Dispatch(rc, "mctest", []string{"a", "b"}); code != 0 {
		t.Fatalf("code = %d, want 0", code)
	}
	if out.String() != "ran:a,b" {
		t.Fatalf("out = %q", out.String())
	}

	out.Reset()
	errb.Reset()
	if code := Dispatch(rc, "nope-not-a-tool", nil); code != 2 {
		t.Fatalf("unknown tool code = %d, want 2", code)
	}
	if !strings.Contains(errb.String(), "not a supported command") {
		t.Fatalf("missing diagnostic: %q", errb.String())
	}
}
