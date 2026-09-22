package tool

import (
	"path/filepath"
	"runtime"
	"testing"

	"mvdan.cc/sh/v3/pathconv"
)

// fixtureMounts is the mount table the Windows bash-5.3 fixture run
// installs (BASHY_ROOT = a private root, TEMP = a private tmp beside it):
// the applets under test resolve /tmp, /bin, /usr/bin and /etc through it
// exactly as the shell does.
func fixtureMounts() *pathconv.Mounts {
	return pathconv.NewMounts(`C:\Users\r\AppData\Local\Temp\bash53-1\root`, nil, `C:\Users\r\AppData\Local\Temp\bash53-1\tmp`)
}

// pinNoMounts runs t with no process mount table, so a Windows expectation
// does not depend on whether the host shell exported BASHY_ROOT. Off Windows
// pathconv never has a table; the pin is a no-op there.
func pinNoMounts(t *testing.T) {
	t.Helper()
	old := pathconv.CurrentMounts()
	pathconv.SetMounts(nil)
	t.Cleanup(func() { pathconv.SetMounts(old) })
}

const (
	fixtureRoot = `C:\Users\r\AppData\Local\Temp\bash53-1\root`
	fixtureTmp  = `C:\Users\r\AppData\Local\Temp\bash53-1\tmp`
)

// TestShellAbsModeMounts is the Story #682 regression for the Windows path
// layer: with BASHY_ROOT mounts an absolute operand in the shell's spelling
// must land where the interpreter's own pathconv.ToOS puts it. Before,
// /tmp/bash-test-N became C:\tmp\bash-test-N ("The system cannot find the
// path specified" in array, comsub2, extglob, ifs) and cp /bin/sh could not
// stat its source (rsh).
func TestShellAbsModeMounts(t *testing.T) {
	m := fixtureMounts()
	cases := []struct{ in, want string }{
		{"/tmp/bash-test-7844", fixtureTmp + `\bash-test-7844`},
		{"/tmp", fixtureTmp},
		{"/tmp/", fixtureTmp + `\`}, // trailing separator is semantic
		{"/bin/sh", fixtureRoot + `\usr\bin\sh`},
		{"/usr/bin/printf", fixtureRoot + `\usr\bin\printf`},
		{"/etc/passwd", fixtureRoot + `\etc\passwd`},
		{"/", fixtureRoot + `\`},               // "/" ends in a separator; it is kept
		{"/foo/bar", fixtureRoot + `\foo\bar`}, // every other /x is under the root
		{"/dev/null", "NUL"},
		// The drive forms beat the root mount, as they do in the shell.
		{"/c/Users/x", `C:\Users\x`},
		{"/mnt/d/x", `D:\x`},
		{`C:\already\native`, `C:\already\native`},
		{`C:/fwd/slash`, `C:\fwd\slash`},
		// Device and UNC paths are native already.
		{`\\.\pipe\sh-np-1`, `\\.\pipe\sh-np-1`},
		{"//server/share/p", `\\server\share\p`},
		{"", ""},
	}
	for _, c := range cases {
		if got := shellAbsMode(m, "", c.in, true); got != c.want {
			t.Errorf("shellAbsMode(mounts, %q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// TestShellAbsModeNoMounts pins the plain drive rules an applet applies
// when no BASHY_ROOT is set: /tmp is the host temp directory, a drive-less
// /foo lands on the invocation directory's volume (C: when there is none).
func TestShellAbsModeNoMounts(t *testing.T) {
	old := pathconv.TempDir
	pathconv.TempDir = func() string { return `C:\Users\me\AppData\Local\Temp` }
	defer func() { pathconv.TempDir = old }()
	tmp := `C:\Users\me\AppData\Local\Temp`

	cases := []struct{ dir, in, want string }{
		{"", "/tmp/x.log", tmp + `\x.log`},
		{"", "/tmp/", tmp + `\`},
		{"", "/dev/null", "NUL"},
		{"", "/c/Users/Lern", `C:\Users\Lern`},
		{"", "/c", `C:\`},
		{"", `\c\Users\Lern`, `C:\Users\Lern`}, // the drive form after FromSlash
		{"", "/mnt/d", `D:\`},
		{"", "/foo/bar", `C:\foo\bar`},
		{`D:\work`, "/foo/bar", `D:\foo\bar`}, // drive-relative: the cwd's volume
		{"/d/work", "/foo/bar", `D:\foo\bar`}, // even when the cwd is shell-spelled
		{"", "/tmpx", `C:\tmpx`},              // near-miss: not the temp dir
		{"", "/dev/nullx", `C:\dev\nullx`},
		// Relative with nothing to join onto: separators and encoding only.
		{"", "foo/bar", `foo\bar`},
		{"", "x*x", "x\uf02ax"},
	}
	for _, c := range cases {
		if got := shellAbsMode(nil, c.dir, c.in, true); got != c.want {
			t.Errorf("shellAbsMode(nil, dir=%q, %q) = %q, want %q", c.dir, c.in, got, c.want)
		}
	}
}

// TestShellAbsModeSpecialChars: the characters NTFS refuses in a filename
// are encoded as U+F000+c (the Cygwin/MSYS convention the shell uses for
// its own redirections), so `touch 'x*x'` no longer fails with "The
// filename, directory name, or volume label syntax is incorrect" (heredoc,
// extglob) — and DisplayName brings the name back.
func TestShellAbsModeSpecialChars(t *testing.T) {
	m := fixtureMounts()
	cases := []struct{ in, want string }{
		{"/tmp/x*x", fixtureTmp + "\\x\uf02ax"},
		{"/tmp/d/a:b", fixtureTmp + "\\d\\a\uf03ab"},
		{"/tmp/q?", fixtureTmp + "\\q\uf03f"},
		{`/tmp/"<>|`, fixtureTmp + "\\\uf022\uf03c\uf03e\uf07c"},
		{`C:\d\a:b`, "C:\\d\\a\uf03ab"}, // the drive colon is kept
	}
	for _, c := range cases {
		got := shellAbsMode(m, "", c.in, true)
		if got != c.want {
			t.Errorf("shellAbsMode(%q) = %q, want %q", c.in, got, c.want)
		}
	}
	for _, name := range []string{"x*x", "a:b", `"<>|?`, "plain", ""} {
		enc := pathconv.EncodeSpecialMode(name, true)
		if got := displayNameMode(enc, true); got != name {
			t.Errorf("displayNameMode(%q) = %q, want %q", enc, got, name)
		}
	}
	if got := displayNameMode("x\uf02ax", false); got != "x\uf02ax" {
		t.Errorf("displayNameMode off windows mode altered %q", got)
	}
}

// TestShellJoinMode: a relative operand joins onto the invocation
// directory AFTER the directory is converted — bashy's in-process applets
// receive the interpreter's cwd in the shell's spelling, and a raw
// filepath.Join produced \tmp\bash-test-N\x, which CreateFile resolved as
// C:\tmp\bash-test-N\x.
func TestShellJoinMode(t *testing.T) {
	m := fixtureMounts()
	cases := []struct{ dir, in, want string }{
		{"/tmp/bash-test-7844", "x", fixtureTmp + `\bash-test-7844\x`},
		{"/tmp/bash-test-7844", "sub/y", fixtureTmp + `\bash-test-7844\sub\y`},
		{"/tmp/eglob-test-1", "x*x", fixtureTmp + "\\eglob-test-1\\x\uf02ax"},
		{"/c/Users/me", "f", `C:\Users\me\f`},
		{fixtureRoot + `\etc`, "passwd", fixtureRoot + `\etc\passwd`},
		{`C:\work\`, "f", `C:\work\f`},
		{"/", "etc", fixtureRoot + `\etc`},
	}
	for _, c := range cases {
		if got := shellJoinMode(m, c.dir, c.in, true); got != c.want {
			t.Errorf("shellJoinMode(dir=%q, %q) = %q, want %q", c.dir, c.in, got, c.want)
		}
	}
	if got := shellJoinMode(nil, "/work", "a/b", false); got != "/work/a/b" {
		t.Errorf("shellJoinMode off windows mode = %q, want /work/a/b", got)
	}
}

// TestShellDirMode: the directory conversion the join and NativeDir rely
// on.
func TestShellDirMode(t *testing.T) {
	m := fixtureMounts()
	cases := []struct{ in, want string }{
		{"", ""},
		{"/tmp/bash-test-1", fixtureTmp + `\bash-test-1`},
		{"/c/Users/me", `C:\Users\me`},
		{`C:\Users\me`, `C:\Users\me`},
	}
	for _, c := range cases {
		if got := shellDirMode(m, c.in, true); got != c.want {
			t.Errorf("shellDirMode(%q) = %q, want %q", c.in, got, c.want)
		}
	}
	if got := shellDirMode(m, "/tmp/x", false); got != "/tmp/x" {
		t.Errorf("shellDirMode off windows mode = %q", got)
	}
}

// TestRawPath: the no-Clean resolver rmdir relies on keeps every component
// the caller spelled and, on Windows, converts only the directory and the
// NTFS specials.
func TestRawPath(t *testing.T) {
	pinNoMounts(t)
	root := testRoot()
	rc := &RunContext{Dir: root + "work"}
	sep := string(filepath.Separator)
	cases := []struct{ in, want string }{
		{"f/..", root + "work" + sep + "f" + sep + ".."},
		{"missing/.", root + "work" + sep + "missing" + sep + "."},
		{"d/", root + "work" + sep + "d" + sep},
	}
	for _, c := range cases {
		if got := rc.RawPath(c.in); got != c.want {
			t.Errorf("RawPath(%q) = %q, want %q", c.in, got, c.want)
		}
	}
	if got, want := rc.RawPath(root+"abs"), root+"abs"; got != want {
		t.Errorf("RawPath(abs) = %q, want %q", got, want)
	}
	noDir := &RunContext{}
	if got := noDir.RawPath("x"); got != "x" {
		t.Errorf("RawPath without Dir = %q, want x", got)
	}
	if got := rc.NativeDir(); got != root+"work" {
		t.Errorf("NativeDir() = %q, want %q", got, root+"work")
	}
	if runtime.GOOS == "windows" {
		if got, want := rc.RawPath("x*x"), root+"work\\x\uf02ax"; got != want {
			t.Errorf("RawPath(x*x) = %q, want %q", got, want)
		}
		shell := &RunContext{Dir: "/c/work"}
		if got, want := shell.RawPath("f/.."), `C:\work\f\..`; got != want {
			t.Errorf("RawPath under a shell-spelled Dir = %q, want %q", got, want)
		}
	}
}
