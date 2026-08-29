package paxcmd

import (
	"archive/tar"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestPAXReadUpdateContinuesAcrossAppendedMembers(t *testing.T) {
	d := t.TempDir()
	archive := filepath.Join(d, "update.pax")
	for _, tc := range []struct {
		name string
		body string
		when time.Time
	}{{"old", "archive-old", time.Unix(100, 0)}, {"new", "archive-new", time.Unix(300, 0)}} {
		if err := os.WriteFile(filepath.Join(d, tc.name), []byte(tc.body), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.Chtimes(filepath.Join(d, tc.name), tc.when, tc.when); err != nil {
			t.Fatal(err)
		}
		args := []string{"-w", "-f", archive, tc.name}
		if tc.name == "new" {
			args = []string{"-w", "-a", "-f", archive, tc.name}
		}
		if _, errs, code := exec(t, d, "", args...); code != 0 || errs != "" {
			t.Fatalf("archive %s=(%d,%q)", tc.name, code, errs)
		}
	}
	for _, name := range []string{"old", "new"} {
		if err := os.WriteFile(filepath.Join(d, name), []byte("current"), 0o600); err != nil {
			t.Fatal(err)
		}
		middle := time.Unix(200, 0)
		if err := os.Chtimes(filepath.Join(d, name), middle, middle); err != nil {
			t.Fatal(err)
		}
	}
	if _, errs, code := exec(t, d, "", "-r", "-u", "-n", "-f", archive, "*"); code != 0 || errs != "" {
		t.Fatalf("read update=(%d,%q)", code, errs)
	}
	if got := string(mustRead(t, filepath.Join(d, "old"))); got != "current" {
		t.Fatalf("old member changed to %q", got)
	}
	if got := string(mustRead(t, filepath.Join(d, "new"))); got != "archive-new" {
		t.Fatalf("new member remained %q", got)
	}
}

func TestBasicPAXStartsWithOrdinaryUSTARHeader(t *testing.T) {
	d := t.TempDir()
	if err := os.WriteFile(filepath.Join(d, "file"), []byte("payload"), 0o777); err != nil {
		t.Fatal(err)
	}
	out, errs, code := exec(t, d, "", "-w", "-x", "pax", "file")
	if code != 0 || errs != "" || len(out) < 512 {
		t.Fatalf("write=(%d,%q,%d bytes)", code, errs, len(out))
	}
	header := []byte(out[:512])
	if header[156] != tar.TypeReg || string(header[257:263]) != "ustar\x00" {
		t.Fatalf("first physical header type=%q magic=%q", header[156], header[257:263])
	}
	for _, b := range header[148:155] {
		if b < '0' || b > '7' {
			t.Fatalf("checksum contains non-octal byte: %q", header[148:156])
		}
	}
	if header[155] != ' ' {
		t.Fatalf("checksum terminator=%q", header[155])
	}
	if header[265] == 0 || header[297] == 0 {
		t.Fatalf("ordinary header lacks uname/gname: %q/%q", header[265:297], header[297:329])
	}
}

func TestPAXCommandLineSymlinkPrefixAccessesTargetDirectory(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("source access-time observation is a Linux certification behavior")
	}
	root := t.TempDir()
	target := t.TempDir()
	dir := filepath.Join(target, "dir")
	if err := os.Mkdir(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "file"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	oldAtime, oldMtime := time.Unix(100, 0), time.Unix(200, 0)
	if err := os.Chtimes(dir, oldAtime, oldMtime); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, filepath.Join(root, "prefix")); err != nil {
		t.Fatal(err)
	}
	if _, errs, code := exec(t, root, "", "-w", "-H", "prefix/dir"); code != 0 || errs != "" {
		t.Fatalf("-H symlink-prefix write=(%d,%q)", code, errs)
	}
	fi, err := os.Stat(dir)
	if err != nil {
		t.Fatal(err)
	}
	atime, ok := sourceAccessTimeFn(fi)
	if !ok || !atime.After(oldAtime) {
		t.Fatalf("target directory access time=%v ok=%v, want after %v", atime, ok, oldAtime)
	}
}

func TestPAXPatternNotationNamedClass(t *testing.T) {
	s := newSelector(&options{}, []string{"[[:alpha:]]"})
	if !s.keep("a", false) || s.keep("1", false) || s.keep("a/b", false) {
		t.Fatal("POSIX alpha pattern did not match exactly one alphabetic pathname component")
	}
}

func TestPAXDefaultWritePreservesRawCLocaleName(t *testing.T) {
	d := t.TempDir()
	name := string([]byte{0xe4})
	if err := os.WriteFile(filepath.Join(d, name), []byte("x"), 0o600); err != nil {
		t.Skipf("filesystem cannot represent raw-byte pathname: %v", err)
	}
	archive := filepath.Join(d, "raw.pax")
	_, errs, code := execPaxEnv(t, d, []string{"LC_ALL=C"}, "-w", "-f", archive, name)
	if code != 0 || errs != "" {
		t.Fatalf("default raw-name write=(%d,%q), want successful quiet preservation", code, errs)
	}
	f, err := os.Open(archive)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	h, err := tar.NewReader(f).Next()
	if err != nil {
		t.Fatal(err)
	}
	if h.Name != name {
		t.Fatalf("physical member name=%x, want raw %x", []byte(h.Name), []byte(name))
	}
}

func TestPAXGermanEquivalenceSelectionAndSubstitution(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("glibc locale provider is Linux-only")
	}
	if _, err := paxLocaleTables([]string{"LC_ALL=de_DE.ISO-8859-1"}); err != nil {
		t.Skipf("carried de_DE.ISO-8859-1 locale unavailable: %v", err)
	}
	d := t.TempDir()
	for _, name := range []string{"a", string([]byte{0xe4}), "z"} {
		if err := os.WriteFile(filepath.Join(d, name), []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	archive := filepath.Join(d, "raw.pax")
	if _, errs, code := execPaxEnv(t, d, []string{"LC_ALL=C"}, "-w", "-f", archive, "a", string([]byte{0xe4}), "z"); code != 0 {
		t.Fatalf("write code=%d stderr=%q", code, errs)
	}
	env := []string{"LC_ALL=de_DE.ISO-8859-1"}
	out, errs, code := execPaxEnv(t, d, env, "-f", archive, "[[=a=]]")
	if code != 0 || errs != "" || !strings.Contains(out, "a\n") || !strings.Contains(out, string([]byte{0xe4})+"\n") || strings.Contains(out, "z\n") {
		t.Fatalf("equivalence selection=(%d,%x,%q)", code, []byte(out), errs)
	}
	out, errs, code = execPaxEnv(t, d, env, "-f", archive, "-s", "/[[=a=]]/_/")
	if code != 0 || errs != "" || strings.Count(out, "_\n") != 2 || !strings.Contains(out, "z\n") {
		t.Fatalf("equivalence substitution=(%d,%x,%q)", code, []byte(out), errs)
	}
}
