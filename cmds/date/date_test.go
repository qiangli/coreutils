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

type failingWriter struct{ err error }

func (w failingWriter) Write([]byte) (int, error) { return 0, w.err }

func runTool(t *testing.T, args ...string) (stdout, stderr string, code int) {
	t.Helper()
	return runToolEnv(t, nil, args...)
}

func runToolEnv(t *testing.T, env []string, args ...string) (stdout, stderr string, code int) {
	t.Helper()
	var out, errb bytes.Buffer
	rc := &tool.RunContext{
		Ctx:   context.Background(),
		Dir:   t.TempDir(),
		Env:   env,
		Stdio: tool.Stdio{In: strings.NewReader(""), Out: &out, Err: &errb},
	}
	code = cmd.Run(rc, args)
	return out.String(), errb.String(), code
}

func runToolClock(t *testing.T, env []string, now time.Time, setter clockSetter, args ...string) (stdout, stderr string, code int) {
	t.Helper()
	var out, errb bytes.Buffer
	rc := &tool.RunContext{Ctx: context.Background(), Dir: t.TempDir(), Env: env, Stdio: tool.Stdio{In: strings.NewReader(""), Out: &out, Err: &errb}}
	code = runWithClock(rc, args, func() time.Time { return now }, setter)
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
		// Missing POSIX strftime directives.
		{[]string{"-u", "-d", "@0", "+%c"}, "Thu Jan  1 00:00:00 1970\n"},
		{[]string{"-u", "-d", "@0", "+%C %g %G %r %U %V %W %x %X"}, "19 70 1970 12:00:00 AM 00 01 00 01/01/70 00:00:00\n"},
		// Week numbers for specific edge cases.
		{[]string{"-u", "-d", "2023-01-01", "+%U %W %V"}, "01 00 52\n"},
		{[]string{"-u", "-d", "2022-01-01", "+%U %W %V"}, "00 00 52\n"},
		{[]string{"-u", "-d", "2024-01-01", "+%U %W %V"}, "00 01 01\n"},
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

// POSIX XBD strftime: in the C/POSIX locale the %E and %O alternative
// modifiers render exactly as the unmodified conversion. Each case
// pins the exact output and cross-checks it against the unmodified
// format string at the same instant (2026-06-12 13:45:09 UTC, a
// Friday, day 163 of the year).
func TestDateAlternativeModifiers(t *testing.T) {
	const instant = "2026-06-12 13:45:09"
	cases := []struct {
		modified, plain string
		want            string
	}{
		{"%Ec", "%c", "Fri Jun 12 13:45:09 2026"},
		{"%EC", "%C", "20"},
		{"%Ex", "%x", "06/12/26"},
		{"%EX", "%X", "13:45:09"},
		{"%Ey", "%y", "26"},
		{"%EY", "%Y", "2026"},
		{"%Od", "%d", "12"},
		{"%Oe", "%e", "12"},
		{"%OH", "%H", "13"},
		{"%OI", "%I", "01"},
		{"%Om", "%m", "06"},
		{"%OM", "%M", "45"},
		{"%OS", "%S", "09"},
		{"%Ou", "%u", "5"},
		{"%OU", "%U", "23"},
		{"%OV", "%V", "24"},
		{"%Ow", "%w", "5"},
		{"%OW", "%W", "23"},
		{"%Oy", "%y", "26"},
	}
	for _, c := range cases {
		out, errb, code := runTool(t, "-u", "-d", instant, "+"+c.modified)
		if code != 0 || errb != "" || out != c.want+"\n" {
			t.Errorf("date +%s = (%q, %q, %d), want %q", c.modified, out, errb, code, c.want+"\n")
		}
		plain, _, _ := runTool(t, "-u", "-d", instant, "+"+c.plain)
		if out != plain {
			t.Errorf("date +%s = %q, differs from unmodified +%s = %q", c.modified, out, c.plain, plain)
		}
	}
	// Invalid modifier combinations and a trailing modifier are not
	// conversions; they pass through literally like other unknown
	// sequences.
	for _, c := range []struct{ format, want string }{
		{"%Oz", "%Oz"},
		{"%Ed", "%Ed"},
		{"%EO", "%EO"},
		{"%E", "%E"},
		{"%O", "%O"},
	} {
		out, _, code := runTool(t, "-u", "-d", instant, "+"+c.format)
		if code != 0 || out != c.want+"\n" {
			t.Errorf("date +%s = (%q, %d), want %q", c.format, out, code, c.want+"\n")
		}
	}
}

// POSIX XBD 8.3: TZ selects the timezone for conversion; -u overrides
// it. Both POSIX expansions and IANA names must work, and an
// unusable value falls back to UTC rather than erroring.
func TestDateTZ(t *testing.T) {
	cases := []struct {
		tz   string
		args []string
		want string
	}{
		{"UTC0", []string{"-d", "@0", "+%H %Z %z"}, "00 UTC +0000\n"},
		{"EST5", []string{"-d", "@0", "+%Y-%m-%d %H:%M:%S %Z %z"}, "1969-12-31 19:00:00 EST -0500\n"},
		// Default C-locale format with TZ applied.
		{"EST5", []string{"-d", "@0"}, "Wed Dec 31 19:00:00 EST 1969\n"},
		// Full spec with rules: summer instant is DST, winter is not.
		{"EST5EDT,M3.2.0,M11.1.0", []string{"-d", "@1755000000", "+%Z %z"}, "EDT -0400\n"},
		{"EST5EDT,M3.2.0,M11.1.0", []string{"-d", "@1735689600", "+%Z %z"}, "EST -0500\n"},
		// Rules omitted: tzcode default US rules.
		{"EST5EDT", []string{"-d", "@1755000000", "+%Z %z"}, "EDT -0400\n"},
		// Quoted designation, sub-hour east-of-Greenwich offset.
		{"<+0530>-5:30", []string{"-d", "@0", "+%H:%M %Z %z"}, "05:30 +0530 +0530\n"},
		// An unusable nonempty TZ follows the shared resolver's UTC fallback.
		{"bogus", []string{"-d", "@0", "+%H %Z"}, "00 UTC\n"},
		// -u wins over TZ.
		{"EST5", []string{"-u", "-d", "@0", "+%H %Z"}, "00 UTC\n"},
		// TZ also governs how a zone-less -d string is interpreted.
		{"EST5", []string{"-d", "1970-01-01 00:00:00", "+%s"}, "18000\n"},
	}
	for _, c := range cases {
		out, errb, code := runToolEnv(t, []string{"TZ=" + c.tz}, c.args...)
		if code != 0 || errb != "" || out != c.want {
			t.Errorf("TZ=%q date %q = (%q, %q, %d), want %q", c.tz, c.args, out, errb, code, c.want)
		}
	}
	if got := dateLocation([]string{"TZ="}); got != time.Local {
		t.Fatalf("null TZ location=%v, want system default %v", got, time.Local)
	}
	if got := dateLocation(nil); got != time.Local {
		t.Fatalf("unset TZ location=%v, want system default %v", got, time.Local)
	}
}

func TestDateLCTimePrecedence(t *testing.T) {
	args := []string{"-u", "-d", "2026-03-06", "+%A %B"} // Friday in March.
	cases := []struct {
		name string
		env  []string
		want string
	}{
		{"lang", []string{"LANG=de_DE.UTF-8"}, "Freitag März\n"},
		{"lc-time", []string{"LANG=POSIX", "LC_TIME=de_DE.UTF-8"}, "Freitag März\n"},
		{"lc-all", []string{"LANG=POSIX", "LC_TIME=POSIX", "LC_ALL=de_DE.UTF-8"}, "Freitag März\n"},
		{"lc-all-overrides", []string{"LANG=de_DE.UTF-8", "LC_TIME=de_DE.UTF-8", "LC_ALL=POSIX"}, "Friday March\n"},
		{"empty-lc-all", []string{"LANG=POSIX", "LC_TIME=de_DE.UTF-8", "LC_ALL="}, "Freitag März\n"},
		{"empty-lc-time", []string{"LANG=de_DE.UTF-8", "LC_TIME="}, "Freitag März\n"},
		// The certification image selects this locale spelling. Pin all
		// three LC_TIME precedence inputs and its ISO-8859-1 output bytes,
		// rather than relying only on the UTF-8 spelling above.
		{"lang-latin1", []string{"LANG=de_DE.iso88591"}, "Freitag M\xe4rz\n"},
		{"lc-time-latin1", []string{"LANG=POSIX", "LC_TIME=de_DE.iso88591"}, "Freitag M\xe4rz\n"},
		{"lc-all-latin1", []string{"LANG=POSIX", "LC_TIME=POSIX", "LC_ALL=de_DE.iso88591"}, "Freitag M\xe4rz\n"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			out, errb, code := runToolEnv(t, tc.env, args...)
			if code != 0 || errb != "" || out != tc.want {
				t.Fatalf("date locale env=%q = (%q, %q, %d), want %q", tc.env, out, errb, code, tc.want)
			}
		})
	}
}

func TestDateLCTimeCompleteFormatsAndUnsupported(t *testing.T) {
	args := []string{"-u", "-d", "2026-03-06 13:45:09", "+%c|%x|%X|%r|%p|%h"}
	for _, tc := range []struct {
		name string
		env  []string
		want string
	}{
		{"C", []string{"LC_TIME=C"}, "Fri Mar  6 13:45:09 2026|03/06/26|13:45:09|01:45:09 PM|PM|Mar\n"},
		{"German UTF-8", []string{"LC_TIME=de_DE.UTF-8"}, "Fr 06 Mär 2026 13:45:09 UTC|06.03.2026|13:45:09|01:45:09 ||Mär\n"},
		{"German Latin-1", []string{"LC_TIME=de_DE.iso88591"}, "Fr 06 M\xe4r 2026 13:45:09 UTC|06.03.2026|13:45:09|01:45:09 ||M\xe4r\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, errOut, code := runToolEnv(t, tc.env, args...)
			if code != 0 || errOut != "" || out != tc.want {
				t.Fatalf("output=(%q,%q,%d), want %q", out, errOut, code, tc.want)
			}
		})
	}
	out, errOut, code := runToolEnv(t, []string{"LC_TIME=de_DE.UTF-8"}, "-u", "-d", "2026-03-06 13:45:09", "+%Ec|%Ex|%EX|%Od|%OH")
	if code != 0 || errOut != "" || out != "Fr 06 Mär 2026 13:45:09 UTC|06.03.2026|13:45:09|06|13\n" {
		t.Fatalf("German alternative forms=(%q,%q,%d)", out, errOut, code)
	}

	called := false
	_, errOut, code = runToolClock(t, []string{"LC_TIME=fr_FR.UTF-8"}, time.Unix(0, 0), func(time.Time) error { called = true; return nil }, "01010000")
	if code != 1 || called || !strings.Contains(errOut, "LC_TIME") {
		t.Fatalf("unsupported locale=(code %d, setter called %v, stderr %q)", code, called, errOut)
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
		{[]string{"-u", "-d", "@0", "--rfc-822"}, "Thu, 01 Jan 1970 00:00:00 +0000\n"},
		{[]string{"-u", "-d", "@0", "--rfc-2822"}, "Thu, 01 Jan 1970 00:00:00 +0000\n"},
		{[]string{"--uct", "-d", "@0", "+%H %Z"}, "00 UTC\n"},
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

func TestDateXSISetDateOperand(t *testing.T) {
	now := time.Date(2026, time.July, 8, 9, 10, 11, 0, time.UTC)
	tests := []struct {
		name      string
		env, args []string
		want      time.Time
		wantOut   string
	}{
		{"current year", []string{"TZ=UTC0"}, []string{"01020304"}, time.Date(2026, 1, 2, 3, 4, 0, 0, time.UTC), "Fri Jan  2 03:04:00 UTC 2026\n"},
		{"two digit 69", []string{"TZ=UTC0"}, []string{"0102030469"}, time.Date(1969, 1, 2, 3, 4, 0, 0, time.UTC), ""},
		{"two digit 68", []string{"TZ=UTC0"}, []string{"0102030468"}, time.Date(2068, 1, 2, 3, 4, 0, 0, time.UTC), ""},
		{"four digit year", []string{"TZ=UTC0"}, []string{"022923042024"}, time.Date(2024, 2, 29, 23, 4, 0, 0, time.UTC), ""},
		{"timezone controls wall clock", []string{"TZ=EST5"}, []string{"010203042026"}, time.Date(2026, 1, 2, 3, 4, 0, 0, time.FixedZone("EST", -5*3600)), "Fri Jan  2 03:04:00 EST 2026\n"},
		{"u overrides timezone", []string{"TZ=EST5"}, []string{"-u", "010203042026"}, time.Date(2026, 1, 2, 3, 4, 0, 0, time.UTC), "Fri Jan  2 03:04:00 UTC 2026\n"},
		{"localized result", []string{"TZ=UTC0", "LC_TIME=de_DE.UTF-8"}, []string{"030613452026"}, time.Date(2026, 3, 6, 13, 45, 0, 0, time.UTC), "Fr Mär  6 13:45:00 UTC 2026\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got []time.Time
			out, errOut, code := runToolClock(t, tt.env, now, func(at time.Time) error { got = append(got, at); return nil }, tt.args...)
			if code != 0 || errOut != "" || len(got) != 1 || !got[0].Equal(tt.want) {
				t.Fatalf("result=(out %q, err %q, code %d, calls %v), want one call %v", out, errOut, code, got, tt.want)
			}
			if out == "" {
				t.Fatal("successful set-date did not write the resulting date")
			}
			if tt.wantOut != "" && out != tt.wantOut {
				t.Fatalf("stdout=%q, want %q", out, tt.wantOut)
			}
		})
	}
}

func TestDateXSISetDateRejectsBeforeMutationAndPropagatesFailure(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	for _, args := range [][]string{{"not-a-date"}, {"13010000"}, {"023000002024"}, {"01012400"}, {"01010060"}, {"01010000", "+%Y"}, {"-d", "@0", "01010000"}, {"-I", "01010000"}, {"--debug", "01010000"}} {
		called := false
		out, errOut, code := runToolClock(t, []string{"TZ=UTC0", "LC_ALL=C"}, now, func(time.Time) error { called = true; return nil }, args...)
		if code == 0 || called || out != "" || errOut == "" {
			t.Errorf("date %q=(out %q, err %q, code %d, called %v), want pre-mutation failure", args, out, errOut, code, called)
		}
	}

	wantErr := os.ErrPermission
	out, errOut, code := runToolClock(t, []string{"TZ=UTC0", "LC_ALL=C"}, now, func(time.Time) error { return wantErr }, "01010000")
	if code != 1 || out != "" || !strings.Contains(errOut, "cannot set date") || !strings.Contains(errOut, wantErr.Error()) {
		t.Fatalf("setter failure=(%q,%q,%d)", out, errOut, code)
	}
}

func TestDateXSIYearDefaultUsesSelectedTimezone(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 30, 0, 0, time.UTC) // still 2025 in EST5
	for _, tc := range []struct {
		name string
		args []string
		year int
	}{
		{"TZ wall clock", []string{"01010000"}, 2025},
		{"u UTC wall clock", []string{"-u", "01010000"}, 2026},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var got time.Time
			_, errOut, code := runToolClock(t, []string{"TZ=EST5", "LC_ALL=C"}, now, func(at time.Time) error { got = at; return nil }, tc.args...)
			if code != 0 || errOut != "" || got.Year() != tc.year {
				t.Fatalf("result=(%v,%q,%d), want year %d", got, errOut, code, tc.year)
			}
		})
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

// VSC/PCTS GA39-style assertion: every invalid invocation writes a
// diagnostic to standard error and exits with nonzero status — never
// silent, never exit 0.
func TestDateInvalidUsageDiagnostics(t *testing.T) {
	cases := [][]string{
		{"-Q"},                         // unknown short option
		{"--frobnicate"},               // unknown long option
		{"--iso-8601=bogus"},           // invalid timespec argument
		{"--rfc-3339=bogus"},           // invalid timespec argument
		{"--rfc-3339"},                 // missing required argument
		{"-d", "not a date"},           // unparsable date string
		{"-f", "no-such-file"},         // unreadable date file
		{"-r", "no-such-file"},         // unreadable reference file
		{"+%Y", "+%m"},                 // extra operand
		{"13311030"},                   // invalid set-date operand fields
		{"--set", "@0"},                // set-date option (unsupported)
		{"-d", "@0", "-r", "x"},        // mutually exclusive date sources
		{"--resolution", "+%Y"},        // --resolution excludes formatting
		{"-I", "--rfc-email"},          // multiple output formats
		{"-d", "@0", "-d", "@1", "@2"}, // repeated flag then bad operand
	}
	for _, args := range cases {
		out, errb, code := runTool(t, args...)
		if code == 0 {
			t.Errorf("date %q: exit 0, want nonzero (out=%q err=%q)", args, out, errb)
		}
		if errb == "" {
			t.Errorf("date %q: no diagnostic on stderr (out=%q code=%d)", args, out, code)
		}
	}
}

func TestDateWriteErrorDiagnostic(t *testing.T) {
	var errb bytes.Buffer
	rc := &tool.RunContext{
		Ctx: context.Background(),
		Dir: t.TempDir(),
		Stdio: tool.Stdio{
			In:  strings.NewReader(""),
			Out: failingWriter{err: os.ErrClosed},
			Err: &errb,
		},
	}
	if code := cmd.Run(rc, []string{"-u", "-d", "@0", "+%s"}); code != 1 {
		t.Fatalf("write failure code=%d, want 1", code)
	}
	if got := errb.String(); !strings.Contains(got, "date: write error:") {
		t.Fatalf("write failure stderr=%q, want diagnostic", got)
	}
}

func TestDateHelp(t *testing.T) {
	out, _, code := runTool(t, "--help")
	if code != 0 || !strings.Contains(out, "Usage: date") {
		t.Errorf("--help: code=%d out=%q", code, out)
	}
	for _, hidden := range []string{"rfc-822", "rfc-2822", "uct"} {
		if strings.Contains(out, hidden) {
			t.Errorf("--help contains hidden alias %q in %q", hidden, out)
		}
	}
}
