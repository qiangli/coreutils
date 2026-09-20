//go:build windows

package tool

import (
	"testing"

	"mvdan.cc/sh/v3/pathconv"
)

// TestShellSpecialPathMapping covers the shell-spelling conversions the tool
// entry points now take from the shared mvdan.cc/sh/v3/pathconv converter:
// the MSYS/Git-Bash drive form, the WSL mount form, and the /dev/null and
// /tmp pseudo-operands. Expected values are derived from the official
// converters' documented behavior, not from prior art:
//
//	wslpath -w /mnt/c/Users -> C:\Users   (wslpath(1), WSL distro tool)
//	cygpath -w /c/Users     -> C:\Users   (cygpath(1); MSYS2 mounts drives at /c)
//	Cygwin/MSYS map /dev/null onto the NUL device and /tmp onto the
//	temp directory (Cygwin User's Guide, "Mapping path names").
//
// Both the rc.Path entry (normalizePath) and the localFS layer (toOSPath)
// must agree on every one of these; they diverge only on a bare drive-less
// "/foo", covered at the bottom. The legacy "/foo -> SystemDrive" mapping
// and the round-trip are covered by TestToOSPath/TestFromOSPath in
// tool_test.go.
func TestShellSpecialPathMapping(t *testing.T) {
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
	// too): rc.Path keeps them drive-relative, exactly as before.
	boundary := []struct{ in, want string }{
		{"/mnt", `\mnt`},   // bare /mnt is not a drive
		{"/mnt/", `\mnt\`}, // /mnt/ with no letter
		{"/mntx/foo", `\mntx\foo`},
		{`\mnt\c\x`, `\mnt\c\x`}, // WSL form is forward-slash only (pathconv)
		{"/tmpdir/x", `\tmpdir\x`},
		{"/tmpx", `\tmpx`},
		{`\tmp\x`, `\tmp\x`},               // native backslash /tmp is drive-relative
		{"/dev/tcp/h/80", `\dev\tcp\h\80`}, // only /dev/null is special
		{"/dev/nullx", `\dev\nullx`},
		{"/foo/bar", `\foo\bar`},
	}
	for _, c := range boundary {
		if got := normalizePath(c.in); got != c.want {
			t.Errorf("normalizePath(%q) = %q, want %q", c.in, got, c.want)
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
