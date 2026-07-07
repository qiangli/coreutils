package stdbufcmd

import (
	"bytes"
	"context"
	"runtime"
	"strings"
	"testing"

	"github.com/qiangli/coreutils/tool"
)

func TestStdbufRejectsLineBufferedInput(t *testing.T) {
	var out, errb bytes.Buffer
	code := run(&tool.RunContext{Ctx: context.Background(), Dir: t.TempDir(), Stdio: tool.Stdio{Out: &out, Err: &errb, In: strings.NewReader("")}}, []string{"-i", "L", "echo"})
	if code != 125 {
		t.Fatalf("code=%d err=%q", code, errb.String())
	}
}

func TestStdbufRequiresBufferingOption(t *testing.T) {
	var out, errb bytes.Buffer
	code := run(&tool.RunContext{Ctx: context.Background(), Dir: t.TempDir(), Stdio: tool.Stdio{Out: &out, Err: &errb, In: strings.NewReader("")}}, []string{"echo"})
	if code != 125 || !strings.Contains(errb.String(), "missing buffering mode option") {
		t.Fatalf("code=%d err=%q", code, errb.String())
	}
}

func TestNormalizeModeParsesSizeSuffixes(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{"0", "0"},
		{"1B", "1"},
		{"2b", "1024"},
		{"1K", "1024"},
		{"1KB", "1000"},
		{"1KiB", "1024"},
		{"3M", "3145728"},
	}
	for _, tt := range tests {
		got, err := normalizeMode(tt.in, false)
		if err != nil || got != tt.want {
			t.Fatalf("normalizeMode(%q) = %q, %v; want %q", tt.in, got, err, tt.want)
		}
	}
}

func TestNormalizeModeRejectsInvalidSuffix(t *testing.T) {
	if _, err := normalizeMode("12XB", false); err == nil {
		t.Fatal("expected invalid suffix error")
	}
}

func TestStdbufRunsCommand(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell command differs on windows")
	}
	var out, errb bytes.Buffer
	code := run(&tool.RunContext{Ctx: context.Background(), Dir: t.TempDir(), Env: []string{"PATH=/bin:/usr/bin"}, Stdio: tool.Stdio{Out: &out, Err: &errb, In: strings.NewReader("")}}, []string{"-o", "0", "sh", "-c", "printf ok"})
	if code != 0 || out.String() != "ok" {
		t.Fatalf("code=%d out=%q err=%q", code, out.String(), errb.String())
	}
}
