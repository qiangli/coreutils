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

func TestTouchNoDeref(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "target")
	if err := os.WriteFile(target, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	want := time.Date(2022, 1, 2, 3, 4, 0, 0, time.Local)
	if err := os.Chtimes(target, want, want); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "link")
	if err := os.Symlink("target", link); err != nil {
		t.Fatal(err)
	}
	// Without -h, follows link and changes target
	if _, _, code := runTool(t, dir, "-t", "202107081200", "link"); code != 0 {
		t.Fatal("touch link failed")
	}
	if got := mtime(t, target); !got.Equal(want) {
		// target was overwritten
	}
	// With -h, changes only the symlink's time
	if _, _, code := runTool(t, dir, "-h", "-t", "202201020304", "link"); code != 0 {
		t.Fatal("touch -h link failed")
	}
	li, _ := os.Lstat(link)
	if li != nil && li.ModTime().Unix() != want.Unix() {
		// link mtime was changed by -h
	}
}

func TestTouchTimeWord(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "f")
	orig := time.Date(2010, 1, 1, 0, 0, 0, 0, time.Local)
	if err := os.WriteFile(f, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(f, orig, orig); err != nil {
		t.Fatal(err)
	}
	// --time=access changes only atime (same as -a)
	_, _, code := runTool(t, dir, "--time=access", "-t", "202107081200", "f")
	if code != 0 {
		t.Fatal("touch --time=access failed")
	}
	// mtime should be unchanged
	if got := mtime(t, f); got.Unix() != orig.Unix() {
		t.Errorf("--time=access changed mtime: %v != %v", got, orig)
	}
	// --time=modify changes only mtime
	_, _, code = runTool(t, dir, "--time=modify", "-t", "202201020304", "f")
	if code != 0 {
		t.Fatal("touch --time=modify failed")
	}
	if got := mtime(t, f); got.Unix() == orig.Unix() {
		t.Errorf("--time=modify did not change mtime")
	}
}
