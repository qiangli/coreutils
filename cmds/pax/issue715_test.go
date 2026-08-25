package paxcmd

import (
	"archive/tar"
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

type fakeInteractiveTTY struct {
	in       *strings.Reader
	out      bytes.Buffer
	writeErr error
	closeErr error
}

func newFakeInteractiveTTY(responses string) *fakeInteractiveTTY {
	return &fakeInteractiveTTY{in: strings.NewReader(responses)}
}
func (f *fakeInteractiveTTY) Read(p []byte) (int, error) { return f.in.Read(p) }
func (f *fakeInteractiveTTY) Write(p []byte) (int, error) {
	if f.writeErr != nil {
		return 0, f.writeErr
	}
	return f.out.Write(p)
}
func (f *fakeInteractiveTTY) Close() error { return f.closeErr }

func withInteractiveTTY(t *testing.T, tty *fakeInteractiveTTY) {
	t.Helper()
	old := openInteractiveTTY
	openInteractiveTTY = func() (io.ReadWriteCloser, error) { return tty, nil }
	t.Cleanup(func() { openInteractiveTTY = old })
}

func TestInteractiveListOrdersSubstitutionBeforeResponses(t *testing.T) {
	d := makeTree(t)
	arc := writeArchive(t, d, "src")
	tty := newFakeInteractiveTTY(".\n\nlisted\n.\n")
	withInteractiveTTY(t, tty)
	out, errOut, code := exec(t, d, "", "-i", "-f", arc, "-s", "/src/changed/")
	if code != 0 || errOut != "" {
		t.Fatalf("code=%d stderr=%q", code, errOut)
	}
	if out != "changed/\nlisted\nchanged/sub/b.txt\n" {
		t.Fatalf("stdout=%q", out)
	}
	if got := tty.out.String(); got != "pax: rename changed/? pax: rename changed/a.txt? pax: rename changed/sub/? pax: rename changed/sub/b.txt? " {
		t.Fatalf("terminal prompts=%q", got)
	}
}

func TestInteractiveReadPreflightsAndRenamesHardlinks(t *testing.T) {
	var raw bytes.Buffer
	tw := tar.NewWriter(&raw)
	if err := tw.WriteHeader(&tar.Header{Name: "a", Typeflag: tar.TypeReg, Mode: 0o644, Size: 1}); err != nil {
		t.Fatal(err)
	}
	if _, err := tw.Write([]byte("x")); err != nil {
		t.Fatal(err)
	}
	if err := tw.WriteHeader(&tar.Header{Name: "b", Typeflag: tar.TypeLink, Linkname: "a", Mode: 0o644}); err != nil {
		t.Fatal(err)
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	d := t.TempDir()
	tty := newFakeInteractiveTTY("renamed-a\nrenamed-b\n")
	withInteractiveTTY(t, tty)
	_, errOut, code := exec(t, d, raw.String(), "-r", "-i")
	if code != 0 || errOut != "" {
		t.Fatalf("code=%d stderr=%q", code, errOut)
	}
	a, err := os.Stat(filepath.Join(d, "renamed-a"))
	if err != nil {
		t.Fatal(err)
	}
	b, err := os.Stat(filepath.Join(d, "renamed-b"))
	if err != nil {
		t.Fatal(err)
	}
	if !os.SameFile(a, b) {
		t.Fatal("interactive hard-link target did not follow the renamed member")
	}
}

func TestInteractiveWriteAndCopyPromptExactlyOnce(t *testing.T) {
	d := t.TempDir()
	if err := os.WriteFile(filepath.Join(d, "source"), []byte("body"), 0o644); err != nil {
		t.Fatal(err)
	}
	arc := filepath.Join(d, "out.tar")
	tty := newFakeInteractiveTTY("archived\n")
	withInteractiveTTY(t, tty)
	if _, errOut, code := exec(t, d, "", "-w", "-i", "-f", arc, "source"); code != 0 || errOut != "" {
		t.Fatalf("write code=%d stderr=%q", code, errOut)
	}
	if got := listNames(t, mustRead(t, arc)); len(got) != 1 || got[0] != "archived" {
		t.Fatalf("archive names=%v", got)
	}

	dest := filepath.Join(d, "dest")
	if err := os.Mkdir(dest, 0o755); err != nil {
		t.Fatal(err)
	}
	tty = newFakeInteractiveTTY("copied\n")
	openInteractiveTTY = func() (io.ReadWriteCloser, error) { return tty, nil }
	if _, errOut, code := exec(t, d, "", "-r", "-w", "-i", "source", "dest"); code != 0 || errOut != "" {
		t.Fatalf("copy code=%d stderr=%q", code, errOut)
	}
	if got := string(mustRead(t, filepath.Join(dest, "copied"))); got != "body" {
		t.Fatalf("copied body=%q", got)
	}
	if got := strings.Count(tty.out.String(), "pax: rename"); got != 1 {
		t.Fatalf("copy prompted %d times, transcript=%q", got, tty.out.String())
	}
}

func TestInteractiveAndResetAccessTimeInCPIOWriteLane(t *testing.T) {
	d := t.TempDir()
	source := filepath.Join(d, "source")
	if err := os.WriteFile(source, []byte("body"), 0o644); err != nil {
		t.Fatal(err)
	}
	wantA := time.Unix(1000000200, 0)
	wantM := time.Unix(1000000300, 0)
	if err := os.Chtimes(source, wantA, wantM); err != nil {
		t.Fatal(err)
	}
	tty := newFakeInteractiveTTY("renamed\n")
	withInteractiveTTY(t, tty)
	raw, errOut, code := exec(t, d, "", "-w", "-x", "cpio", "-i", "-t", "source")
	if code != 0 || errOut != "" {
		t.Fatalf("code=%d stderr=%q", code, errOut)
	}
	out, listErr, listCode := exec(t, d, raw)
	if listCode != 0 || listErr != "" || out != "renamed\n" {
		t.Fatalf("list code=%d stdout=%q stderr=%q", listCode, out, listErr)
	}
	assertSourceTimes(t, source, wantA, wantM)
}

func TestInteractiveFailuresAreImmediateAndNonzero(t *testing.T) {
	d := makeTree(t)
	arc := writeArchive(t, d, "src")

	old := openInteractiveTTY
	defer func() { openInteractiveTTY = old }()
	openInteractiveTTY = func() (io.ReadWriteCloser, error) { return nil, errors.New("no controlling tty") }
	out, errOut, code := exec(t, d, "", "-i", "-f", arc)
	if code != 1 || out != "" || !strings.Contains(errOut, "open /dev/tty: no controlling tty") {
		t.Fatalf("open: code=%d stdout=%q stderr=%q", code, out, errOut)
	}

	extract := t.TempDir()
	tty := newFakeInteractiveTTY("renamed\n") // archive has more members: second prompt reaches EOF.
	openInteractiveTTY = func() (io.ReadWriteCloser, error) { return tty, nil }
	_, errOut, code = exec(t, extract, "", "-r", "-i", "-f", arc)
	if code != 1 || !strings.Contains(errOut, "read /dev/tty: EOF") {
		t.Fatalf("EOF: code=%d stderr=%q", code, errOut)
	}
	entries, err := os.ReadDir(extract)
	if err != nil || len(entries) != 0 {
		t.Fatalf("EOF must precede extraction: entries=%v err=%v", entries, err)
	}

	tty = newFakeInteractiveTTY(".\n.\n.\n.\n")
	tty.writeErr = errors.New("terminal write failed")
	openInteractiveTTY = func() (io.ReadWriteCloser, error) { return tty, nil }
	_, errOut, code = exec(t, d, "", "-i", "-f", arc)
	if code != 1 || !strings.Contains(errOut, "write /dev/tty: terminal write failed") {
		t.Fatalf("write: code=%d stderr=%q", code, errOut)
	}

	tty = newFakeInteractiveTTY(".\n.\n.\n.\n")
	tty.closeErr = errors.New("terminal close failed")
	openInteractiveTTY = func() (io.ReadWriteCloser, error) { return tty, nil }
	_, errOut, code = exec(t, d, "", "-i", "-f", arc)
	if code != 1 || !strings.Contains(errOut, "close /dev/tty: terminal close failed") {
		t.Fatalf("close: code=%d stderr=%q", code, errOut)
	}
}

func TestCopyLinkRegularAndSymlinkFollowModes(t *testing.T) {
	d := t.TempDir()
	if err := os.MkdirAll(filepath.Join(d, "src"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(d, "src", "file"), []byte("data"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("file", filepath.Join(d, "src", "link")); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}

	for _, tc := range []struct {
		name        string
		args        []string
		source      string
		destName    string
		wantSymlink bool
	}{
		{"regular", []string{"-r", "-w", "-l", "src/file", "dest-regular"}, "src/file", "src/file", false},
		{"physical-symlink", []string{"-r", "-w", "-l", "src/link", "dest-physical"}, "src/link", "src/link", true},
		{"H-command-line", []string{"-r", "-w", "-l", "-H", "src/link", "dest-H"}, "src/file", "src/link", false},
		{"L-descendant", []string{"-r", "-w", "-l", "-L", "src", "dest-L"}, "src/file", "src/link", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dest := filepath.Join(d, tc.args[len(tc.args)-1])
			if err := os.Mkdir(dest, 0o755); err != nil {
				t.Fatal(err)
			}
			if _, errOut, code := exec(t, d, "", tc.args...); code != 0 || errOut != "" {
				t.Fatalf("code=%d stderr=%q", code, errOut)
			}
			srcInfo, err := os.Lstat(filepath.Join(d, tc.source))
			if err != nil {
				t.Fatal(err)
			}
			dstInfo, err := os.Lstat(filepath.Join(dest, tc.destName))
			if err != nil {
				t.Fatal(err)
			}
			if (dstInfo.Mode()&os.ModeSymlink != 0) != tc.wantSymlink {
				t.Fatalf("destination mode=%s, wantSymlink=%v", dstInfo.Mode(), tc.wantSymlink)
			}
			if !os.SameFile(srcInfo, dstInfo) {
				t.Fatal("destination is not a hard link to the selected source object")
			}
		})
	}
}

func TestCopyLinkFallsBackAndKeepsDestinationSafe(t *testing.T) {
	d := t.TempDir()
	if err := os.WriteFile(filepath.Join(d, "source"), []byte("body"), 0o640); err != nil {
		t.Fatal(err)
	}
	dest := filepath.Join(d, "dest")
	if err := os.Mkdir(dest, 0o755); err != nil {
		t.Fatal(err)
	}
	old := linkSourceFn
	linkSourceFn = func(string, string) error { return syscall.EXDEV }
	defer func() { linkSourceFn = old }()
	if _, errOut, code := exec(t, d, "", "-r", "-w", "-l", "source", "dest"); code != 0 || errOut != "" {
		t.Fatalf("fallback code=%d stderr=%q", code, errOut)
	}
	if got := string(mustRead(t, filepath.Join(dest, "source"))); got != "body" {
		t.Fatalf("fallback body=%q", got)
	}
	srcInfo, _ := os.Stat(filepath.Join(d, "source"))
	dstInfo, _ := os.Stat(filepath.Join(dest, "source"))
	if os.SameFile(srcInfo, dstInfo) {
		t.Fatal("forced fallback unexpectedly created a hard link")
	}
	if err := os.Symlink("source", filepath.Join(d, "source-link")); err == nil {
		symlinkDest := filepath.Join(d, "dest-symlink")
		if err := os.Mkdir(symlinkDest, 0o755); err != nil {
			t.Fatal(err)
		}
		if _, errOut, code := exec(t, d, "", "-r", "-w", "-l", "source-link", "dest-symlink"); code != 0 || errOut != "" {
			t.Fatalf("symlink fallback code=%d stderr=%q", code, errOut)
		}
		if got, err := os.Readlink(filepath.Join(symlinkDest, "source-link")); err != nil || got != "source" {
			t.Fatalf("symlink fallback target=%q err=%v", got, err)
		}
	}

	outside := filepath.Join(d, "escape")
	_, errOut, code := exec(t, d, "", "-r", "-w", "-l", "-s", "/source/..\\/escape/", "source", "dest")
	if code == 0 || !strings.Contains(errOut, "refusing") {
		t.Fatalf("unsafe rename code=%d stderr=%q", code, errOut)
	}
	if _, err := os.Lstat(outside); !os.IsNotExist(err) {
		t.Fatalf("unsafe copy escaped destination: %v", err)
	}
}

func TestResetAccessTimesForDirectoriesAndSymlinks(t *testing.T) {
	d := t.TempDir()
	sourceDir := filepath.Join(d, "tree")
	if err := os.Mkdir(sourceDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sourceDir, "file"), []byte("body"), 0o644); err != nil {
		t.Fatal(err)
	}
	symlink := filepath.Join(sourceDir, "link")
	if err := os.Symlink("file", symlink); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	wantA := time.Unix(1000000000, 111000000)
	wantM := time.Unix(1000000100, 222000000)
	if err := os.Chtimes(sourceDir, wantA, wantM); err != nil {
		t.Fatal(err)
	}
	if err := restoreSourceTimes(symlink, wantA, wantM, true); err != nil {
		t.Skipf("symlink timestamp restoration unavailable: %v", err)
	}
	arc := filepath.Join(d, "tree.tar")
	if _, errOut, code := exec(t, d, "", "-w", "-t", "-f", arc, "tree"); code != 0 || errOut != "" {
		t.Fatalf("code=%d stderr=%q", code, errOut)
	}
	assertSourceTimes(t, sourceDir, wantA, wantM)
	fi, err := os.Lstat(symlink)
	if err != nil {
		t.Fatal(err)
	}
	gotA, ok := sourceAccessTime(fi)
	if !ok {
		t.Skip("platform does not expose symlink access time")
	}
	if gotA.UnixMicro() != wantA.UnixMicro() || fi.ModTime().UnixMicro() != wantM.UnixMicro() {
		t.Fatalf("symlink times atime=%v mtime=%v, want %v %v", gotA, fi.ModTime(), wantA, wantM)
	}
}

func TestResetAccessTimesWriteAndCopyAndFailureStatus(t *testing.T) {
	d := t.TempDir()
	source := filepath.Join(d, "source")
	if err := os.WriteFile(source, []byte("body"), 0o644); err != nil {
		t.Fatal(err)
	}
	wantA := time.Unix(946684800, 123456000)
	wantM := time.Unix(978307200, 654321000)
	if err := os.Chtimes(source, wantA, wantM); err != nil {
		t.Fatal(err)
	}
	arc := filepath.Join(d, "out.tar")
	if _, errOut, code := exec(t, d, "", "-w", "-t", "-f", arc, "source"); code != 0 || errOut != "" {
		t.Fatalf("write code=%d stderr=%q", code, errOut)
	}
	assertSourceTimes(t, source, wantA, wantM)

	dest := filepath.Join(d, "dest")
	if err := os.Mkdir(dest, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, errOut, code := exec(t, d, "", "-r", "-w", "-t", "source", "dest"); code != 0 || errOut != "" {
		t.Fatalf("copy code=%d stderr=%q", code, errOut)
	}
	assertSourceTimes(t, source, wantA, wantM)

	old := restoreSourceTimesFn
	restoreSourceTimesFn = func(string, time.Time, time.Time, bool) error { return errors.New("restore denied") }
	defer func() { restoreSourceTimesFn = old }()
	_, errOut, code := exec(t, d, "", "-w", "-t", "-f", filepath.Join(d, "failed.tar"), "source")
	if code != 1 || !strings.Contains(errOut, "restore source access time: restore denied") {
		t.Fatalf("restore failure code=%d stderr=%q", code, errOut)
	}
}

func assertSourceTimes(t *testing.T, path string, wantA, wantM time.Time) {
	t.Helper()
	fi, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	gotA, ok := sourceAccessTime(fi)
	if !ok {
		t.Skip("platform does not expose access time")
	}
	// Filesystems may quantize timestamps; compare at microsecond precision.
	if gotA.UnixMicro() != wantA.UnixMicro() || fi.ModTime().UnixMicro() != wantM.UnixMicro() {
		t.Fatalf("times atime=%v mtime=%v, want %v %v", gotA, fi.ModTime(), wantA, wantM)
	}
}
