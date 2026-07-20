package nicecmd

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
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

func TestNiceCommandExitStatuses(t *testing.T) {
	dir := t.TempDir()
	var out, errb bytes.Buffer
	rc := &tool.RunContext{
		Ctx:   context.Background(),
		Dir:   dir,
		Env:   []string{"PATH=" + dir},
		Stdio: tool.Stdio{Out: &out, Err: &errb},
	}
	if code := runCommand(rc, "nice", []string{"missing-command"}, nil, 0); code != 127 {
		t.Fatalf("missing command code=%d, want 127", code)
	}

	if err := os.WriteFile(filepath.Join(dir, "not-executable"), []byte("ignored\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	out.Reset()
	errb.Reset()
	// POSIX distinguishes "found but not executable" (126) from "not found"
	// (127). On Unix the file exists with no exec bit, so nice resolves it and
	// exec fails with EACCES -> 126. Windows has no exec-permission bit:
	// executability is determined solely by a PATHEXT extension, so a file
	// named "not-executable" (no extension) is not an executable command at
	// all and Go's LookPath reports "executable file not found" -> 127. That is
	// the legitimately correct status on Windows; a 126 case is unconstructable
	// there because "exists but not executable" does not exist as a state.
	wantCode := 126
	if runtime.GOOS == "windows" {
		wantCode = 127
	}
	if code := runCommand(rc, "nice", []string{"not-executable"}, nil, 0); code != wantCode {
		t.Fatalf("non-executable command code=%d, want %d (stderr=%q)", code, wantCode, errb.String())
	}
}

func TestLookCommandResolvesPathEntriesFromRunContext(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "command"), []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(dir, "bin"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "bin", "command"), []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name string
		path string
		want string
	}{
		{"relative entry", "bin", filepath.Join(dir, "bin", "command")},
		{"empty path", "", filepath.Join(dir, "command")},
		{"leading empty entry", string(os.PathListSeparator) + "missing", filepath.Join(dir, "command")},
		{"trailing empty entry", "missing" + string(os.PathListSeparator), filepath.Join(dir, "command")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rc := &tool.RunContext{Dir: dir, Env: []string{"PATH=" + tt.path}}
			if got := lookCommand(rc, "command"); got != tt.want {
				t.Fatalf("lookCommand()=%q want %q", got, tt.want)
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
