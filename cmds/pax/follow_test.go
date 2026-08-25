package paxcmd

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCopySourceFileDoesNotMisclassifyStreamFailures(t *testing.T) {
	readErr := copySourceFile(io.Discard, &errorAfterReader{data: []byte("x")})
	if sourceTraversalFailure(readErr) || !strings.Contains(readErr.Error(), "injected read failure") {
		t.Fatalf("read failure classification = %T %v", readErr, readErr)
	}
	writeErr := copySourceFile(shortWriter{}, strings.NewReader("x"))
	if sourceTraversalFailure(writeErr) || !errors.Is(writeErr, io.ErrShortWrite) {
		t.Fatalf("write failure classification = %T %v", writeErr, writeErr)
	}
}

// The -H/-L/-X traversal tests. Every format goes through the one shared
// walker (walkOperand), so each behavior is asserted per archive lane —
// pax, ustar, cpio — and through copy mode, which reuses the same walker.

var followFormats = []string{"pax", "ustar", "cpio"}

func mustSymlink(t *testing.T, target, link string) {
	t.Helper()
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("symlinks unavailable on this host: %v", err)
	}
}

// makeFollowTree builds:
//
//	target.txt              "tdata"
//	real/f.txt              "data"
//	real/inner-link         -> ../target.txt   (encountered symlink)
//	dirlink                 -> real            (command-line symlink to dir)
//	filelink                -> target.txt      (command-line symlink to file)
func makeFollowTree(t *testing.T) string {
	t.Helper()
	d := t.TempDir()
	if err := os.MkdirAll(filepath.Join(d, "real"), 0o755); err != nil {
		t.Fatal(err)
	}
	for p, c := range map[string]string{"target.txt": "tdata", "real/f.txt": "data"} {
		if err := os.WriteFile(filepath.Join(d, p), []byte(c), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	mustSymlink(t, "../target.txt", filepath.Join(d, "real", "inner-link"))
	mustSymlink(t, "real", filepath.Join(d, "dirlink"))
	mustSymlink(t, "target.txt", filepath.Join(d, "filelink"))
	return d
}

// archiveAndExtract writes the operands with the given extra write flags and
// format, then extracts into a fresh directory it returns.
func archiveAndExtract(t *testing.T, d, format string, writeFlags []string, operands ...string) string {
	t.Helper()
	arc := filepath.Join(d, "follow-"+format+".arc")
	args := append([]string{"-w", "-x", format, "-f", arc}, writeFlags...)
	args = append(args, operands...)
	if _, errs, code := exec(t, d, "", args...); code != 0 {
		t.Fatalf("write (-x %s) failed: %d %s", format, code, errs)
	}
	dest := t.TempDir()
	if _, errs, code := exec(t, dest, "", "-r", "-f", arc); code != 0 {
		t.Fatalf("read (-x %s) failed: %d %s", format, code, errs)
	}
	return dest
}

func mustBeSymlink(t *testing.T, path string) {
	t.Helper()
	fi, err := os.Lstat(path)
	if err != nil {
		t.Fatalf("%s: %v", path, err)
	}
	if fi.Mode()&os.ModeSymlink == 0 {
		t.Errorf("%s should be a symlink, is %v", path, fi.Mode())
	}
}

func mustBeRegularWith(t *testing.T, path, content string) {
	t.Helper()
	fi, err := os.Lstat(path)
	if err != nil {
		t.Fatalf("%s: %v", path, err)
	}
	if !fi.Mode().IsRegular() {
		t.Fatalf("%s should be a regular file, is %v", path, fi.Mode())
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != content {
		t.Errorf("%s content = %q, want %q", path, got, content)
	}
}

func mustBeRealDir(t *testing.T, path string) {
	t.Helper()
	fi, err := os.Lstat(path)
	if err != nil {
		t.Fatalf("%s: %v", path, err)
	}
	if fi.Mode()&os.ModeSymlink != 0 || !fi.IsDir() {
		t.Errorf("%s should be a real directory, is %v", path, fi.Mode())
	}
}

// Default traversal is physical: symlink operands are archived as the
// symlinks they are, in every write format.
func TestWriteDefaultIsPhysical(t *testing.T) {
	for _, format := range followFormats {
		t.Run(format, func(t *testing.T) {
			d := makeFollowTree(t)
			dest := archiveAndExtract(t, d, format, nil, "dirlink", "filelink")
			mustBeSymlink(t, filepath.Join(dest, "dirlink"))
			mustBeSymlink(t, filepath.Join(dest, "filelink"))
		})
	}
}

// -H resolves symlinks named on the command line — a file operand archives
// the target's data under the operand's name, a directory operand is
// descended — while symlinks ENCOUNTERED below stay symlinks.
func TestDashHFollowsCommandLineSymlinksOnly(t *testing.T) {
	for _, format := range followFormats {
		t.Run(format, func(t *testing.T) {
			d := makeFollowTree(t)
			dest := archiveAndExtract(t, d, format, []string{"-H"}, "dirlink", "filelink")
			mustBeRegularWith(t, filepath.Join(dest, "filelink"), "tdata")
			mustBeRealDir(t, filepath.Join(dest, "dirlink"))
			mustBeRegularWith(t, filepath.Join(dest, "dirlink", "f.txt"), "data")
			mustBeSymlink(t, filepath.Join(dest, "dirlink", "inner-link"))
		})
	}
}

// -L resolves every symlink: a nested file link becomes the file's data, and
// a nested directory link is descended like a directory.
func TestDashLFollowsEncounteredSymlinks(t *testing.T) {
	for _, format := range followFormats {
		t.Run(format, func(t *testing.T) {
			d := makeFollowTree(t)
			if err := os.MkdirAll(filepath.Join(d, "realsub"), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(d, "realsub", "g.txt"), []byte("gdata"), 0o644); err != nil {
				t.Fatal(err)
			}
			mustSymlink(t, "../realsub", filepath.Join(d, "real", "sublink"))
			dest := archiveAndExtract(t, d, format, []string{"-L"}, "real")
			mustBeRegularWith(t, filepath.Join(dest, "real", "inner-link"), "tdata")
			mustBeRealDir(t, filepath.Join(dest, "real", "sublink"))
			mustBeRegularWith(t, filepath.Join(dest, "real", "sublink", "g.txt"), "gdata")
		})
	}
}

// With repeated or mixed -H/-L, the LAST one on the command line wins,
// including inside a clustered short-option group.
func TestLastOfRepeatedFollowOptionsWins(t *testing.T) {
	cases := []struct {
		name       string
		flags      []string
		nestedLink bool // real/inner-link stays a symlink (H won)
	}{
		{"L-then-H", []string{"-L", "-H"}, true},
		{"H-then-L", []string{"-H", "-L"}, false},
		{"clustered-LH", []string{"-LH"}, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			d := makeFollowTree(t)
			dest := archiveAndExtract(t, d, "pax", tc.flags, "real")
			p := filepath.Join(dest, "real", "inner-link")
			if tc.nestedLink {
				mustBeSymlink(t, p)
			} else {
				mustBeRegularWith(t, p, "tdata")
			}
		})
	}
}

// A followed symlink that leads back to an ancestor of the current descent is
// a filesystem cycle: it must be diagnosed, force a nonzero exit, and
// terminate the pax invocation without processing later operands.
func TestFollowCycleTerminatesPax(t *testing.T) {
	d := t.TempDir()
	if err := os.MkdirAll(filepath.Join(d, "loop", "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(d, "loop", "a.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(d, "later.txt"), []byte("later"), 0o644); err != nil {
		t.Fatal(err)
	}
	mustSymlink(t, filepath.Join("..", "..", "loop"), filepath.Join(d, "loop", "sub", "back"))

	arc := filepath.Join(d, "cycle.arc")
	_, errs, code := exec(t, d, "", "-w", "-L", "-f", arc, "loop", "later.txt")
	if code == 0 {
		t.Fatalf("a traversal cycle must exit nonzero; stderr %q", errs)
	}
	if !strings.Contains(errs, "cycle") {
		t.Fatalf("expected a cycle diagnostic, got %q", errs)
	}
	out, errs2, code := exec(t, d, "", "-f", arc)
	if code != 0 {
		t.Fatalf("listing the partial archive failed: %d %s", code, errs2)
	}
	if !strings.Contains(out, "loop/a.txt") {
		t.Errorf("members visited before the cycle should remain in the partial archive; got %q", out)
	}
	if strings.Contains(out, "later.txt") {
		t.Errorf("pax must terminate after detecting the cycle; got %q", out)
	}
}

// A file that disappears (or a dangling symlink followed by -L) must be
// diagnosed and make the status nonzero, but POSIX requires write processing
// to continue with later files.
func TestDashLBrokenSymlinkContinuesDirectory(t *testing.T) {
	for _, format := range followFormats {
		t.Run(format, func(t *testing.T) {
			d := t.TempDir()
			if err := os.Mkdir(filepath.Join(d, "src"), 0o755); err != nil {
				t.Fatal(err)
			}
			mustSymlink(t, "missing", filepath.Join(d, "src", "a-broken"))
			if err := os.WriteFile(filepath.Join(d, "src", "z-good"), []byte("good"), 0o644); err != nil {
				t.Fatal(err)
			}
			arc := filepath.Join(d, "broken-"+format+".arc")
			_, errs, code := exec(t, d, "", "-w", "-L", "-x", format, "-f", arc, "src")
			if code == 0 || !strings.Contains(errs, "a-broken") {
				t.Fatalf("broken followed link = code %d, stderr %q", code, errs)
			}
			out, listErr, listCode := exec(t, d, "", "-f", arc)
			if listCode != 0 {
				t.Fatalf("list partial archive: code %d, stderr %q", listCode, listErr)
			}
			if !strings.Contains(out, "src/z-good") {
				t.Fatalf("later file omitted after broken link: %q", out)
			}
		})
	}
}

func TestCopyModeSourceErrorsContinue(t *testing.T) {
	t.Run("missing operand does not discard later operand", func(t *testing.T) {
		d := t.TempDir()
		if err := os.WriteFile(filepath.Join(d, "good"), []byte("good"), 0o644); err != nil {
			t.Fatal(err)
		}
		dest := t.TempDir()
		_, errs, code := exec(t, d, "", "-r", "-w", "missing", "good", dest)
		if code == 0 || !strings.Contains(errs, "missing") {
			t.Fatalf("missing source = code %d, stderr %q", code, errs)
		}
		mustBeRegularWith(t, filepath.Join(dest, "good"), "good")
	})

	t.Run("broken followed descendant does not discard later member", func(t *testing.T) {
		d := t.TempDir()
		if err := os.Mkdir(filepath.Join(d, "src"), 0o755); err != nil {
			t.Fatal(err)
		}
		mustSymlink(t, "missing", filepath.Join(d, "src", "a-broken"))
		if err := os.WriteFile(filepath.Join(d, "src", "z-good"), []byte("good"), 0o644); err != nil {
			t.Fatal(err)
		}
		dest := t.TempDir()
		_, errs, code := exec(t, d, "", "-r", "-w", "-L", "src", dest)
		if code == 0 || !strings.Contains(errs, "a-broken") {
			t.Fatalf("broken followed source = code %d, stderr %q", code, errs)
		}
		mustBeRegularWith(t, filepath.Join(dest, "src", "z-good"), "good")
	})
}

// Ancestor-only means a DAG is not a cycle: two sibling symlinks into the
// same directory archive it twice without any diagnostic.
func TestSharedSiblingDirectoryIsNotACycle(t *testing.T) {
	d := t.TempDir()
	for _, p := range []string{"shared", "top"} {
		if err := os.MkdirAll(filepath.Join(d, p), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(d, "shared", "s.txt"), []byte("s"), 0o644); err != nil {
		t.Fatal(err)
	}
	mustSymlink(t, "../shared", filepath.Join(d, "top", "l1"))
	mustSymlink(t, "../shared", filepath.Join(d, "top", "l2"))

	arc := filepath.Join(d, "dag.arc")
	if _, errs, code := exec(t, d, "", "-w", "-L", "-f", arc, "top"); code != 0 {
		t.Fatalf("a diamond is not a cycle; exit %d stderr %q", code, errs)
	}
	out, _, code := exec(t, d, "", "-f", arc)
	if code != 0 {
		t.Fatal("list failed")
	}
	for _, want := range []string{"top/l1/s.txt", "top/l2/s.txt"} {
		if !strings.Contains(out, want) {
			t.Errorf("expected member %q in %q", want, out)
		}
	}
}

// -d with -H on a symlink-to-directory operand: the operand resolves to a
// directory, is archived as a directory, and is not descended.
func TestDashDWithDashHArchivesDirectoryEntryOnly(t *testing.T) {
	d := makeFollowTree(t)
	dest := archiveAndExtract(t, d, "pax", []string{"-H", "-d"}, "dirlink")
	mustBeRealDir(t, filepath.Join(dest, "dirlink"))
	if _, err := os.Lstat(filepath.Join(dest, "dirlink", "f.txt")); err == nil {
		t.Error("-d must not descend into the resolved directory")
	}
}

// withFakeDevices maps any path containing an "xmnt" element to a different
// device than everything else, simulating a mount point without needing a
// second filesystem.
func withFakeDevices(t *testing.T) {
	t.Helper()
	old := deviceOf
	deviceOf = func(abs string, fi os.FileInfo) (uint64, bool) {
		if strings.Contains(filepath.ToSlash(abs), "/xmnt") {
			return 2, true
		}
		return 1, true
	}
	t.Cleanup(func() { deviceOf = old })
}

func makeCrossDeviceTree(t *testing.T) string {
	t.Helper()
	d := t.TempDir()
	if err := os.MkdirAll(filepath.Join(d, "src", "xmnt"), 0o755); err != nil {
		t.Fatal(err)
	}
	for p, c := range map[string]string{"src/a.txt": "alpha", "src/xmnt/b.txt": "beta"} {
		if err := os.WriteFile(filepath.Join(d, p), []byte(c), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return d
}

// -X archives the directory that sits on another device (it was encountered
// on the operand's device) but never descends into it — in every write format.
func TestDashXPrunesOtherDeviceDirectories(t *testing.T) {
	for _, format := range followFormats {
		t.Run(format, func(t *testing.T) {
			withFakeDevices(t)
			d := makeCrossDeviceTree(t)
			arc := filepath.Join(d, "xdev.arc")
			if _, errs, code := exec(t, d, "", "-w", "-X", "-x", format, "-f", arc, "src"); code != 0 {
				t.Fatalf("write -X failed: %d %s", code, errs)
			}
			out, _, code := exec(t, d, "", "-f", arc)
			if code != 0 {
				t.Fatal("list failed")
			}
			if !strings.Contains(out, "src/a.txt") || !strings.Contains(out, "src/xmnt") {
				t.Errorf("the same-device file and the mount-point directory itself belong in the archive; got %q", out)
			}
			if strings.Contains(out, "src/xmnt/b.txt") {
				t.Errorf("-X must not descend past the device boundary; got %q", out)
			}
		})
	}
}

// -X applies identically in copy mode, which shares the walker.
func TestDashXPrunesInCopyMode(t *testing.T) {
	withFakeDevices(t)
	d := makeCrossDeviceTree(t)
	dest := t.TempDir()
	if _, errs, code := exec(t, d, "", "-r", "-w", "-X", "src", dest); code != 0 {
		t.Fatalf("copy -X failed: %d %s", code, errs)
	}
	mustBeRegularWith(t, filepath.Join(dest, "src", "a.txt"), "alpha")
	mustBeRealDir(t, filepath.Join(dest, "src", "xmnt"))
	if _, err := os.Lstat(filepath.Join(dest, "src", "xmnt", "b.txt")); err == nil {
		t.Error("-X must not copy past the device boundary")
	}
}

// Where the platform exposes no device identity, -X refuses loudly rather
// than silently archiving across mount points.
func TestDashXFailsLoudlyWithoutDeviceIdentity(t *testing.T) {
	old := deviceOf
	deviceOf = func(string, os.FileInfo) (uint64, bool) { return 0, false }
	t.Cleanup(func() { deviceOf = old })

	d := makeCrossDeviceTree(t)
	arc := filepath.Join(d, "noid.arc")
	_, errs, code := exec(t, d, "", "-w", "-X", "-f", arc, "src")
	if code == 0 {
		t.Fatal("-X without device identity must fail")
	}
	if !strings.Contains(errs, "cannot determine the device") {
		t.Fatalf("expected a loud device-identity refusal, got %q", errs)
	}
}

// POSIX lists -H/-L in every synopsis form: list and read modes accept them
// (they traverse no source hierarchy, so they have nothing to do).
func TestFollowOptionsAreNoOpsInListAndRead(t *testing.T) {
	d := makeTree(t)
	arc := writeArchive(t, d, "src")
	if out, errs, code := exec(t, d, "", "-L", "-f", arc); code != 0 || !strings.Contains(out, "src/a.txt") {
		t.Fatalf("list -L must be accepted: %d %s", code, errs)
	}
	dest := t.TempDir()
	if _, errs, code := exec(t, dest, "", "-r", "-H", "-f", arc); code != 0 {
		t.Fatalf("read -H must be accepted: %d %s", code, errs)
	}
	mustBeRegularWith(t, filepath.Join(dest, "src", "a.txt"), "alpha")
}

// A safe relative archive pathname retains the caller's spelling rather than
// being cleaned: "./sub/dir" stays "./sub/dir", and copy mode recreates the
// full operand path under the destination.
func TestOperandSpellingIsPreservedAsArchivePath(t *testing.T) {
	d := t.TempDir()
	if err := os.MkdirAll(filepath.Join(d, "sub", "dir"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(d, "sub", "dir", "f.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	arc := filepath.Join(d, "spell.arc")
	if _, errs, code := exec(t, d, "", "-w", "-f", arc, "./sub/dir"); code != 0 {
		t.Fatalf("write failed: %d %s", code, errs)
	}
	out, _, code := exec(t, d, "", "-f", arc)
	if code != 0 {
		t.Fatal("list failed")
	}
	for _, want := range []string{"./sub/dir/", "./sub/dir/f.txt"} {
		if !strings.Contains(out, want) {
			t.Errorf("expected the operand spelling %q in the member list, got %q", want, out)
		}
	}

	dest := t.TempDir()
	if _, errs, code := exec(t, d, "", "-r", "-w", "sub/dir", dest); code != 0 {
		t.Fatalf("copy failed: %d %s", code, errs)
	}
	mustBeRegularWith(t, filepath.Join(dest, "sub", "dir", "f.txt"), "x")
}

// Absolute and parent-escaping operands retain the pre-walker basename
// behavior. They must never create an archive member that the fail-closed
// reader rejects, especially because copy mode is implemented by piping the
// writer into that reader.
func TestUnsafeOperandSpellingUsesSafeBasename(t *testing.T) {
	d := t.TempDir()
	src := filepath.Join(d, "src")
	if err := os.Mkdir(src, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(src, "f.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	for _, tc := range []struct {
		name    string
		workdir string
		operand string
	}{
		{"absolute", d, src},
		{"parent", filepath.Join(d, "child"), "../src"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := os.MkdirAll(tc.workdir, 0o755); err != nil {
				t.Fatal(err)
			}
			dest := t.TempDir()
			if _, errs, code := exec(t, tc.workdir, "", "-r", "-w", tc.operand, dest); code != 0 {
				t.Fatalf("copy %q failed: %d %s", tc.operand, code, errs)
			}
			mustBeRegularWith(t, filepath.Join(dest, "src", "f.txt"), "x")
		})
	}

	arc := filepath.Join(d, "absolute.arc")
	if _, errs, code := exec(t, d, "", "-w", "-f", arc, src); code != 0 {
		t.Fatalf("archive absolute operand failed: %d %s", code, errs)
	}
	listed, errs, code := exec(t, d, "", "-f", arc)
	if code != 0 || !strings.Contains(listed, "src/f.txt") || strings.Contains(listed, filepath.ToSlash(src)) {
		t.Fatalf("absolute operand members = (%q, %q, %d)", listed, errs, code)
	}
	extract := t.TempDir()
	if _, errs, code := exec(t, extract, "", "-r", "-f", arc); code != 0 {
		t.Fatalf("extract own absolute-operand archive failed: %d %s", code, errs)
	}
	mustBeRegularWith(t, filepath.Join(extract, "src", "f.txt"), "x")
}
