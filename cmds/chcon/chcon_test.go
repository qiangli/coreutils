package chconcmd

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/qiangli/coreutils/tool"
)

func runTool(t *testing.T, dir string, args ...string) (stdout, stderr string, code int) {
	t.Helper()
	var out, errb bytes.Buffer
	rc := &tool.RunContext{
		Ctx:   context.Background(),
		Dir:   dir,
		Stdio: tool.Stdio{In: strings.NewReader(""), Out: &out, Err: &errb},
	}
	code = cmd.Run(rc, args)
	return out.String(), errb.String(), code
}

func TestChconUsageErrors(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{"no args", nil, "missing operand"},
		{"context no file", []string{"system_u:object_r:tmp_t:s0"}, "missing operand after"},
		{"reference with components", []string{"--reference=r", "-u", "user_u", "f"}, "cannot specify both --reference and context component options"},
		{"context with components", []string{"system_u:object_r:etc_t:s0", "-u", "user_u", "f"}, "cannot specify a CONTEXT value with context component options"},
		{"context with reference", []string{"--reference=r", "system_u:object_r:etc_t:s0", "f"}, "cannot specify a CONTEXT value with --reference"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, errb, code := runTool(t, t.TempDir(), tt.args...)
			if code != 2 || !strings.Contains(errb, tt.want) {
				t.Fatalf("code=%d err=%q, want usage containing %q", code, errb, tt.want)
			}
		})
	}
}

func TestChconHelpAndVersion(t *testing.T) {
	out, _, code := runTool(t, t.TempDir(), "--help")
	if code != 0 || !strings.Contains(out, "Usage: chcon") {
		t.Fatalf("--help: code=%d out=%q", code, out)
	}
	for _, want := range []string{
		"--recursive", "--dereference", "--no-dereference", "--preserve-root",
		"--no-preserve-root", "--reference", "--user", "--role", "--type",
		"--range", "--verbose", "-H", "-L", "-P",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("--help missing %q in:\n%s", want, out)
		}
	}
	out, _, code = runTool(t, t.TempDir(), "--version")
	if code != 0 || !strings.Contains(out, "chcon") {
		t.Fatalf("--version: code=%d out=%q", code, out)
	}
}

func TestChconFlagParsingGetsPastValidation(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "f"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	_, errb, code := runTool(t, dir, "-R", "-v", "-u", "unconfined_u", "f")
	if code == 2 || strings.Contains(errb, "unknown shorthand flag") {
		t.Fatalf("component flags did not get past parsing: code=%d err=%q", code, errb)
	}
}

func TestChconUnsupportedOutsideLinux(t *testing.T) {
	if runtime.GOOS == "linux" {
		t.Skip("non-linux assertion")
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "f"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	_, errb, code := runTool(t, dir, "system_u:object_r:tmp_t:s0", "f")
	if code != 1 || !strings.Contains(errb, "SELinux context changes are not supported on "+runtime.GOOS) {
		t.Fatalf("unsupported: code=%d err=%q", code, errb)
	}
}

func TestMergeContext(t *testing.T) {
	ptr := func(s string) *string { return &s }
	tests := []struct {
		name    string
		current string
		parts   contextParts
		want    string
		wantErr bool
	}{
		{
			name:    "replace user",
			current: "system_u:object_r:tmp_t:s0",
			parts:   contextParts{user: ptr("unconfined_u")},
			want:    "unconfined_u:object_r:tmp_t:s0",
		},
		{
			name:    "replace range containing colon",
			current: "system_u:object_r:tmp_t:s0",
			parts:   contextParts{rang: ptr("s0-s0:c0.c15")},
			want:    "system_u:object_r:tmp_t:s0-s0:c0.c15",
		},
		{
			name:    "add range",
			current: "system_u:object_r:tmp_t",
			parts:   contextParts{rang: ptr("s0")},
			want:    "system_u:object_r:tmp_t:s0",
		},
		{
			name:    "replace all components",
			current: "a:b:c:d:e",
			parts: contextParts{
				user: ptr("u"),
				role: ptr("r"),
				typ:  ptr("t"),
				rang: ptr("s0-s0:c0.c15"),
			},
			want: "u:r:t:s0-s0:c0.c15",
		},
		{
			name:    "invalid context",
			current: "system_u:object_r",
			parts:   contextParts{typ: ptr("tmp_t")},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := mergeContext(tt.current, tt.parts)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("mergeContext() error=nil")
				}
				return
			}
			if err != nil || got != tt.want {
				t.Fatalf("mergeContext() = %q, %v; want %q", got, err, tt.want)
			}
		})
	}
}
