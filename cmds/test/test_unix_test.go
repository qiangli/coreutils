//go:build unix

package testcmd

import (
	"net"
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/sys/unix"
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
