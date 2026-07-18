package nicecmd

import (
	"bytes"
	"context"
	"reflect"
	"strings"
	"testing"

	"github.com/qiangli/coreutils/tool"
)

func TestParseNiceOptions(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		adjust  int
		given   bool
		command []string
	}{
		{"default adjustment", []string{"echo", "hi"}, 10, false, []string{"echo", "hi"}},
		{"separate -n", []string{"-n", "5", "echo"}, 5, true, []string{"echo"}},
		{"attached -n", []string{"-n5", "echo"}, 5, true, []string{"echo"}},
		{"negative -n", []string{"-n", "-5", "echo"}, -5, true, []string{"echo"}},
		{"plus signed -n", []string{"-n", "+5", "echo"}, 5, true, []string{"echo"}},
		{"long separate", []string{"--adjustment", "7", "echo"}, 7, true, []string{"echo"}},
		{"long equals", []string{"--adjustment=7", "echo"}, 7, true, []string{"echo"}},
		{"long equals negative", []string{"--adjustment=-7", "echo"}, -7, true, []string{"echo"}},
		{"long abbreviation", []string{"--adj", "7", "echo"}, 7, true, []string{"echo"}},
		{"long abbreviation equals", []string{"--adj=7", "echo"}, 7, true, []string{"echo"}},
		{"obsolete positive", []string{"-5", "echo"}, 5, true, []string{"echo"}},
		{"obsolete explicit plus", []string{"-+5", "echo"}, 5, true, []string{"echo"}},
		{"obsolete negative", []string{"--5", "echo"}, -5, true, []string{"echo"}},
		{"last adjustment wins", []string{"-n", "3", "-n", "9", "echo"}, 9, true, []string{"echo"}},
		{"end of options", []string{"--", "echo", "hi"}, 10, false, []string{"echo", "hi"}},
		{"end of options guards dash operand", []string{"--", "-n"}, 10, false, []string{"-n"}},
		{"end of options after adjustment", []string{"-n", "5", "--", "-5"}, 5, true, []string{"-5"}},
		{"command args are not options", []string{"echo", "-n", "3"}, 10, false, []string{"echo", "-n", "3"}},
		{"adjustment clamped high", []string{"-n", "100", "echo"}, 39, true, []string{"echo"}},
		{"adjustment clamped low", []string{"-n", "-100", "echo"}, -39, true, []string{"echo"}},
		{"no operands", []string{}, 10, false, nil},
		{"adjustment without command", []string{"-n", "5"}, 5, true, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var out, errb bytes.Buffer
			rc := &tool.RunContext{Ctx: context.Background(), Stdio: tool.Stdio{Out: &out, Err: &errb}}
			adjust, given, command, code := parseNice(rc, tt.args)
			if code >= 0 {
				t.Fatalf("unexpected early exit code=%d err=%q", code, errb.String())
			}
			if adjust != tt.adjust {
				t.Errorf("adjust=%d want %d", adjust, tt.adjust)
			}
			if given != tt.given {
				t.Errorf("given=%v want %v", given, tt.given)
			}
			if !reflect.DeepEqual(command, tt.command) {
				t.Errorf("command=%q want %q", command, tt.command)
			}
		})
	}
}

func TestParseNiceInvalidAdjustment(t *testing.T) {
	tests := []struct {
		name string
		args []string
	}{
		{"separate", []string{"-n", "x", "echo"}},
		{"attached", []string{"-nx", "echo"}},
		{"long equals", []string{"--adjustment=x", "echo"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var out, errb bytes.Buffer
			rc := &tool.RunContext{Ctx: context.Background(), Stdio: tool.Stdio{Out: &out, Err: &errb}}
			_, _, _, code := parseNice(rc, tt.args)
			if code != 125 {
				t.Fatalf("code=%d want 125", code)
			}
			if !strings.Contains(errb.String(), "invalid adjustment") {
				t.Errorf("stderr=%q", errb.String())
			}
		})
	}
}

func TestParseNiceMissingArgument(t *testing.T) {
	var out, errb bytes.Buffer
	rc := &tool.RunContext{Ctx: context.Background(), Stdio: tool.Stdio{Out: &out, Err: &errb}}
	_, _, _, code := parseNice(rc, []string{"-n"})
	if code != 125 {
		t.Fatalf("code=%d want 125", code)
	}
	if !strings.Contains(errb.String(), "option requires an argument") {
		t.Errorf("stderr=%q", errb.String())
	}
}

func TestParseNiceRejectsUnknownOptions(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{"short", []string{"-x", "echo"}, "invalid option"},
		{"long", []string{"--bogus", "echo"}, "unrecognized option"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var out, errb bytes.Buffer
			rc := &tool.RunContext{Ctx: context.Background(), Stdio: tool.Stdio{Out: &out, Err: &errb}}
			_, _, _, code := parseNice(rc, tt.args)
			if code != 125 {
				t.Fatalf("code=%d want 125", code)
			}
			if !strings.Contains(errb.String(), tt.want) {
				t.Errorf("stderr=%q want %q", errb.String(), tt.want)
			}
		})
	}
}

// A lone "-" is an operand, not an option.
func TestParseNiceDashIsOperand(t *testing.T) {
	var out, errb bytes.Buffer
	rc := &tool.RunContext{Ctx: context.Background(), Stdio: tool.Stdio{Out: &out, Err: &errb}}
	_, _, command, code := parseNice(rc, []string{"-"})
	if code >= 0 {
		t.Fatalf("code=%d err=%q", code, errb.String())
	}
	if !reflect.DeepEqual(command, []string{"-"}) {
		t.Errorf("command=%q", command)
	}
}

// GNU nice exits 125 (EXIT_CANCELED), not with a generic usage status, when an
// adjustment is supplied without a command.
func TestNiceAdjustmentWithoutCommand(t *testing.T) {
	var out, errb bytes.Buffer
	rc := &tool.RunContext{Ctx: context.Background(), Stdio: tool.Stdio{Out: &out, Err: &errb}}
	code := run(rc, []string{"-n", "5"})
	if code != 125 {
		t.Fatalf("code=%d want 125 (stderr=%q)", code, errb.String())
	}
	if !strings.Contains(errb.String(), "a command must be given with an adjustment") {
		t.Errorf("stderr=%q", errb.String())
	}
	if out.String() != "" {
		t.Errorf("stdout=%q want empty", out.String())
	}
}
