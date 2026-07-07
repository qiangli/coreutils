package sttycmd

import (
	"bytes"
	"context"
	"reflect"
	"strings"
	"testing"

	"github.com/qiangli/coreutils/tool"
)

func TestSttyRejectsNonTTY(t *testing.T) {
	var out, errb bytes.Buffer
	code := run(&tool.RunContext{Ctx: context.Background(), Stdio: tool.Stdio{Out: &out, Err: &errb, In: strings.NewReader("")}}, nil)
	if code != 1 || !strings.Contains(errb.String(), "ioctl") {
		t.Fatalf("code=%d err=%q", code, errb.String())
	}
}

func TestSttyRejectsConflictingOutputStylesBeforeTTY(t *testing.T) {
	var out, errb bytes.Buffer
	code := run(&tool.RunContext{Ctx: context.Background(), Stdio: tool.Stdio{Out: &out, Err: &errb, In: strings.NewReader("")}}, []string{"-a", "-g"})
	if code != 1 || !strings.Contains(errb.String(), "mutually exclusive") {
		t.Fatalf("code=%d err=%q", code, errb.String())
	}
}

func TestSttyRejectsSettingsWithOutputStyleBeforeTTY(t *testing.T) {
	var out, errb bytes.Buffer
	code := run(&tool.RunContext{Ctx: context.Background(), Stdio: tool.Stdio{Out: &out, Err: &errb, In: strings.NewReader("")}}, []string{"-a", "echo"})
	if code != 1 || !strings.Contains(errb.String(), "modes may not be set") {
		t.Fatalf("code=%d err=%q", code, errb.String())
	}
}

func TestParseArgsKeepsHyphenSettings(t *testing.T) {
	var out, errb bytes.Buffer
	all, save, file, operands, code := parseArgs(&tool.RunContext{Ctx: context.Background(), Stdio: tool.Stdio{Out: &out, Err: &errb}}, []string{"-F", "/dev/ttyS0", "-echo", "-raw", "min", "1"})
	if code >= 0 {
		t.Fatalf("code=%d err=%q", code, errb.String())
	}
	if all || save || file != "/dev/ttyS0" {
		t.Fatalf("all=%v save=%v file=%q", all, save, file)
	}
	want := []string{"-echo", "-raw", "min", "1"}
	if !reflect.DeepEqual(operands, want) {
		t.Fatalf("operands=%q want %q", operands, want)
	}
}

func TestParseRowsCols(t *testing.T) {
	tests := []struct {
		in   string
		want uint16
	}{
		{"24", 24},
		{"0x10", 16},
		{"010", 8},
		{"65536", 0},
	}
	for _, tt := range tests {
		got, err := parseRowsCols(tt.in)
		if err != nil || got != tt.want {
			t.Fatalf("parseRowsCols(%q) = %d, %v; want %d", tt.in, got, err, tt.want)
		}
	}
}

func TestParseUint8RejectsOverflow(t *testing.T) {
	if _, err := parseUint8("256"); err == nil {
		t.Fatal("expected overflow error")
	}
}
