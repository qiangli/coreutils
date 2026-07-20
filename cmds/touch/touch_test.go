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
	_, errb, code = runTool(t, dir, "-")
	if code != 2 || !strings.Contains(errb, "not supported") {
		t.Errorf("'-' operand: code=%d err=%q", code, errb)
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
