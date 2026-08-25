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
		bad  bool
	}{
		{"24", 24, false},
		{"010", 10, false},
		{"0x10", 0, true},
		{"65536", 0, false},
	}
	for _, tt := range tests {
		got, err := parseRowsCols(tt.in)
		if (err != nil) != tt.bad || !tt.bad && got != tt.want {
			t.Fatalf("parseRowsCols(%q) = %d, %v; want %d, bad=%v", tt.in, got, err, tt.want, tt.bad)
		}
	}
}

func TestParseUint8RejectsOverflow(t *testing.T) {
	if _, err := parseUint8("256"); err == nil {
		t.Fatal("expected overflow error")
	}
	t.Run("separate POSIX synopses", testSttyHelpHasSeparatePOSIXSynopses)
	t.Run("malformed saved state", testDecodeStateRejectsMalformedInput)
	t.Run("control character syntax", testParseControlChar)
}

func testSttyHelpHasSeparatePOSIXSynopses(t *testing.T) {
	var out, errb bytes.Buffer
	code := run(&tool.RunContext{Ctx: context.Background(), Stdio: tool.Stdio{Out: &out, Err: &errb}}, []string{"--help"})
	if code != 0 {
		t.Fatalf("code=%d err=%q", code, errb.String())
	}
	for _, synopsis := range []string{"stty [-a|-g]", "stty operand..."} {
		if !strings.Contains(out.String(), synopsis) {
			t.Errorf("help missing %q:\n%s", synopsis, out.String())
		}
	}
}

func testDecodeStateRejectsMalformedInput(t *testing.T) {
	for _, input := range []string{"", "abcd", "v1:1:2", "v1:1:2:3:4:5:6:xyz"} {
		if _, err := decodeState(input); err == nil {
			t.Errorf("decodeState(%q) unexpectedly succeeded", input)
		}
	}
}

func testParseControlChar(t *testing.T) {
	for _, tt := range []struct {
		input string
		want  uint8
	}{{"^C", 3}, {"^c", 3}, {"^?", 0x7f}, {"^-", posixVDisable}, {"x", 'x'}} {
		got, err := parseControlChar(tt.input)
		if err != nil || got != tt.want {
			t.Errorf("parseControlChar(%q)=(%d,%v), want (%d,nil)", tt.input, got, err, tt.want)
		}
	}
	if _, err := parseControlChar("long"); err == nil {
		t.Fatal("multi-character control value unexpectedly accepted")
	}
}
