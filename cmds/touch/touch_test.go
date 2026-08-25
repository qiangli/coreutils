package touchcmd

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

// runTool is the canonical test harness shape for cmds packages.
func runTool(t *testing.T, dir string, args ...string) (stdout, stderr string, code int) {
	return runToolEnv(t, dir, nil, args...)
}

func runToolEnv(t *testing.T, dir string, env []string, args ...string) (stdout, stderr string, code int) {
	t.Helper()
	var out, errb bytes.Buffer
	rc := &tool.RunContext{
		Ctx:   context.Background(),
		Dir:   dir,
		Env:   env,
		Stdio: tool.Stdio{In: strings.NewReader(""), Out: &out, Err: &errb},
	}
	code = cmd.Run(rc, args)
	return out.String(), errb.String(), code
}

func mtime(t *testing.T, path string) time.Time {
	t.Helper()
	fi, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat %s: %v", path, err)
	}
	return fi.ModTime()
}

func atime(t *testing.T, path string) time.Time {
	t.Helper()
	fi, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat %s: %v", path, err)
	}
	return statAtime(fi)
}

func TestTouchCreates(t *testing.T) {
	dir := t.TempDir()
	before := time.Now().Add(-2 * time.Second)
	_, errb, code := runTool(t, dir, "f.txt")
	if code != 0 || errb != "" {
		t.Fatalf("touch f.txt: code=%d err=%q", code, errb)
	}
	mt := mtime(t, filepath.Join(dir, "f.txt"))
	if mt.Before(before) || mt.After(time.Now().Add(2*time.Second)) {
		t.Errorf("mtime %v not near now", mt)
	}
}

func TestTouchStamp(t *testing.T) {
	cases := []struct {
		stamp string
		want  time.Time
	}{
		{"202001021504", time.Date(2020, 1, 2, 15, 4, 0, 0, time.Local)},
		{"199912312359.59", time.Date(1999, 12, 31, 23, 59, 59, 0, time.Local)},
		{"7001011200", time.Date(1970, 1, 1, 12, 0, 0, 0, time.Local)},
		{"6901011200", time.Date(1969, 1, 1, 12, 0, 0, 0, time.Local)},
		{"0101011200", time.Date(2001, 1, 1, 12, 0, 0, 0, time.Local)},
	}
	for _, c := range cases {
		dir := t.TempDir()
		_, errb, code := runTool(t, dir, "-t", c.stamp, "f")
		if code != 0 {
			t.Errorf("-t %s: code=%d err=%q", c.stamp, code, errb)
			continue
		}
		if got := mtime(t, filepath.Join(dir, "f")); got.Unix() != c.want.Unix() {
			t.Errorf("-t %s: mtime=%v want %v", c.stamp, got, c.want)
		}
	}
	// Attached value form.
	dir := t.TempDir()
	if _, _, code := runTool(t, dir, "-t202001021504", "f"); code != 0 {
		t.Errorf("-t202001021504: code=%d", code)
	}
}

func TestTouchStampRejectsOutsidePortableTimeRange(t *testing.T) {
	dir := t.TempDir()
	_, errb, code := runToolEnv(t, dir, []string{"TZ=UTC0"}, "-t", "190001010000.00", "-a", "-m", "example2.txt")
	if code == 0 || !strings.Contains(errb, "invalid date format") {
		t.Fatalf("pre-portable -t stamp: code=%d err=%q", code, errb)
	}
	if _, err := os.Stat(filepath.Join(dir, "example2.txt")); !os.IsNotExist(err) {
		t.Fatalf("invalid -t stamp created operand: %v", err)
	}
}

func TestTouchStampUsesInvocationTZAndCurrentYear(t *testing.T) {
	loc, err := time.LoadLocation("PST8PDT")
	if err != nil {
		t.Skipf("PST8PDT zoneinfo unavailable: %v", err)
	}
	dir := t.TempDir()
	year := time.Now().In(loc).Year()
	want := time.Date(year, time.January, 2, 3, 4, 5, 0, loc)

	_, errb, code := runToolEnv(t, dir, []string{"TZ=PST8PDT"}, "-t", "01020304.05", "f")
	if code != 0 {
		t.Fatalf("touch -t with TZ: code=%d err=%q", code, errb)
	}
	if got := mtime(t, filepath.Join(dir, "f")); got.Unix() != want.Unix() {
		t.Errorf("mtime=%v (%d), want %v (%d)", got, got.Unix(), want, want.Unix())
	}
}

func TestParseStampWithoutYearNeverRollsBack(t *testing.T) {
	loc := time.FixedZone("test", -8*60*60)
	now := time.Date(2026, time.January, 1, 0, 0, 0, 0, loc)
	got, err := parseStamp("12312359.58", now)
	if err != nil {
		t.Fatal(err)
	}
	want := time.Date(2026, time.December, 31, 23, 59, 58, 0, loc)
	if !got.Equal(want) {
		t.Fatalf("parseStamp omitted year = %v, want current year %v", got, want)
	}
}

func TestTouchStopsOptionParsingAtFirstOperand(t *testing.T) {
	dir := t.TempDir()
	_, errb, code := runTool(t, dir, "first", "-m", "--")
	if code != 0 {
		t.Fatalf("touch operands after first: code=%d err=%q", code, errb)
	}
	for _, name := range []string{"first", "-m", "--"} {
		if _, err := os.Stat(filepath.Join(dir, name)); err != nil {
			t.Errorf("operand %q was not created: %v", name, err)
		}
	}
}

func TestTouchDate(t *testing.T) {
	cases := []struct {
		date string
		want time.Time
	}{
		{"2020-01-02 03:04:05", time.Date(2020, 1, 2, 3, 4, 5, 0, time.Local)},
		{"2020-01-02T03:04:05", time.Date(2020, 1, 2, 3, 4, 5, 0, time.Local)},
		{"2020-01-02", time.Date(2020, 1, 2, 0, 0, 0, 0, time.Local)},
		{"@1577934245", time.Unix(1577934245, 0)},
	}
	for _, c := range cases {
		dir := t.TempDir()
		_, errb, code := runTool(t, dir, "-d", c.date, "f")
		if code != 0 {
			t.Errorf("-d %q: code=%d err=%q", c.date, code, errb)
			continue
		}
		if got := mtime(t, filepath.Join(dir, "f")); got.Unix() != c.want.Unix() {
			t.Errorf("-d %q: mtime=%v want %v", c.date, got, c.want)
		}
	}
}

func TestTouchDateISOSeconds60AndFractions(t *testing.T) {
	local := time.FixedZone("PST8", -8*60*60)
	cases := []struct {
		name string
		date string
		want time.Time
	}{
		{"local-space-seconds-60", "2026-01-02 13:17:60", time.Date(2026, 1, 2, 13, 18, 0, 0, local)},
		{"local-dot-fraction", "2026-01-02T13:17:60.987654321", time.Date(2026, 1, 2, 13, 18, 0, 987654321, local)},
		{"local-comma-fraction", "2026-01-02T13:17:60,987654321", time.Date(2026, 1, 2, 13, 18, 0, 987654321, local)},
		{"zulu-dot-fraction", "2026-01-02T13:17:55.987654321Z", time.Date(2026, 1, 2, 13, 17, 55, 987654321, time.UTC)},
		{"zulu-comma-fraction", "2026-01-02 13:17:55,987654321Z", time.Date(2026, 1, 2, 13, 17, 55, 987654321, time.UTC)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			_, errb, code := runToolEnv(t, dir, []string{"TZ=PST8"}, "-d", tc.date, "example.txt")
			if code != 0 {
				t.Fatalf("touch -d %q: code=%d err=%q", tc.date, code, errb)
			}
			got := mtime(t, filepath.Join(dir, "example.txt"))
			if !got.Equal(tc.want) || got.Nanosecond() != tc.want.Nanosecond() {
				t.Fatalf("touch -d %q mtime=%v, want %v", tc.date, got, tc.want)
			}
		})
	}
}

func TestTouchNoCreate(t *testing.T) {
	dir := t.TempDir()
	_, errb, code := runTool(t, dir, "-c", "missing")
	if code != 0 || errb != "" {
		t.Fatalf("-c missing: code=%d err=%q", code, errb)
	}
	if _, err := os.Stat(filepath.Join(dir, "missing")); !os.IsNotExist(err) {
		t.Error("-c created the file")
	}
}

func TestTouchReference(t *testing.T) {
	dir := t.TempDir()
	ref := filepath.Join(dir, "ref")
	if err := os.WriteFile(ref, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	want := time.Date(2011, 2, 3, 4, 5, 6, 0, time.Local)
	if err := os.Chtimes(ref, want, want); err != nil {
		t.Fatal(err)
	}
	_, errb, code := runTool(t, dir, "-r", "ref", "f")
	if code != 0 {
		t.Fatalf("-r: code=%d err=%q", code, errb)
	}
	if got := mtime(t, filepath.Join(dir, "f")); got.Unix() != want.Unix() {
		t.Errorf("mtime=%v want %v", got, want)
	}
}

func TestTouchAccessOnly(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "f")
	orig := time.Date(2010, 6, 7, 8, 9, 10, 0, time.Local)
	if err := os.WriteFile(f, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(f, orig, orig); err != nil {
		t.Fatal(err)
	}
	// -a must leave mtime untouched.
	if _, _, code := runTool(t, dir, "-a", "-t", "202001021504", "f"); code != 0 {
		t.Fatal("touch -a failed")
	}
	if got := mtime(t, f); got.Unix() != orig.Unix() {
		t.Errorf("-a changed mtime: %v", got)
	}
	// -m must change mtime.
	want := time.Date(2020, 1, 2, 15, 4, 0, 0, time.Local)
	if _, _, code := runTool(t, dir, "-m", "-t", "202001021504", "f"); code != 0 {
		t.Fatal("touch -m failed")
	}
	if got := mtime(t, f); got.Unix() != want.Unix() {
		t.Errorf("-m mtime=%v want %v", got, want)
	}
	// Combined cluster -am behaves like default.
	if _, _, code := runTool(t, dir, "-am", "-t", "202103040506", "f"); code != 0 {
		t.Fatal("touch -am failed")
	}
	want = time.Date(2021, 3, 4, 5, 6, 0, 0, time.Local)
	if got := mtime(t, f); got.Unix() != want.Unix() {
		t.Errorf("-am mtime=%v want %v", got, want)
	}
}

// With no -t/-d/-r, touch sets the current time. It must update both
// timestamps of an existing file, and it must do so via the current-time
// primitive (UTIME_NOW on unix) rather than an explicit stamp — the path
// separate from every -t-based case above.
func TestTouchCurrentTimeExisting(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "f")
	old := time.Date(2001, 2, 3, 4, 5, 6, 0, time.Local)
	if err := os.WriteFile(f, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(f, old, old); err != nil {
		t.Fatal(err)
	}
	before := time.Now().Add(-2 * time.Second)
	if _, errb, code := runTool(t, dir, "f"); code != 0 {
		t.Fatalf("touch f: code=%d err=%q", code, errb)
	}
	after := time.Now().Add(2 * time.Second)
	if got := mtime(t, f); got.Before(before) || got.After(after) {
		t.Errorf("mtime %v not near now", got)
	}
	if got := atime(t, f); got.Before(before) || got.After(after) {
		t.Errorf("atime %v not near now", got)
	}
}

// The current-time path must also honour -a/-m: -a moves only the access time
// to now and leaves the modification time; -m does the reverse. This exercises
// UTIME_NOW paired with UTIME_OMIT, distinct from the explicit-stamp -a/-m
// cases in TestTouchAccessOnly.
func TestTouchCurrentTimePartial(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "f")
	old := time.Date(2001, 2, 3, 4, 5, 6, 0, time.Local)
	if err := os.WriteFile(f, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(f, old, old); err != nil {
		t.Fatal(err)
	}
	before := time.Now().Add(-2 * time.Second)
	// -a: access moves to now, modification stays.
	if _, errb, code := runTool(t, dir, "-a", "f"); code != 0 {
		t.Fatalf("touch -a f: code=%d err=%q", code, errb)
	}
	if got := mtime(t, f); got.Unix() != old.Unix() {
		t.Errorf("-a changed mtime: got %v want %v", got, old)
	}
	if got := atime(t, f); got.Before(before) {
		t.Errorf("-a did not move atime to now: %v", got)
	}
	// -m: modification moves to now, access stays where -a left it.
	priorA := atime(t, f)
	if err := os.Chtimes(f, priorA, old); err != nil {
		t.Fatal(err)
	}
	before = time.Now().Add(-2 * time.Second)
	if _, errb, code := runTool(t, dir, "-m", "f"); code != 0 {
		t.Fatalf("touch -m f: code=%d err=%q", code, errb)
	}
	if got := mtime(t, f); got.Before(before) {
		t.Errorf("-m did not move mtime to now: %v", got)
	}
	if got := atime(t, f); got.Unix() != priorA.Unix() {
		t.Errorf("-m changed atime: got %v want %v", got, priorA)
	}
}

func TestTouchErrors(t *testing.T) {
	dir := t.TempDir()
	_, errb, code := runTool(t, dir)
	if code != 2 || !strings.Contains(errb, "missing file operand") {
		t.Errorf("no args: code=%d err=%q", code, errb)
	}
	_, errb, code = runTool(t, dir, "-t")
	if code != 2 || !strings.Contains(errb, "requires an argument") {
		t.Errorf("-t no value: code=%d err=%q", code, errb)
	}
	_, errb, code = runTool(t, dir, "-t", "bogus", "f")
	if code != 1 || !strings.Contains(errb, "invalid date format") {
		t.Errorf("-t bogus: code=%d err=%q", code, errb)
	}
	_, errb, code = runTool(t, dir, "-d", "not a date", "f")
	if code != 1 || !strings.Contains(errb, "invalid date format") {
		t.Errorf("-d bogus: code=%d err=%q", code, errb)
	}
	_, errb, code = runTool(t, dir, "-t", "202001021504", "-d", "2020-01-02", "f")
	if code != 2 || !strings.Contains(errb, "more than one source") {
		t.Errorf("-t with -d: code=%d err=%q", code, errb)
	}
	_, errb, code = runTool(t, dir, "--frobnicate", "f")
	if code != 2 || !strings.Contains(errb, "frobnicate") || !strings.Contains(errb, "pure-Go") {
		t.Errorf("unknown flag: code=%d err=%q", code, errb)
	}
}

func TestTouchHelpAndVersion(t *testing.T) {
	out, _, code := runTool(t, t.TempDir(), "--help")
	if code != 0 || !strings.Contains(out, "Usage: touch") {
		t.Errorf("--help: code=%d out=%q", code, out)
	}
	out, _, code = runTool(t, t.TempDir(), "--version")
	if code != 0 || !strings.Contains(out, "touch") {
		t.Errorf("--version: code=%d out=%q", code, out)
	}
}

// -h creates a missing operand as a regular file, and honours -c.
func TestTouchNoDerefMissingFile(t *testing.T) {
	dir := t.TempDir()
	if _, errb, code := runTool(t, dir, "-h", "new"); code != 0 {
		t.Fatalf("-h new: code=%d err=%q", code, errb)
	}
	if _, err := os.Stat(filepath.Join(dir, "new")); err != nil {
		t.Errorf("-h did not create the file: %v", err)
	}
	if _, errb, code := runTool(t, dir, "-h", "-c", "absent"); code != 0 || errb != "" {
		t.Errorf("-h -c absent: code=%d err=%q", code, errb)
	}
	if _, err := os.Stat(filepath.Join(dir, "absent")); !os.IsNotExist(err) {
		t.Error("-h -c created the file")
	}
}

// touch -d accepts GNU date strings beyond plain ISO timestamps: fractional
// epoch offsets, bare times of day, and relative items.
func TestTouchDateRelative(t *testing.T) {
	now := time.Date(2020, 6, 15, 12, 30, 45, 0, time.Local)
	midnight := func(t time.Time) time.Time {
		return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.Local)
	}
	cases := []struct {
		date string
		want time.Time
	}{
		{"now", now},
		{"today", now},
		{"yesterday", midnight(now.AddDate(0, 0, -1))},
		{"tomorrow", midnight(now.AddDate(0, 0, 1))},
		{"+1 hour", now.Add(time.Hour)},
		{"-1 hour", now.Add(-time.Hour)},
		{"2 hours ago", now.Add(-2 * time.Hour)},
		{"30 minutes", now.Add(30 * time.Minute)},
		{"+2 days", now.AddDate(0, 0, 2)},
		{"1 week ago", now.AddDate(0, 0, -7)},
		{"3 months", now.AddDate(0, 3, 0)},
		{"1 year ago", now.AddDate(-1, 0, 0)},
		{"+2days", now.AddDate(0, 0, 2)},
		{"1 hour 30 minutes ago", now.Add(-90 * time.Minute)},
	}
	for _, c := range cases {
		got, err := parseDate(c.date, now)
		if err != nil {
			t.Errorf("-d %q: unexpected error %v", c.date, err)
			continue
		}
		if !got.Equal(c.want) {
			t.Errorf("-d %q = %v, want %v", c.date, got, c.want)
		}
	}
}

func TestTouchDateAbsoluteForms(t *testing.T) {
	now := time.Date(2020, 6, 15, 12, 30, 45, 0, time.Local)
	cases := []struct {
		date string
		want time.Time
	}{
		{"@1577934245.5", time.Unix(1577934245, 500000000)},
		{"@0", time.Unix(0, 0)},
		{"@-1", time.Unix(-1, 0)},
		// A bare time of day is anchored to the current date.
		{"08:09", time.Date(2020, 6, 15, 8, 9, 0, 0, time.Local)},
		{"08:09:10", time.Date(2020, 6, 15, 8, 9, 10, 0, time.Local)},
		{"2020/01/02", time.Date(2020, 1, 2, 0, 0, 0, 0, time.Local)},
		{"2020-01-02 03:04:05 -0700", time.Date(2020, 1, 2, 10, 4, 5, 0, time.UTC)},
	}
	for _, c := range cases {
		got, err := parseDate(c.date, now)
		if err != nil {
			t.Errorf("-d %q: unexpected error %v", c.date, err)
			continue
		}
		if !got.Equal(c.want) {
			t.Errorf("-d %q = %v, want %v", c.date, got, c.want)
		}
	}
}

func TestTouchDateInvalid(t *testing.T) {
	now := time.Now()
	for _, bad := range []string{"", "  ", "not a date", "@", "@abc", "5 parsecs", "+3", "ago", "@1.x"} {
		if got, err := parseDate(bad, now); err == nil {
			t.Errorf("-d %q: want error, got %v", bad, got)
		}
	}
	// An empty -d value must be rejected, not silently treated as "no -d".
	dir := t.TempDir()
	_, errb, code := runTool(t, dir, "-d", "", "f")
	if code != 1 || !strings.Contains(errb, "invalid date format") {
		t.Errorf(`-d "": code=%d err=%q`, code, errb)
	}
}

// -d relative times reach the filesystem, not just the parser.
func TestTouchDateRelativeEndToEnd(t *testing.T) {
	dir := t.TempDir()
	if _, errb, code := runTool(t, dir, "-d", "1 day ago", "f"); code != 0 {
		t.Fatalf(`-d "1 day ago": code=%d err=%q`, code, errb)
	}
	got := mtime(t, filepath.Join(dir, "f"))
	want := time.Now().AddDate(0, 0, -1)
	if d := got.Sub(want); d > time.Minute || d < -time.Minute {
		t.Errorf("mtime=%v, want within a minute of %v", got, want)
	}
}

// TestTouchStampPOSIXTZ is the POSIX-TZ half of TZ handling: "PST8" is
// a pure POSIX expansion (std + offset), not a zoneinfo name, so
// time.LoadLocation cannot resolve it — yet GNU touch interprets -t in
// it via tzset. touch must route TZ through tzenv's POSIX handling
// rather than fall back to UTC (which would land the timestamp exactly
// eight hours early).
func TestTouchStampPOSIXTZ(t *testing.T) {
	dir := t.TempDir()
	// The certification case uses SS=60 as well as a pure POSIX TZ. POSIX
	// permits 60 and it normalizes to the first instant of the next minute.
	want := time.Date(2026, time.January, 2, 13, 18, 0, 0, time.FixedZone("PST", -8*60*60))
	_, errb, code := runToolEnv(t, dir, []string{"TZ=PST8"}, "-t", "202601021317.60", "f")
	if code != 0 {
		t.Fatalf("touch -t with TZ=PST8: code=%d err=%q", code, errb)
	}
	if got := mtime(t, filepath.Join(dir, "f")); got.Unix() != want.Unix() {
		t.Errorf("mtime=%v (%d), want %v (%d)", got, got.Unix(), want, want.Unix())
	}
}

// TestTouchDashIsAnOrdinaryPathname pins Issue 7's touch OPERANDS clause:
// "file  A pathname of a file whose times are to be modified." The
// specification gives "-" no special meaning for touch (unlike, e.g., the
// STDIN-reading utilities), so a "-" operand must create and stamp the file
// literally named "-" rather than being rejected or routed to a stream.
func TestTouchDashIsAnOrdinaryPathname(t *testing.T) {
	dir := t.TempDir()
	out, errb, code := runTool(t, dir, "-")
	if code != 0 || out != "" || errb != "" {
		t.Fatalf(`touch -: code=%d out=%q err=%q, want the file named "-" created`, code, out, errb)
	}
	if _, err := os.Stat(filepath.Join(dir, "-")); err != nil {
		t.Fatalf(`touch - did not create the file named "-": %v`, err)
	}

	// A "-" operand also accepts the required timestamp options, and is
	// stamped exactly like any other pathname.
	if _, errb, code := runTool(t, dir, "-t", "202001020304.05", "-"); code != 0 || errb != "" {
		t.Fatalf("touch -t ... -: code=%d err=%q", code, errb)
	}
	want := time.Date(2020, 1, 2, 3, 4, 5, 0, time.Local)
	if got := mtime(t, filepath.Join(dir, "-")); !got.Equal(want) {
		t.Errorf(`mtime of "-" = %v, want %v`, got, want)
	}

	// -c on a missing "-" must not create it, and must not fail.
	dir2 := t.TempDir()
	if _, errb, code := runTool(t, dir2, "-c", "-"); code != 0 || errb != "" {
		t.Fatalf("touch -c -: code=%d err=%q", code, errb)
	}
	if _, err := os.Stat(filepath.Join(dir2, "-")); !os.IsNotExist(err) {
		t.Errorf("touch -c - created the file: err=%v", err)
	}
}
