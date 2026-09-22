//go:build windows

package tool

import (
	"testing"

	"mvdan.cc/sh/v3/pathconv"
)

// TestShellSpecialPathMapping covers the Windows entry points end to end:
// normalizePath (rc.Path's absolute case) and toOSPath (the localFS layer)
// both delegate to the shared mvdan.cc/sh/v3/pathconv converter — the SAME
// one bashy's sh interpreter runs — so a path names the same file inside and
// outside a script. Expected values follow the official converters'
// documented behavior, not prior art:
//
//	wslpath -w /mnt/c/Users -> C:\Users   (wslpath(1), WSL distro tool)
//	cygpath -w /c/Users     -> C:\Users   (cygpath(1); MSYS2 mounts drives at /c)
//	Cygwin/MSYS map /dev/null onto the NUL device, /tmp onto the temp
//	directory, and the characters NTFS refuses onto U+F000+c (Cygwin
//	User's Guide, "Mapping path names"; "Special characters in filenames").
//
// The mount-table cases (BASHY_ROOT) and the join are covered
// host-independently in path_shell_test.go; this file pins the tagged
// wrappers on a real Windows host. The legacy "/foo -> SystemDrive" mapping
// and the round-trip are covered by TestToOSPath/TestFromOSPath in
// tool_test.go.
func TestShellSpecialPathMapping(t *testing.T) {
	pinNoMounts(t)
	// Pin pathconv's temp-dir hook so the /tmp expectations are stable.
	oldTempDir := pathconv.TempDir
	pathconv.TempDir = func() string { return `C:\Users\me\AppData\Local\Temp` }
	defer func() { pathconv.TempDir = oldTempDir }()
	tmp := `C:\Users\me\AppData\Local\Temp`

	cases := []struct{ in, want string }{
		// Native drive spellings (already-Windows paths stay themselves).
		{`C:\Users\Lern`, `C:\Users\Lern`},
		{`C:/Users/Lern`, `C:\Users\Lern`},
		// MSYS/Git-Bash drive form (cygpath -w /c/… -> C:\…).
		{"/c/Users/Lern", `C:\Users\Lern`},
		{"/c", `C:\`},
		{"/d/foo/bar", `D:\foo\bar`},
		{"/C/Up", `C:\Up`}, // uppercase drive letter
		// The same form after filepath.FromSlash — how rmdir and mktemp's
		// join hand it on — must still mean the drive, not C:\c\....
		{`\c\Users\Lern`, `C:\Users\Lern`},
		{`\d\foo/bar`, `D:\foo\bar`},
		// WSL mount form (wslpath -w /mnt/c/… -> C:\…). Forward-slash
		// spelling only, exactly as pathconv defines it.
		{"/mnt/c/Users/Lern", `C:\Users\Lern`},
		{"/mnt/d", `D:\`},
		{"/mnt/D/foo", `D:\foo`}, // uppercase drive letter
		// POSIX pseudo-operands (Cygwin User's Guide: /dev/null -> NUL;
		// /tmp -> the temp directory).
		{"/dev/null", "NUL"},
		{"/tmp", tmp},
		{"/tmp/x.log", tmp + `\x.log`},
		// A trailing separator is semantic (RunContext.Path); it survives.
		{"/tmp/", tmp + `\`},
		{"/c/Users/", `C:\Users\`},
		// NTFS-forbidden characters are encoded the Cygwin/MSYS way.
		{"/tmp/x*x", tmp + "\\x\uf02ax"},
		{`C:\d\a:b`, "C:\\d\\a\uf03ab"},
	}
	for _, c := range cases {
		if got := normalizePath(c.in); got != c.want {
			t.Errorf("normalizePath(%q) = %q, want %q", c.in, got, c.want)
		}
		if got := toOSPath(c.in); got != c.want {
			t.Errorf("toOSPath(%q) = %q, want %q", c.in, got, c.want)
		}
	}

	// Boundary: near-miss spellings are NOT drive references or
	// pseudo-operands (wslpath and cygpath reject or pass these through
	// too). Without a mount table they are drive-less absolute paths, which
	// the interpreter lands on C: (normalizePath, no directory) or the
	// system drive (toOSPath). A native backslash spelling (\tmp\x) is
	// drive-relative in the same way.
	sd := systemDrive()
	boundary := []struct{ in, rest string }{
		{"/mnt", `mnt`}, // bare /mnt is not a drive
		{"/mnt/", `mnt\`},
		{"/mntx/foo", `mntx\foo`},
		{`\mnt\c\x`, `mnt\c\x`}, // WSL form is forward-slash only (pathconv)
		{"/tmpdir/x", `tmpdir\x`},
		{"/tmpx", `tmpx`},
		{`\tmp\x`, `tmp\x`},               // native backslash /tmp is drive-relative
		{"/dev/tcp/h/80", `dev\tcp\h\80`}, // only /dev/null is special
		{"/dev/nullx", `dev\nullx`},
		{"/foo/bar", `foo\bar`},
	}
	for _, c := range boundary {
		if got, want := normalizePath(c.in), `C:\`+c.rest; got != want {
			t.Errorf("normalizePath(%q) = %q, want %q", c.in, got, want)
		}
		if got, want := toOSPath(c.in), sd+c.rest; got != want {
			t.Errorf("toOSPath(%q) = %q, want %q", c.in, got, want)
		}
	}

	// Failure/passthrough: empty, relative and UNC operands convert
	// separators at most — never gain a drive.
	passthrough := []struct{ in, want string }{
		{"", ""},
		{"foo/bar", `foo\bar`},
		{"//server/share/path", `\\server\share\path`},
		{`\\.\pipe\sh-np-1`, `\\.\pipe\sh-np-1`},
	}
	for _, c := range passthrough {
		if got := normalizePath(c.in); got != c.want {
			t.Errorf("normalizePath(%q) = %q, want %q", c.in, got, c.want)
		}
		if got := toOSPath(c.in); got != c.want {
			t.Errorf("toOSPath(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// TestPathUnderMountsWindows drives RunContext.Path on a Windows host with
// the fixture's mount table installed: the shell-spelled operands and
// directory the bash-5.3 corpus produces must resolve into the private
// root and tmp (Story #682).
func TestPathUnderMountsWindows(t *testing.T) {
	old := pathconv.CurrentMounts()
	pathconv.SetMounts(fixtureMounts())
	t.Cleanup(func() { pathconv.SetMounts(old) })

	rc := &RunContext{Dir: "/tmp/bash-test-1"}
	cases := []struct{ in, want string }{
		{"/tmp/bash-test-1", fixtureTmp + `\bash-test-1`},
		{"/bin/sh", fixtureRoot + `\usr\bin\sh`},
		{"x", fixtureTmp + `\bash-test-1\x`},
		{"x*x", fixtureTmp + "\\bash-test-1\\x\uf02ax"},
		{"sub/", fixtureTmp + `\bash-test-1\sub\`},
	}
	for _, c := range cases {
		if got := rc.Path(c.in); got != c.want {
			t.Errorf("Path(%q) = %q, want %q", c.in, got, c.want)
		}
	}
	if got := rc.NativeDir(); got != fixtureTmp+`\bash-test-1` {
		t.Errorf("NativeDir() = %q", got)
	}
	if got := DisplayName("x\uf02ax"); got != "x*x" {
		t.Errorf("DisplayName = %q, want x*x", got)
	}
}
