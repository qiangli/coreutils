package lncmd

import (
	"os"
	"path/filepath"
	"testing"
)

// TestChooseSymlinkFallback pins the Windows privilege fallback policy on
// every host (Story #682): only the privilege refusal on a regular-file
// source falls back, to a hard link on the same volume and a copy across
// volumes.
func TestChooseSymlinkFallback(t *testing.T) {
	cases := []struct {
		privilegeDenied, regular, sameVolume bool
		want                                 symlinkFallback
	}{
		{true, true, true, fallbackHardLink},
		{true, true, false, fallbackCopy},
		{true, false, true, fallbackNone},  // a directory source: no hard link, no copy
		{false, true, true, fallbackNone},  // any other error stands
		{false, true, false, fallbackNone}, // (destination exists, source missing, ...)
		{false, false, false, fallbackNone},
	}
	for _, c := range cases {
		if got := chooseSymlinkFallback(c.privilegeDenied, c.regular, c.sameVolume); got != c.want {
			t.Errorf("chooseSymlinkFallback(denied=%v, regular=%v, sameVolume=%v) = %v, want %v",
				c.privilegeDenied, c.regular, c.sameVolume, got, c.want)
		}
	}
}

func TestSameWindowsVolume(t *testing.T) {
	cases := []struct {
		a, b string
		want bool
	}{
		{`C:\Users\r\tmp\bash`, `C:\Users\r\AppData\bash.exe`, true},
		{`c:\x`, `C:/y`, true}, // case and separator insensitive
		{`C:\x`, `D:\y`, false},
		{`\\srv\share\a`, `\\srv\share\b\c`, true},
		{`//srv/share/a`, `\\SRV\SHARE\b`, true},
		{`\\srv\share\a`, `\\srv\other\a`, false},
		{`\\srv\share\a`, `C:\a`, false},
		{`\x\a`, `\y\b`, true},    // both drive-relative: the current drive
		{`\x\a`, `C:\y\b`, false}, // unknown current drive: assume different
		{`rel\a`, `rel\b`, true},
	}
	for _, c := range cases {
		if got := sameWindowsVolume(c.a, c.b); got != c.want {
			t.Errorf("sameWindowsVolume(%q, %q) = %v, want %v", c.a, c.b, got, c.want)
		}
	}
	vols := []struct{ in, want string }{
		{`C:\x`, "C:"},
		{`d:/x`, "d:"},
		{`\\srv\share\x`, `\\srv\share`},
		{`\\srv\share`, `\\srv\share`},
		{`//srv/share/`, `\\srv\share`},
		{`\\srv`, ""},
		{`\\srv\`, ""},
		{`\\\share`, ""},
		{`\x`, ""},
		{`x`, ""},
		{``, ""},
	}
	for _, c := range vols {
		if got := windowsVolume(c.in); got != c.want {
			t.Errorf("windowsVolume(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// TestCopyRegularFile: the last-resort fallback copies the bytes and never
// clobbers an existing destination.
func TestCopyRegularFile(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src")
	if err := os.WriteFile(src, []byte("#!/bin/sh\necho hi\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	dst := filepath.Join(dir, "dst")
	if err := copyRegularFile(src, dst); err != nil {
		t.Fatalf("copyRegularFile: %v", err)
	}
	got, err := os.ReadFile(dst)
	if err != nil || string(got) != "#!/bin/sh\necho hi\n" {
		t.Fatalf("copied content = %q, err %v", got, err)
	}
	if err := copyRegularFile(src, dst); err == nil {
		t.Fatal("copyRegularFile over an existing destination succeeded; want O_EXCL failure")
	}
	if err := copyRegularFile(filepath.Join(dir, "missing"), filepath.Join(dir, "dst2")); err == nil {
		t.Fatal("copyRegularFile of a missing source succeeded")
	}
	if _, err := os.Lstat(filepath.Join(dir, "dst2")); err == nil {
		t.Fatal("a failed copy left a destination behind")
	}
}
