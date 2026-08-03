//go:build unix

package testcmd

import (
	"bytes"
	"context"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/sys/unix"

	"github.com/qiangli/coreutils/tool"
)

// The primaries covered here need a POSIX permission model, POSIX
// uid/gid, or file types only unix creates. Windows answers them under
// its own rules (or fails loudly for ownership), so these cases are
// unix-only rather than skipped at runtime.

func TestPermissionPrimaries(t *testing.T) {
	if os.Geteuid() == 0 {
		// root bypasses the permission bits entirely, so -r/-w/-x would
		// be true for every mode and prove nothing.
		t.Skip("running as root: permission bits are not enforced")
	}
	dir := t.TempDir()

	cases := []struct {
		mode os.FileMode
		r    int
		w    int
		x    int
	}{
		{0o000, statusFalse, statusFalse, statusFalse},
		{0o400, statusTrue, statusFalse, statusFalse},
		{0o200, statusFalse, statusTrue, statusFalse},
		{0o100, statusFalse, statusFalse, statusTrue},
		{0o600, statusTrue, statusTrue, statusFalse},
		{0o700, statusTrue, statusTrue, statusTrue},
	}
	for _, c := range cases {
		name := "f" + c.mode.String()
		path := filepath.Join(dir, name)
		mustWrite(t, path, "x")
		if err := os.Chmod(path, c.mode); err != nil {
			t.Fatal(err)
		}
		for _, probe := range []struct {
			op   string
			want int
		}{{"-r", c.r}, {"-w", c.w}, {"-x", c.x}} {
			_, errb, code := runIn(t, cmd, dir, probe.op, name)
			if code != probe.want || errb != "" {
				t.Errorf("mode %04o: test %s = (%q, %d), want %d", c.mode, probe.op, errb, code, probe.want)
			}
		}
	}

	// A file that does not exist grants no permission at all.
	for _, op := range []string{"-r", "-w", "-x"} {
		if _, _, code := runIn(t, cmd, dir, op, "missing"); code != statusFalse {
			t.Errorf("test %s missing = %d, want 1", op, code)
		}
	}

	// A searchable directory is executable.
	if _, _, code := runIn(t, cmd, dir, "-x", "."); code != statusTrue {
		t.Error("-x on a searchable directory = 1, want 0")
	}
}

func TestOwnershipPrimaries(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "f")
	mustWrite(t, path, "x")

	// -O is the effective uid, which is by definition the creator's.
	if _, errb, code := runIn(t, cmd, dir, "-O", "f"); code != statusTrue || errb != "" {
		t.Errorf("-O on a file we created = (%q, %d), want 0", errb, code)
	}

	// -G is the effective gid. The group is inherited from the parent
	// directory on some systems (BSD semantics, or a setgid dir), so the
	// expected answer is computed rather than assumed.
	var st unix.Stat_t
	if err := unix.Stat(path, &st); err != nil {
		t.Fatal(err)
	}
	want := statusFalse
	if int(st.Gid) == os.Getegid() {
		want = statusTrue
	}
	if _, errb, code := runIn(t, cmd, dir, "-G", "f"); code != want || errb != "" {
		t.Errorf("-G = (%q, %d), want %d for gid %d vs egid %d", errb, code, want, st.Gid, os.Getegid())
	}

	// A file that does not exist is owned by nobody — false, not an error.
	for _, op := range []string{"-O", "-G"} {
		if _, errb, code := runIn(t, cmd, dir, op, "missing"); code != statusFalse || errb != "" {
			t.Errorf("test %s missing = (%q, %d), want (\"\", 1)", op, errb, code)
		}
	}
}

func TestSetuidSetgidStickyPrimaries(t *testing.T) {
	dir := t.TempDir()

	setuid := filepath.Join(dir, "setuid")
	mustWrite(t, setuid, "x")
	setgid := filepath.Join(dir, "setgid")
	mustWrite(t, setgid, "x")
	sticky := filepath.Join(dir, "sticky")
	if err := os.Mkdir(sticky, 0o755); err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		name string
		path string
		perm os.FileMode
		bit  os.FileMode
		op   string
	}{
		{"setuid", setuid, 0o644, os.ModeSetuid, "-u"},
		{"setgid", setgid, 0o644, os.ModeSetgid, "-g"},
		{"sticky", sticky, 0o755, os.ModeSticky, "-k"},
	}
	for _, c := range cases {
		// The bit is false before it is set.
		if _, _, code := runIn(t, cmd, dir, c.op, c.name); code != statusFalse {
			t.Errorf("test %s %s before chmod = %d, want 1", c.op, c.name, code)
		}
		if err := os.Chmod(c.path, c.perm|c.bit); err != nil {
			t.Logf("cannot set %s on %s: %v", c.op, c.name, err)
			continue
		}
		fi, err := os.Stat(c.path)
		if err != nil {
			t.Fatal(err)
		}
		if fi.Mode()&c.bit == 0 {
			// Some filesystems accept the chmod and drop the bit; with
			// no bit on disk there is nothing to assert.
			t.Logf("filesystem dropped the %s bit on %s", c.op, c.name)
			continue
		}
		if _, errb, code := runIn(t, cmd, dir, c.op, c.name); code != statusTrue || errb != "" {
			t.Errorf("test %s %s = (%q, %d), want 0", c.op, c.name, errb, code)
		}
	}
}

func TestDevicePrimaries(t *testing.T) {
	// /dev/null is a character device on every unix in scope, and is the
	// one device path the tests may rely on.
	if _, err := os.Stat("/dev/null"); err != nil {
		t.Skipf("/dev/null unavailable: %v", err)
	}
	dir := t.TempDir()
	cases := []struct {
		op   string
		want int
	}{
		{"-c", statusTrue},
		{"-b", statusFalse},
		{"-e", statusTrue},
		{"-f", statusFalse},
		{"-d", statusFalse},
		{"-s", statusFalse},
	}
	for _, c := range cases {
		if _, errb, code := runIn(t, cmd, dir, c.op, "/dev/null"); code != c.want || errb != "" {
			t.Errorf("test %s /dev/null = (%q, %d), want %d", c.op, errb, code, c.want)
		}
	}
}

func TestFifoAndSocketPrimaries(t *testing.T) {
	dir := t.TempDir()

	fifo := filepath.Join(dir, "fifo")
	if err := unix.Mkfifo(fifo, 0o600); err != nil {
		t.Skipf("mkfifo unavailable: %v", err)
	}
	// Opening a fifo blocks until both ends are present, so only
	// primaries that stat (never open) are exercised here.
	for _, c := range []struct {
		op   string
		want int
	}{{"-p", statusTrue}, {"-f", statusFalse}, {"-e", statusTrue}, {"-S", statusFalse}} {
		if _, errb, code := runIn(t, cmd, dir, c.op, "fifo"); code != c.want || errb != "" {
			t.Errorf("test %s fifo = (%q, %d), want %d", c.op, errb, code, c.want)
		}
	}

	sock := filepath.Join(dir, "sock")
	l, err := net.Listen("unix", sock)
	if err != nil {
		t.Skipf("unix sockets unavailable: %v", err)
	}
	defer l.Close()
	for _, c := range []struct {
		op   string
		want int
	}{{"-S", statusTrue}, {"-p", statusFalse}, {"-f", statusFalse}, {"-e", statusTrue}} {
		if _, errb, code := runIn(t, cmd, dir, c.op, "sock"); code != c.want || errb != "" {
			t.Errorf("test %s sock = (%q, %d), want %d", c.op, errb, code, c.want)
		}
	}
}

// TestFilePrimariesLongWorkingDirectory is the PATH_MAX conformance
// case: a working directory built one short component at a time (every
// step a valid, individually short mkdir, exactly like a shell's own
// cd) can legitimately reach unix.PathMax in length. Joining that
// directory with even a short relative operand into one materialized
// absolute string can then exceed PathMax, even though the file is
// perfectly reachable via a plain relative stat from the already-valid
// directory — the lookup GNU's own libc call performs, which never
// needs to build that string at all. Every primary that resolves a
// path must retry against the directory itself rather than silently
// reporting "false" for a file that does exist.
func TestFilePrimariesLongWorkingDirectory(t *testing.T) {
	// Resolve the platform temp dir's own symlinks (e.g. macOS's
	// /var -> /private/var) up front: growing the *unresolved* form to
	// just under PathMax would silently overrun once the kernel expands
	// that prefix during lookup — a real but different limit than the
	// one under test here.
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}

	// Grown by os.Chdir + a relative os.Mkdir at every step — exactly
	// like a shell's own `mkdir x && cd x` — so the setup itself never
	// materializes an overlong absolute string either.
	orig, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(orig) })
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}

	mkdirChdir := func(name string) {
		t.Helper()
		if err := os.Mkdir(name, 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		if err := os.Chdir(name); err != nil {
			t.Fatalf("chdir: %v", err)
		}
		dir = filepath.Join(dir, name)
	}

	comp := strings.Repeat("a", 40)
	// Grow one component at a time while there is still room for
	// another full one plus the eventual "/file" suffix.
	for len(dir)+1+len(comp)+1+len("file") < unix.PathMax {
		mkdirChdir(comp)
	}
	// Top up with a final, precisely-sized component so the directory
	// itself lands just 2 bytes short of PathMax: still valid and
	// reachable on its own, but joining it with "/file" overruns.
	want := unix.PathMax - 2
	if extra := want - len(dir) - 1; extra > 0 {
		mkdirChdir(strings.Repeat("a", extra))
	}
	// The setup only proves anything if the naively-joined absolute
	// path actually exceeds PathMax; the directory itself must still be
	// valid (reachable) on its own.
	joined := filepath.Join(dir, "file")
	if len(joined) <= unix.PathMax {
		t.Fatalf("setup: joined path len %d does not exceed PathMax %d", len(joined), unix.PathMax)
	}
	if len(dir) >= unix.PathMax {
		t.Fatalf("setup: working directory itself len %d already exceeds PathMax %d", len(dir), unix.PathMax)
	}
	if err := os.WriteFile("file", []byte("x"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	for _, op := range []string{"-e", "-f", "-r", "-w", "-s"} {
		if _, errb, code := runIn(t, cmd, dir, op, "file"); code != statusTrue || errb != "" {
			t.Errorf("test %s file (long working directory) = (%q, %d), want (\"\", 0)", op, errb, code)
		}
	}
	if _, errb, code := runIn(t, cmd, dir, "-d", "."); code != statusTrue || errb != "" {
		t.Errorf("test -d . (long working directory) = (%q, %d), want (\"\", 0)", errb, code)
	}
	if _, errb, code := runIn(t, cmd, dir, "file", "-ef", "file"); code != statusTrue || errb != "" {
		t.Errorf("test file -ef file (long working directory) = (%q, %d), want (\"\", 0)", errb, code)
	}
}

// TestFilePrimariesNearPathMaxRelativeOperand is the other half of the
// GA67 case (TestFilePrimariesLongWorkingDirectory covers the deep-cwd
// half): the operand itself is a relative pathname of nearly
// unix.PathMax bytes. Joining it onto even a short working directory
// overruns PathMax as one string, yet the pathname is resolvable as
// POSIX requires. Both resolution modes must get there: the embedded
// shape (Dir set, no process-cwd guarantee) through the os.Root retry,
// and the standalone shape (DirIsProcessCwd, as multicall.Main runs)
// through RunContext.Path keeping the operand relative.
func TestFilePrimariesNearPathMaxRelativeOperand(t *testing.T) {
	base, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	orig, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(orig) })
	if err := os.Chdir(base); err != nil {
		t.Fatal(err)
	}

	// Grown by relative mkdir + cd at every step, like a shell, so the
	// setup never materializes an overlong string either.
	dir := base
	var parts []string
	mkdirChdir := func(name string) {
		t.Helper()
		if err := os.Mkdir(name, 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		if err := os.Chdir(name); err != nil {
			t.Fatalf("chdir: %v", err)
		}
		dir = filepath.Join(dir, name)
		parts = append(parts, name)
	}
	comp := strings.Repeat("a", 40)
	for len(dir)+1+len(comp)+1+len("ff") < unix.PathMax {
		mkdirChdir(comp)
	}
	if extra := unix.PathMax - 2 - len(dir) - 1; extra > 0 {
		mkdirChdir(strings.Repeat("a", extra))
	}
	if err := os.WriteFile("ff", []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	rel := filepath.Join(strings.Join(parts, "/"), "ff")
	if joined := filepath.Join(base, rel); len(joined) < unix.PathMax {
		t.Fatalf("setup: joined path len %d does not reach PathMax %d", len(joined), unix.PathMax)
	}
	if len(rel)+1 > unix.PathMax {
		t.Fatalf("setup: relative operand len %d is not itself resolvable under PathMax %d", len(rel), unix.PathMax)
	}
	if err := os.Chdir(base); err != nil {
		t.Fatal(err)
	}

	// Embedded shape: resolved via the os.Root retry against Dir.
	for _, op := range []string{"-e", "-f", "-s"} {
		if _, errb, code := runIn(t, cmd, base, op, rel); code != statusTrue || errb != "" {
			t.Errorf("embedded: test %s <%d-byte operand> = (%q, %d), want (\"\", 0)", op, len(rel), errb, code)
		}
	}
	if _, errb, code := runIn(t, bracketCmd, base, "-f", rel, "]"); code != statusTrue || errb != "" {
		t.Errorf("embedded: [ -f <%d-byte operand> ] = (%q, %d), want (\"\", 0)", len(rel), errb, code)
	}

	// Standalone shape: the process cwd is base (chdir above), declared
	// via DirIsProcessCwd exactly as multicall.Main does.
	runNative := func(cmdt *tool.Tool, args ...string) (string, int) {
		var out, errb bytes.Buffer
		rc := &tool.RunContext{
			Ctx:             context.Background(),
			Dir:             base,
			DirIsProcessCwd: true,
			FS:              tool.NewLocalFS(),
			Stdio:           tool.Stdio{In: strings.NewReader(""), Out: &out, Err: &errb},
		}
		return errb.String(), cmdt.Run(rc, args)
	}
	for _, op := range []string{"-e", "-f", "-s"} {
		if errb, code := runNative(cmd, op, rel); code != statusTrue || errb != "" {
			t.Errorf("native: test %s <%d-byte operand> = (%q, %d), want (\"\", 0)", op, len(rel), errb, code)
		}
	}
	if errb, code := runNative(bracketCmd, "-f", rel, "]"); code != statusTrue || errb != "" {
		t.Errorf("native: [ -f <%d-byte operand> ] = (%q, %d), want (\"\", 0)", len(rel), errb, code)
	}
}
