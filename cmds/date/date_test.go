package datecmd

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/qiangli/coreutils/tool"
)

func runTool(t *testing.T, args ...string) (stdout, stderr string, code int) {
	t.Helper()
	var out, errb bytes.Buffer
	rc := &tool.RunContext{
		Ctx:   context.Background(),
		Dir:   t.TempDir(),
		Stdio: tool.Stdio{In: strings.NewReader(""), Out: &out, Err: &errb},
	}
	code = cmd.Run(rc, args)
	return out.String(), errb.String(), code
}

func TestDateFormats(t *testing.T) {
	// All anchored at a fixed instant in UTC for determinism.
	cases := []struct {
		args []string
		want string
	}{
		{[]string{"-u", "-d", "@0"}, "Thu Jan  1 00:00:00 UTC 1970\n"},
		{[]string{"-u", "-d", "@0", "+%Y-%m-%d %H:%M:%S"}, "1970-01-01 00:00:00\n"},
		// 2026-06-12 is a Friday; day 163 of the year.
		{[]string{"-u", "-d", "2026-06-12 13:45:09", "+%a %A %b %B"}, "Fri Friday Jun June\n"},
		{[]string{"-u", "-d", "2026-06-12 13:45:09", "+%y %j %e %T %D %F %R"}, "26 163 12 13:45:09 06/12/26 2026-06-12 13:45\n"},
		{[]string{"-u", "-d", "2026-06-12 13:45:09", "+%I %p %u %w"}, "01 PM 5 5\n"},
		{[]string{"-u", "-d", "2026-06-02", "+[%e]"}, "[ 2]\n"},
		{[]string{"-u", "-d", "@1765432109", "+%s"}, "1765432109\n"},
		{[]string{"-u", "-d", "@0.123456789", "+%N"}, "123456789\n"},
		{[]string{"-u", "-d", "@0", "+%z %Z"}, "+0000 UTC\n"},
		{[]string{"-u", "-d", "@0", "+a%nb%tc%%d"}, "a\nb\tc%d\n"},
		// Sunday: %u=7, %w=0.
		{[]string{"-u", "-d", "2026-06-14", "+%u %w"}, "7 0\n"},
		// Midnight: %I is 12, %p is AM.
		{[]string{"-u", "-d", "2026-06-12 00:30:00", "+%I %p"}, "12 AM\n"},
		// Unknown directive passes through literally, like GNU.
		{[]string{"-u", "-d", "@0", "+%q"}, "%q\n"},
		// RFC 3339 input with explicit zone.
		{[]string{"-u", "-d", "2026-06-12T10:00:00Z", "+%F %T"}, "2026-06-12 10:00:00\n"},
		{[]string{"-u", "-d", "2026-06-12T10:00:00+02:00", "+%H"}, "08\n"},
		// --universal alias.
		{[]string{"--universal", "-d", "@0", "+%H"}, "00\n"},
	}
	for _, c := range cases {
		out, errb, code := runTool(t, c.args...)
		if out != c.want || code != 0 {
			t.Errorf("date %q = (%q, %q, %d), want (%q, \"\", 0)", c.args, out, errb, code, c.want)
		}
	}
}

func TestDateFormatAliases(t *testing.T) {
	cases := []struct {
		args []string
		want string
	}{
		{[]string{"-u", "-d", "@0", "--iso-8601"}, "1970-01-01\n"},
		{[]string{"-u", "-d", "@0", "--iso-8601=seconds"}, "1970-01-01T00:00:00+0000\n"},
		{[]string{"-u", "-d", "@0", "--rfc-3339=seconds"}, "1970-01-01 00:00:00+00:00\n"},
		{[]string{"-u", "-d", "@0", "--rfc-email"}, "Thu, 01 Jan 1970 00:00:00 +0000\n"},
	}
	for _, c := range cases {
		out, errb, code := runTool(t, c.args...)
		if code != 0 || errb != "" || out != c.want {
			t.Fatalf("date %q = (%q, %q, %d), want %q", c.args, out, errb, code, c.want)
		}
	}
}

func TestDateFileDebugAndResolution(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "dates")
	if err := os.WriteFile(file, []byte("@0\n1970-01-02 03:04:05\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	out, errb, code := runTool(t, "-u", "--debug", "--file", file, "+%F %T")
	if code != 0 || !strings.Contains(errb, "parsed date") {
		t.Fatalf("--file debug code=%d err=%q", code, errb)
	}
	if want := "1970-01-01 00:00:00\n1970-01-02 03:04:05\n"; out != want {
		t.Fatalf("out=%q want %q", out, want)
	}

	out, errb, code = runTool(t, "--resolution")
	if code != 0 || errb != "" || out != "0.000000001\n" {
		t.Fatalf("--resolution = (%q, %q, %d)", out, errb, code)
	}
}

func TestDateDefaultShape(t *testing.T) {
	out, _, code := runTool(t)
	if code != 0 {
		t.Fatalf("date: code=%d", code)
	}
	// "Fri Jun 12 10:30:45 PDT 2026" shape: 5+ space-separated fields,
	// with a HH:MM:SS field.
	fields := strings.Fields(strings.TrimSuffix(out, "\n"))
	if len(fields) < 5 || !strings.Contains(out, ":") {
		t.Errorf("default output %q does not match C-locale date shape", out)
	}
}

func TestDateReference(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "stamp")
	if err := os.WriteFile(f, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	mtime := time.Unix(1700000000, 0)
	if err := os.Chtimes(f, mtime, mtime); err != nil {
		t.Fatal(err)
	}

	var out, errb bytes.Buffer
	rc := &tool.RunContext{
		Ctx:   context.Background(),
		Dir:   dir,
		Stdio: tool.Stdio{In: strings.NewReader(""), Out: &out, Err: &errb},
	}
	// Relative operand: resolved against rc.Dir, not process cwd.
	code := cmd.Run(rc, []string{"-u", "-r", "stamp", "+%s"})
	if code != 0 || out.String() != "1700000000\n" {
		t.Errorf("-r: code=%d out=%q err=%q", code, out.String(), errb.String())
	}
}

func TestDateErrors(t *testing.T) {
	_, errb, code := runTool(t, "-d", "next fortnight")
	if code != 1 || !strings.Contains(errb, "invalid date") {
		t.Errorf("invalid -d: code=%d err=%q", code, errb)
	}
	_, errb, code = runTool(t, "-r", "no-such-file")
	if code != 1 || !strings.Contains(errb, "no-such-file") {
		t.Errorf("missing -r file: code=%d err=%q", code, errb)
	}
	_, errb, code = runTool(t, "-d", "@0", "-r", "x")
	if code != 2 || !strings.Contains(errb, "mutually exclusive") {
		t.Errorf("-d with -r: code=%d err=%q", code, errb)
	}
	// Set-date operand mode is documented-but-unsupported.
	_, errb, code = runTool(t, "12011030")
	if code != 2 || !strings.Contains(errb, "not supported") {
		t.Errorf("set-date: code=%d err=%q", code, errb)
	}
	_, errb, code = runTool(t, "--set", "@0")
	if code != 2 || !strings.Contains(errb, "not supported") {
		t.Errorf("--set: code=%d err=%q", code, errb)
	}
	_, errb, code = runTool(t, "+%Y", "+%m")
	if code != 2 || !strings.Contains(errb, "extra operand") {
		t.Errorf("two formats: code=%d err=%q", code, errb)
	}
	_, errb, code = runTool(t, "--frobnicate")
	if code != 2 || !strings.Contains(errb, "frobnicate") {
		t.Errorf("unknown flag: code=%d err=%q", code, errb)
	}
}

func TestDateHelp(t *testing.T) {
	out, _, code := runTool(t, "--help")
	if code != 0 || !strings.Contains(out, "Usage: date") {
		t.Errorf("--help: code=%d out=%q", code, out)
	}
}
