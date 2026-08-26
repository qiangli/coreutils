package ctagsfifo

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/qiangli/coreutils/tool"
)

func TestInspectAndRewritePOSIXOutputOptions(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want argPlan
		out  []string
	}{
		{
			name: "default tags",
			args: []string{"source.c"},
			want: argPlan{output: "tags", valueIndex: -1},
			out:  []string{"-f", "/private/output", "source.c"},
		},
		{
			name: "separate operand",
			args: []string{"-a", "-f", "tags pipe", "source.c"},
			want: argPlan{output: "tags pipe", valueIndex: 2, explicit: true},
			out:  []string{"-a", "-f", "/private/output", "source.c"},
		},
		{
			name: "attached operand",
			args: []string{"-ftags pipe", "source.c"},
			want: argPlan{output: "tags pipe", valueIndex: 0, prefixBytes: 2, explicit: true},
			out:  []string{"-f/private/output", "source.c"},
		},
		{
			name: "grouped append and output",
			args: []string{"-aftags pipe", "source.c"},
			want: argPlan{output: "tags pipe", valueIndex: 0, prefixBytes: 3, explicit: true},
			out:  []string{"-af/private/output", "source.c"},
		},
		{
			name: "last output wins",
			args: []string{"-f", "old", "-fnew", "source.c"},
			want: argPlan{output: "new", valueIndex: 2, prefixBytes: 2, explicit: true},
			out:  []string{"-f", "old", "-f/private/output", "source.c"},
		},
		{
			name: "x bypass",
			args: []string{"-x", "-f", "tags pipe", "source.c"},
			want: argPlan{output: "tags pipe", valueIndex: 2, explicit: true, stdout: true},
			out:  []string{"-x", "-f", "/private/output", "source.c"},
		},
		{
			name: "double dash ends options",
			args: []string{"--", "-f", "operand"},
			want: argPlan{output: "tags", valueIndex: -1},
			out:  []string{"-f", "/private/output", "--", "-f", "operand"},
		},
		{
			name: "provider probe disables option files before POSIX options",
			args: []string{"--options=NONE", "-f", "tags pipe", "source.c"},
			want: argPlan{output: "tags pipe", valueIndex: 2, explicit: true},
			out:  []string{"--options=NONE", "-f", "/private/output", "source.c"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := inspectArgs(tt.args)
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("inspectArgs() = %#v, want %#v", got, tt.want)
			}
			if got := got.rewrite(tt.args, "/private/output"); !reflect.DeepEqual(got, tt.out) {
				t.Fatalf("rewrite() = %#v, want %#v", got, tt.out)
			}
		})
	}
}

func TestRegularOutputPassesArgumentsThroughUnchanged(t *testing.T) {
	dir := t.TempDir()
	output := filepath.Join(dir, "tags")
	if err := os.WriteFile(output, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	wantArgs := []string{"-a", "-f", output, "source.c"}
	called := false
	exec := func(_ *tool.RunContext, name, path string, args []string) int {
		called = true
		if name != "ctags" || path != "/provider/ctags" || !reflect.DeepEqual(args, wantArgs) {
			t.Fatalf("exec(%q, %q, %#v)", name, path, args)
		}
		return 7
	}
	rc := &tool.RunContext{Stdio: tool.Stdio{In: bytes.NewReader(nil), Out: io.Discard, Err: io.Discard}}
	if got := Run(rc, "ctags", "/provider/ctags", wantArgs, exec); got != 7 {
		t.Fatalf("Run() = %d, want provider status 7", got)
	}
	if !called {
		t.Fatal("provider was not invoked")
	}
}

func TestMissingFOperandPassesThroughForProviderDiagnostic(t *testing.T) {
	wantArgs := []string{"source.c", "-f"}
	called := false
	exec := func(_ *tool.RunContext, _, _ string, args []string) int {
		called = true
		if !reflect.DeepEqual(args, wantArgs) {
			t.Fatalf("args = %#v, want unchanged %#v", args, wantArgs)
		}
		return 2
	}
	rc := &tool.RunContext{Stdio: tool.Stdio{In: bytes.NewReader(nil), Out: io.Discard, Err: io.Discard}}
	if got := Run(rc, "ctags", "/provider/ctags", wantArgs, exec); got != 2 || !called {
		t.Fatalf("Run() = %d, called=%v", got, called)
	}
}

func TestProviderExtensionArgumentsPassThroughWithoutPOSIXRescan(t *testing.T) {
	tests := []struct {
		name string
		args []string
	}{
		{name: "attached argument contains f", args: []string{"-Ifoo", "source.c"}},
		{name: "attached argument contains x", args: []string{"-Ixmacro", "source.c"}},
		{name: "unknown grouped option", args: []string{"-Rafoutput", "source.c"}},
		{name: "provider long option", args: []string{"--filter=yes", "source.c"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			plan := inspectArgs(tt.args)
			if !plan.passthrough {
				t.Fatalf("inspectArgs(%#v) = %#v, want passthrough", tt.args, plan)
			}
			if got := plan.rewrite(tt.args, "/private/output"); !reflect.DeepEqual(got, tt.args) {
				t.Fatalf("rewrite() = %#v, want unchanged %#v", got, tt.args)
			}

			called := false
			exec := func(_ *tool.RunContext, _, _ string, args []string) int {
				called = true
				if !reflect.DeepEqual(args, tt.args) {
					t.Fatalf("provider args = %#v, want %#v", args, tt.args)
				}
				return 23
			}
			if got := Run(&tool.RunContext{Stdio: tool.Stdio{Out: io.Discard, Err: io.Discard}}, "ctags", "/provider/ctags", tt.args, exec); got != 23 || !called {
				t.Fatalf("Run() = %d, called=%v", got, called)
			}
		})
	}
}
