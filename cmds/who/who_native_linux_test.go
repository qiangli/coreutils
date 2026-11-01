//go:build linux

package whocmd

import (
	"bytes"
	"context"
	"crypto/sha256"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/qiangli/coreutils/tool"
)

// TestWhoNativeLinuxDifferential compiles the fixture generator against the
// target's real libc struct utmp. This is intentionally independent of every
// Go offset table in session and complements the hermetic byte fixtures. The
// differential is optional on minimal builders that do not carry a C compiler
// and a reference who(1).
func TestWhoNativeLinuxDifferential(t *testing.T) {
	cc, err := exec.LookPath("cc")
	if err != nil {
		t.Skip("native ABI probe requires cc")
	}
	dir := t.TempDir()
	reference := nativeGNUWho(t, dir)
	gen := filepath.Join(dir, "mkutmp")
	fixture := filepath.Join(dir, "utmp")
	if out, err := exec.Command(cc, "-Wall", "-Wextra", "-o", gen, "testdata/mkutmp_linux.c").CombinedOutput(); err != nil {
		t.Fatalf("compile native utmp generator: %v: %s", err, out)
	}
	if out, err := exec.Command(gen, fixture).CombinedOutput(); err != nil {
		t.Fatalf("generate native utmp: %v: %s", err, out)
	}

	invokeBashy := func(args ...string) (int, string, string) {
		var out, errb bytes.Buffer
		rc := &tool.RunContext{
			Ctx: context.Background(), Env: []string{"TZ=UTC", "LC_ALL=C"},
			Stdio: tool.Stdio{Out: &out, Err: &errb},
		}
		return run(rc, append(args, fixture)), out.String(), errb.String()
	}
	invokeReference := func(args ...string) string {
		cmd := exec.Command(reference, append(args, fixture)...)
		cmd.Env = append(os.Environ(), "TZ=UTC", "LC_ALL=C")
		out, err := cmd.Output()
		if err != nil {
			t.Fatalf("reference who %v: %v", args, err)
		}
		return string(out)
	}

	for _, args := range [][]string{{"-s", "-u"}, {"-T"}, {"-b"}, {"-r"}, {"-t"}} {
		code, got, errout := invokeBashy(args...)
		if code != 0 {
			t.Fatalf("bashy who %v: code=%d stderr=%q", args, code, errout)
		}
		want := invokeReference(args...)
		if strings.Fields(got) == nil || strings.Join(strings.Fields(got), " ") != strings.Join(strings.Fields(want), " ") {
			t.Fatalf("native differential %v:\n bashy=%q\n reference=%q", args, got, want)
		}
	}

	code, got, errout := invokeBashy("-T", "-l")
	if code != 0 {
		t.Fatalf("bashy who -T -l: code=%d stderr=%q", code, errout)
	}
	fields := strings.Fields(got)
	if strings.Join(fields, " ") != "LOGIN tty2 Nov 14 22:13" {
		t.Fatalf("LOGIN_PROCESS must have no state field: %q", got)
	}
	refFields := strings.Fields(invokeReference("-T", "-l"))
	if len(refFields) < len(fields) || strings.Join(refFields[:len(fields)], " ") != strings.Join(fields, " ") {
		t.Fatalf("native login prefix diverges: bashy=%q reference=%q", got, strings.Join(refFields, " "))
	}
}

// The differential measures GNU semantics, not whichever implementation a
// distribution assigns to /usr/bin/who. Ubuntu 26 also confines installed who
// paths with AppArmor, denying our explicit temporary utmp file and returning
// empty output. Run identical, recorded reference bytes in the private test
// directory; do not change system profiles or mistake denied reads for results.
func nativeGNUWho(t *testing.T, dir string) string {
	t.Helper()
	found := false
	for _, path := range []string{"/usr/bin/gnuwho", "/usr/bin/who"} {
		if _, err := os.Stat(path); os.IsNotExist(err) {
			continue
		}
		found = true
		version, err := exec.Command(path, "--version").Output()
		if err != nil {
			t.Fatalf("identify reference %s: %v", path, err)
		}
		if !strings.Contains(string(version), "(GNU coreutils)") {
			continue
		}
		binary, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read reference %s: %v", path, err)
		}
		reference := filepath.Join(dir, "gnu-who")
		if err := os.WriteFile(reference, binary, 0o755); err != nil {
			t.Fatal(err)
		}
		t.Logf("reference %s: %s; sha256=%x", path, strings.SplitN(string(version), "\n", 2)[0], sha256.Sum256(binary))
		return reference
	}
	if found {
		t.Fatal("native differential requires GNU who; installed reference providers are not GNU")
	}
	t.Skip("native differential requires GNU who")
	return ""
}
