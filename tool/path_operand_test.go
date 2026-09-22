package tool

import "testing"

// TestOperandJoinMode is the Story #682 regression for the cp/mv/ln
// destination: on the Windows runner `cp $THIS_SH /tmp/execdir-4440` built
// its destination with filepath.Join and produced \tmp\execdir-4440\bash.exe,
// which no longer matches the /tmp mount (pathconv's lookup is defined on the
// POSIX spelling) and failed with "The system cannot find the path
// specified".
func TestOperandJoinMode(t *testing.T) {
	cases := []struct {
		base string
		elem string
		want string
	}{
		{"/tmp/execdir-4440", "bash.exe", "/tmp/execdir-4440/bash.exe"},
		{"/tmp", "d", "/tmp/d"},
		{"/tmp/", "d", "/tmp/d"},
		{"/", "d", "/d"},
		{"d", "x", "d/x"},
		{"./d", "x", "d/x"}, // path.Join cleans, as filepath.Join does
		{"", "x", "x"},
		// A native-spelled base keeps the native join: it is past the mount
		// layer already.
		{`C:\x`, "y", `C:\x\y`},
		{`\tmp\d`, "y", `\tmp\d\y`},
	}
	for _, tc := range cases {
		if got := operandJoinMode(tc.base, []string{tc.elem}, true); got != tc.want {
			t.Errorf("operandJoinMode(%q, %q, windows) = %q; want %q", tc.base, tc.elem, got, tc.want)
		}
	}
}

// TestOperandDirMode is the same regression for mkdir -p, which walks its
// operand upward one component at a time: filepath.Dir("/tmp/empty/a/a/a")
// returned \tmp\empty\a\a on the runner, so every ancestor after the first
// was created under C:\tmp instead of the mounted temp directory
// ("mkdir: cannot create directory '/tmp/empty/a/a/a'").
func TestOperandDirMode(t *testing.T) {
	cases := []struct{ in, want string }{
		{"/tmp/empty/a/a/a", "/tmp/empty/a/a"},
		{"/tmp/empty", "/tmp"},
		{"/tmp", "/"},
		{"/", "/"},
		{"a/b", "a"},
		{"a", "."},
		{`C:\x\y`, `C:\x`},
	}
	for _, tc := range cases {
		if got := operandDirMode(tc.in, true); got != tc.want {
			t.Errorf("operandDirMode(%q, windows) = %q; want %q", tc.in, got, tc.want)
		}
	}
}

// Off windows mode both helpers are exactly filepath.Join/filepath.Dir, so
// no Unix behavior moves.
func TestOperandHelpersUnixIdentity(t *testing.T) {
	if got := operandJoinMode("a/b", []string{"..", "c"}, false); got != "a/c" {
		t.Errorf("operandJoinMode unix = %q; want a/c", got)
	}
	if got := operandDirMode("a/b/c", false); got != "a/b" {
		t.Errorf("operandDirMode unix = %q; want a/b", got)
	}
}

// posixSpelledMode is the discriminator: anything carrying a backslash or a
// drive prefix has left the shell's spelling and must not be re-cleaned as a
// POSIX path.
func TestPosixSpelledMode(t *testing.T) {
	posix := []string{"/tmp/x", "a/b", "", "/", "x*x"}
	native := []string{`\tmp\x`, `C:\x`, "C:/x", `a\b`}
	for _, p := range posix {
		if !posixSpelledMode(p, true) {
			t.Errorf("posixSpelledMode(%q) = false; want true", p)
		}
	}
	for _, p := range native {
		if posixSpelledMode(p, true) {
			t.Errorf("posixSpelledMode(%q) = true; want false", p)
		}
		// Off windows mode every operand is POSIX-spelled.
		if !posixSpelledMode(p, false) {
			t.Errorf("posixSpelledMode(%q, unix) = false; want true", p)
		}
	}
}

// The point of keeping the POSIX spelling is that the joined operand still
// resolves through the mount table. This is the end-to-end statement of the
// runner failure, on any host.
func TestOperandJoinStillResolvesThroughMounts(t *testing.T) {
	m := fixtureMounts()
	dst := operandJoinMode("/tmp/execdir-4440", []string{"bash.exe"}, true)
	want := fixtureTmp + `\execdir-4440\bash.exe`
	if got := shellAbsMode(m, "", dst, true); got != want {
		t.Errorf("resolved %q = %q; want %q", dst, got, want)
	}
	// The pre-fix spelling is what the runner actually tried: the mount
	// lookup is defined on the POSIX spelling, so \tmp\... falls through to
	// the drive-relative rule and lands on C:. Kept as the negative half of
	// the regression.
	if got := shellAbsMode(m, "", `\tmp\execdir-4440\bash.exe`, true); got == want {
		t.Fatalf("native-spelled operand resolved through the mount: %q", got)
	} else if got != `C:\tmp\execdir-4440\bash.exe` {
		t.Errorf("native-spelled operand = %q; want the drive-relative fallback", got)
	}
}
