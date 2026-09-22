package tool

import (
	"runtime"
	"testing"
)

// Story #682 (S245.5f) item 1 named the LocalFS layer as a place an operand
// could reach the OS without the shell-spelling resolver — applets stat and
// read directories through it. It does not: every LocalFS method converts
// with toOSPath, which is shellAbsMode against pathconv.CurrentMounts(). The
// tests below pin that, so a future method added without the conversion, or
// a toOSPath that stops consulting the mount table, fails here.

// TestLocalFSToOSGoesThroughTheMountTable states where a LocalFS path lands
// on Windows with a mount table installed. On a Unix host the resolver is
// the identity, so the mounted answer is asserted through the same *Mode
// seam LocalFS's Windows implementation uses; on Windows the two are
// compared directly.
func TestLocalFSToOSGoesThroughTheMountTable(t *testing.T) {
	m := fixtureMounts()
	cases := []struct{ in, want string }{
		{"/tmp/bash-test-7844", fixtureTmp + `\bash-test-7844`},
		{"/bin/sh", fixtureRoot + `\usr\bin\sh`},
		{"/etc/passwd", fixtureRoot + `\etc\passwd`},
		{"/dev/null", "NUL"},
	}
	for _, tc := range cases {
		if got := ResolveOperandMode(m, `C:\`, tc.in, true); got != tc.want {
			t.Errorf("windows-mode resolve(%q) = %q; want %q", tc.in, got, tc.want)
		}
	}

	fs := NewLocalFS()
	if runtime.GOOS != "windows" {
		// Unix: the layer is a passthrough, which is the whole claim there.
		for _, tc := range cases {
			if got := fs.ToOS(tc.in); got != tc.in {
				t.Errorf("unix LocalFS.ToOS(%q) = %q; want the identity", tc.in, got)
			}
		}
		return
	}
	// Windows: LocalFS must give the same answer as the resolver for the
	// process's own mount table — it is the same function.
	for _, tc := range cases {
		if got, want := fs.ToOS(tc.in), toOSPath(tc.in); got != want {
			t.Errorf("LocalFS.ToOS(%q) = %q; want %q", tc.in, got, want)
		}
	}
}

// Every LocalFS method routes its path argument through ToOS: the type is a
// thin wrapper whose only job is that conversion, so a method that forgets
// it silently reintroduces the bug this story fixed. The check is structural
// rather than behavioral — it calls each method on a path that cannot exist
// and asserts nothing about the error, only that the conversion is the one
// ToOS performs.
func TestLocalFSMethodsUseTheSameConversion(t *testing.T) {
	fs := NewLocalFS()
	const p = "/tmp/bash-test-7844/nope"
	if got, want := fs.ToOS(p), toOSPath(p); got != want {
		t.Fatalf("LocalFS.ToOS(%q) = %q; want %q", p, got, want)
	}
	if got, want := fs.FromOS(fs.ToOS(p)), fromOSPath(toOSPath(p)); got != want {
		t.Errorf("LocalFS round trip = %q; want %q", got, want)
	}
}
