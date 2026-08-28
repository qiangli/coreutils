package lncmd

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/qiangli/coreutils/tool"
)

// runTool is the canonical test harness shape for cmds packages.
func runTool(t *testing.T, dir string, args ...string) (stdout, stderr string, code int) {
	t.Helper()
	return runToolInEnv(t, dir, "", nil, args...)
}

func runToolIn(t *testing.T, dir, input string, args ...string) (stdout, stderr string, code int) {
	t.Helper()
	return runToolInEnv(t, dir, input, nil, args...)
}

func runToolInEnv(t *testing.T, dir, input string, env []string, args ...string) (stdout, stderr string, code int) {
	t.Helper()
	var out, errb bytes.Buffer
	rc := &tool.RunContext{
		Ctx:   context.Background(),
		Dir:   dir,
		Env:   env,
		Stdio: tool.Stdio{In: strings.NewReader(input), Out: &out, Err: &errb},
	}
	code = cmd.Run(rc, args)
	return out.String(), errb.String(), code
}

func TestLnPOSIXStopsOptionParsingAtFirstOperand(t *testing.T) {
	requireSymlinks(t)
	for _, value := range []string{"", "1"} {
		t.Run("POSIXLY_CORRECT="+value, func(t *testing.T) {
			for _, linkName := range []string{"-f", "--"} {
				t.Run(linkName, func(t *testing.T) {
					dir := t.TempDir()
					_, errOut, code := runToolInEnv(t, dir, "", []string{"POSIXLY_CORRECT=" + value}, "-s", "target", linkName)
					if code != 0 || errOut != "" {
						t.Fatalf("ln -s target %s = stderr %q code %d", linkName, errOut, code)
					}
					if target, err := os.Readlink(filepath.Join(dir, linkName)); err != nil || target != "target" {
						t.Fatalf("post-operand %s link target = %q err=%v", linkName, target, err)
					}
				})
			}
		})
	}
}

func TestLnPOSIXPostOperandForceNameDoesNotReplaceExistingFile(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, "-f"), "original")
	_, errOut, code := runToolInEnv(t, dir, "", []string{"POSIXLY_CORRECT=1"}, "-s", "target", "-f")
	if code == 0 || errOut == "" {
		t.Fatalf("ln -s target -f with existing destination = stderr %q code %d", errOut, code)
	}
	if got, err := os.ReadFile(filepath.Join(dir, "-f")); err != nil || string(got) != "original" {
		t.Fatalf("literal -f destination was replaced: content=%q err=%v", got, err)
	}
}

func TestLnPOSIXPostOperandPhysicalNameDoesNotChangeOptionOrder(t *testing.T) {
	requireSymlinks(t)
	dir := t.TempDir()
	write(t, filepath.Join(dir, "real"), "content")
	if err := os.Symlink("real", filepath.Join(dir, "source")); err != nil {
		t.Fatal(err)
	}
	_, errOut, code := runToolInEnv(t, dir, "", []string{"POSIXLY_CORRECT=1"}, "-PL", "source", "-P")
	if code != 0 || errOut != "" {
		t.Fatalf("ln -PL source -P = stderr %q code %d", errOut, code)
	}
	if target, err := os.Readlink(filepath.Join(dir, "-P")); err == nil {
		t.Fatalf("post-operand -P changed -PL ordering and linked the symlink (%q)", target)
	}
	if got, err := os.ReadFile(filepath.Join(dir, "-P")); err != nil || string(got) != "content" {
		t.Fatalf("logical hard link content=%q err=%v", got, err)
	}
}

func TestLnGNUParsingRemainsInterspersed(t *testing.T) {
	requireSymlinks(t)
	dir := t.TempDir()
	_, errOut, code := runTool(t, dir, "target", "-s", "link")
	if code != 0 || errOut != "" {
		t.Fatalf("GNU interspersed -s = stderr %q code %d", errOut, code)
	}
	if target, err := os.Readlink(filepath.Join(dir, "link")); err != nil || target != "target" {
		t.Fatalf("interspersed symbolic link target = %q err=%v", target, err)
	}
}

// requireSymlinks skips on platforms where the test user cannot create
// symlinks (Windows without Developer Mode).
func requireSymlinks(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	if err := os.Symlink("probe-target", filepath.Join(dir, "probe")); err != nil {
		t.Skipf("symlinks not available: %v", err)
	}
}

func write(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestLnHard(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, "a.txt"), "hello")
	_, errb, code := runTool(t, dir, "a.txt", "b.txt")
	if code != 0 || errb != "" {
		t.Fatalf("ln a b: code=%d err=%q", code, errb)
	}
	got, err := os.ReadFile(filepath.Join(dir, "b.txt"))
	if err != nil || string(got) != "hello" {
		t.Errorf("link content=%q err=%v", got, err)
	}
}

func TestLnSymbolic(t *testing.T) {
	requireSymlinks(t)
	dir := t.TempDir()
	write(t, filepath.Join(dir, "a.txt"), "hello")
	_, errb, code := runTool(t, dir, "-s", "a.txt", "l")
	if code != 0 || errb != "" {
		t.Fatalf("ln -s: code=%d err=%q", code, errb)
	}
	// The link stores the target verbatim.
	target, err := os.Readlink(filepath.Join(dir, "l"))
	if err != nil || target != "a.txt" {
		t.Errorf("readlink=%q err=%v", target, err)
	}
}

func TestLnSingleOperand(t *testing.T) {
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	write(t, filepath.Join(dir, "sub", "a.txt"), "x")
	_, errb, code := runTool(t, dir, "sub/a.txt")
	if code != 0 || errb != "" {
		t.Fatalf("ln TARGET: code=%d err=%q", code, errb)
	}
	if _, err := os.Stat(filepath.Join(dir, "a.txt")); err != nil {
		t.Errorf("basename link missing: %v", err)
	}
}

func TestLnIntoDirectory(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, "a1"), "1")
	write(t, filepath.Join(dir, "a2"), "2")
	if err := os.Mkdir(filepath.Join(dir, "d"), 0o755); err != nil {
		t.Fatal(err)
	}
	_, errb, code := runTool(t, dir, "a1", "a2", "d")
	if code != 0 || errb != "" {
		t.Fatalf("ln a1 a2 d: code=%d err=%q", code, errb)
	}
	for _, n := range []string{"a1", "a2"} {
		if _, err := os.Stat(filepath.Join(dir, "d", n)); err != nil {
			t.Errorf("d/%s missing: %v", n, err)
		}
	}
}

func TestLnTargetDirectoryOption(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, "a1"), "1")
	write(t, filepath.Join(dir, "a2"), "2")
	if err := os.Mkdir(filepath.Join(dir, "d"), 0o755); err != nil {
		t.Fatal(err)
	}
	_, errb, code := runTool(t, dir, "-t", "d", "a1", "a2")
	if code != 0 || errb != "" {
		t.Fatalf("ln -t d a1 a2: code=%d err=%q", code, errb)
	}
	for _, n := range []string{"a1", "a2"} {
		if _, err := os.Stat(filepath.Join(dir, "d", n)); err != nil {
			t.Errorf("d/%s missing: %v", n, err)
		}
	}
}

func TestLnNoTargetDirectory(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, "a"), "a")
	if err := os.Mkdir(filepath.Join(dir, "d"), 0o755); err != nil {
		t.Fatal(err)
	}
	_, errb, code := runTool(t, dir, "-T", "a", "d")
	if code != 1 || !strings.Contains(errb, "failed to create hard link") {
		t.Errorf("ln -T a d: code=%d err=%q", code, errb)
	}
}

func TestLnNoDereferenceDestinationSymlink(t *testing.T) {
	requireSymlinks(t)
	dir := t.TempDir()
	write(t, filepath.Join(dir, "a"), "a")
	if err := os.Mkdir(filepath.Join(dir, "real"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("real", filepath.Join(dir, "linkdir")); err != nil {
		t.Fatal(err)
	}
	_, errb, code := runTool(t, dir, "-n", "a", "linkdir")
	if code != 1 || !strings.Contains(errb, "failed to create hard link") {
		t.Errorf("ln -n a linkdir: code=%d err=%q", code, errb)
	}
	if _, err := os.Lstat(filepath.Join(dir, "real", "a")); !os.IsNotExist(err) {
		t.Errorf("ln -n unexpectedly linked inside symlinked directory: %v", err)
	}
}

func TestLnRelativeSymbolic(t *testing.T) {
	requireSymlinks(t)
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "src"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "links", "deep"), 0o755); err != nil {
		t.Fatal(err)
	}
	write(t, filepath.Join(dir, "src", "file"), "x")
	_, errb, code := runTool(t, dir, "-sr", "src/file", "links/deep/file-link")
	if code != 0 || errb != "" {
		t.Fatalf("ln -sr: code=%d err=%q", code, errb)
	}
	target, err := os.Readlink(filepath.Join(dir, "links", "deep", "file-link"))
	want := filepath.Join("..", "..", "src", "file")
	if err != nil || target != want {
		t.Errorf("relative symlink target=%q err=%v, want %q", target, err, want)
	}
}

func TestLnForce(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, "a"), "a")
	write(t, filepath.Join(dir, "b"), "b")
	// Without -f: destination exists -> failure.
	_, errb, code := runTool(t, dir, "a", "b")
	if code != 1 || !strings.Contains(errb, "failed to create hard link") {
		t.Errorf("no -f: code=%d err=%q", code, errb)
	}
	// With -f: replaced.
	_, errb, code = runTool(t, dir, "-f", "a", "b")
	if code != 0 || errb != "" {
		t.Fatalf("-f: code=%d err=%q", code, errb)
	}
	got, _ := os.ReadFile(filepath.Join(dir, "b"))
	if string(got) != "a" {
		t.Errorf("after -f content=%q", got)
	}
}

// POSIX Issue 7 requires -f replacement to perform the equivalent of
// unlink(2). In particular, an empty directory at the destination must not be
// removed (os.Remove would fall back to rmdir and is therefore too broad).
func TestLnForceDoesNotRemoveDestinationDirectory(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, "src"), "source")
	if err := os.MkdirAll(filepath.Join(dir, "dest", "src"), 0o755); err != nil {
		t.Fatal(err)
	}

	_, errb, code := runTool(t, dir, "-f", "src", "dest")
	if code != 1 || !strings.Contains(errb, "cannot remove 'dest/src'") {
		t.Fatalf("ln -f src dest: code=%d err=%q, want unlink diagnostic", code, errb)
	}
	fi, err := os.Lstat(filepath.Join(dir, "dest", "src"))
	if err != nil || !fi.IsDir() {
		t.Fatalf("destination directory was removed or replaced: mode=%v err=%v", fi, err)
	}
}

// The required -f sequence is unlink first, link second, then continue with
// later source operands after an error. A missing source therefore still
// removes its old non-identical destination before the hard-link failure.
func TestLnForceRemovalPrecedesLinkErrorAndProcessingContinues(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, "good"), "good")
	if err := os.Mkdir(filepath.Join(dir, "dest"), 0o755); err != nil {
		t.Fatal(err)
	}
	write(t, filepath.Join(dir, "dest", "missing"), "old")

	_, errb, code := runTool(t, dir, "-f", "missing", "good", "dest")
	if code != 1 || !strings.Contains(errb, "dest/missing") {
		t.Fatalf("ln -f missing good dest: code=%d err=%q", code, errb)
	}
	if _, err := os.Lstat(filepath.Join(dir, "dest", "missing")); !os.IsNotExist(err) {
		t.Errorf("old destination remains after required unlink-before-link ordering: %v", err)
	}
	if got, err := os.ReadFile(filepath.Join(dir, "dest", "good")); err != nil || string(got) != "good" {
		t.Errorf("later source was not processed: content=%q err=%v", got, err)
	}
}

func TestLnBackupAndSuffix(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, "src"), "new")
	write(t, filepath.Join(dir, "dest"), "old")
	_, errb, code := runTool(t, dir, "-b", "-S", ".bak", "src", "dest")
	if code != 0 || errb != "" {
		t.Fatalf("ln -b -S: code=%d err=%q", code, errb)
	}
	got, _ := os.ReadFile(filepath.Join(dir, "dest"))
	if string(got) != "new" {
		t.Errorf("dest content=%q", got)
	}
	backup, err := os.ReadFile(filepath.Join(dir, "dest.bak"))
	if err != nil || string(backup) != "old" {
		t.Errorf("backup content=%q err=%v", backup, err)
	}
}

func TestLnBackupExistingUsesNumberedWhenPresent(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, "src"), "new")
	write(t, filepath.Join(dir, "dest"), "old")
	write(t, filepath.Join(dir, "dest.~1~"), "older")
	_, errb, code := runTool(t, dir, "--backup", "src", "dest")
	if code != 0 || errb != "" {
		t.Fatalf("ln --backup: code=%d err=%q", code, errb)
	}
	backup, err := os.ReadFile(filepath.Join(dir, "dest.~2~"))
	if err != nil || string(backup) != "old" {
		t.Errorf("numbered backup content=%q err=%v", backup, err)
	}
}

func TestLnInteractiveAcceptsAndDeclines(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, "src"), "new")
	write(t, filepath.Join(dir, "dest"), "old")
	_, errb, code := runToolIn(t, dir, "", "-i", "src", "dest")
	if code != 0 || !strings.Contains(errb, "replace 'dest'?") || strings.Contains(errb, "cannot read response") {
		t.Fatalf("ln -i default EOF: code=%d err=%q", code, errb)
	}
	got, _ := os.ReadFile(filepath.Join(dir, "dest"))
	if string(got) != "old" {
		t.Errorf("declined replacement content=%q", got)
	}
	_, errb, code = runToolIn(t, dir, "n\n", "-i", "src", "dest")
	if code != 0 || !strings.Contains(errb, "replace 'dest'?") {
		t.Fatalf("ln -i decline: code=%d err=%q", code, errb)
	}
	got, _ = os.ReadFile(filepath.Join(dir, "dest"))
	if string(got) != "old" {
		t.Errorf("declined replacement content=%q", got)
	}
	_, errb, code = runToolIn(t, dir, "y\n", "-i", "src", "dest")
	if code != 0 || !strings.Contains(errb, "replace 'dest'?") {
		t.Fatalf("ln -i accept: code=%d err=%q", code, errb)
	}
	got, _ = os.ReadFile(filepath.Join(dir, "dest"))
	if string(got) != "new" {
		t.Errorf("accepted replacement content=%q", got)
	}
}

func TestLnForceAndInteractiveLastOptionWins(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, "src"), "new")
	write(t, filepath.Join(dir, "dest"), "old")
	_, errb, code := runToolIn(t, dir, "n\n", "-f", "-i", "src", "dest")
	if code != 0 || !strings.Contains(errb, "replace 'dest'?") {
		t.Fatalf("ln -f -i: code=%d err=%q", code, errb)
	}
	got, _ := os.ReadFile(filepath.Join(dir, "dest"))
	if string(got) != "old" {
		t.Errorf("-f -i should obey prompt, got %q", got)
	}
	_, errb, code = runToolIn(t, dir, "n\n", "-i", "-f", "src", "dest")
	if code != 0 || strings.Contains(errb, "replace 'dest'?") {
		t.Fatalf("ln -i -f: code=%d err=%q", code, errb)
	}
	got, _ = os.ReadFile(filepath.Join(dir, "dest"))
	if string(got) != "new" {
		t.Errorf("-i -f should force replacement, got %q", got)
	}
}

func TestLnLogicalAndPhysicalSourceSymlink(t *testing.T) {
	requireSymlinks(t)
	dir := t.TempDir()
	write(t, filepath.Join(dir, "real"), "real")
	if err := os.Symlink("real", filepath.Join(dir, "sym")); err != nil {
		t.Fatal(err)
	}
	_, errb, code := runTool(t, dir, "-L", "sym", "logical")
	if code != 0 || errb != "" {
		t.Fatalf("ln -L: code=%d err=%q", code, errb)
	}
	if target, err := os.Readlink(filepath.Join(dir, "logical")); err == nil {
		t.Fatalf("ln -L created symlink hard link to %q, want regular file", target)
	}
	got, _ := os.ReadFile(filepath.Join(dir, "logical"))
	if string(got) != "real" {
		t.Errorf("logical content=%q", got)
	}
	_, errb, code = runTool(t, dir, "-P", "sym", "physical")
	if code != 0 || errb != "" {
		t.Fatalf("ln -P: code=%d err=%q", code, errb)
	}
	target, err := os.Readlink(filepath.Join(dir, "physical"))
	if err != nil || target != "real" {
		t.Errorf("physical readlink=%q err=%v", target, err)
	}
}

// POSIX Issue 7: the -L/-P default for a symlink source_file is
// implementation-defined; this implementation documents -P (link to the
// symlink itself) on every platform.  darwin's link(2) follows symlinks,
// so this pins that the default does not silently become -L there.
func TestLnDefaultHardLinkToSymlinkIsPhysical(t *testing.T) {
	requireSymlinks(t)
	dir := t.TempDir()
	write(t, filepath.Join(dir, "real"), "real")
	if err := os.Symlink("real", filepath.Join(dir, "sym")); err != nil {
		t.Fatal(err)
	}
	_, errb, code := runTool(t, dir, "sym", "out")
	if code != 0 || errb != "" {
		t.Fatalf("ln sym out: code=%d err=%q", code, errb)
	}
	target, err := os.Readlink(filepath.Join(dir, "out"))
	if err != nil || target != "real" {
		t.Errorf("default hard link readlink=%q err=%v, want symlink to 'real' (-P default)", target, err)
	}
}

// POSIX Issue 7: specifying both -L and -P is not an error; the last one
// specified determines the behavior.
func TestLnLastOfLogicalPhysicalWins(t *testing.T) {
	requireSymlinks(t)
	dir := t.TempDir()
	write(t, filepath.Join(dir, "real"), "real")
	if err := os.Symlink("real", filepath.Join(dir, "sym")); err != nil {
		t.Fatal(err)
	}
	// -L then -P: physical wins -> new name is itself a symlink.
	_, errb, code := runTool(t, dir, "-L", "-P", "sym", "lp")
	if code != 0 || errb != "" {
		t.Fatalf("ln -L -P: code=%d err=%q", code, errb)
	}
	if target, err := os.Readlink(filepath.Join(dir, "lp")); err != nil || target != "real" {
		t.Errorf("-L -P readlink=%q err=%v, want physical link to symlink", target, err)
	}
	// -P then -L: logical wins -> new name is a hard link to the referenced file.
	_, errb, code = runTool(t, dir, "-P", "-L", "sym", "pl")
	if code != 0 || errb != "" {
		t.Fatalf("ln -P -L: code=%d err=%q", code, errb)
	}
	if target, err := os.Readlink(filepath.Join(dir, "pl")); err == nil {
		t.Fatalf("-P -L created link to symlink (%q), want dereferenced file", target)
	}
	if got, _ := os.ReadFile(filepath.Join(dir, "pl")); string(got) != "real" {
		t.Errorf("-P -L content=%q", got)
	}
	// Combined short form: -PL is equivalent to -P -L.
	_, errb, code = runTool(t, dir, "-PL", "sym", "combined")
	if code != 0 || errb != "" {
		t.Fatalf("ln -PL: code=%d err=%q", code, errb)
	}
	if target, err := os.Readlink(filepath.Join(dir, "combined")); err == nil {
		t.Fatalf("-PL created link to symlink (%q), want dereferenced file", target)
	}
}

// POSIX Issue 7: if -s is specified, -L and -P are silently ignored.
func TestLnSymbolicIgnoresLogicalPhysical(t *testing.T) {
	requireSymlinks(t)
	dir := t.TempDir()
	write(t, filepath.Join(dir, "real"), "real")
	if err := os.Symlink("real", filepath.Join(dir, "sym")); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{
		{"-s", "-L", "sym", "outL"},
		{"-s", "-P", "sym", "outP"},
		{"-sLP", "sym", "outLP"},
	} {
		_, errb, code := runTool(t, dir, args...)
		if code != 0 || errb != "" {
			t.Fatalf("ln %v: code=%d err=%q", args, code, errb)
		}
		out := args[len(args)-1]
		if target, err := os.Readlink(filepath.Join(dir, out)); err != nil || target != "sym" {
			t.Errorf("ln %v readlink=%q err=%v, want verbatim 'sym'", args, target, err)
		}
	}
}

// POSIX Issue 7 -s: source_file need not exist; a dangling target and even
// a self-referential name are stored verbatim.
func TestLnSymbolicDanglingSource(t *testing.T) {
	requireSymlinks(t)
	dir := t.TempDir()
	_, errb, code := runTool(t, dir, "-s", "no-such-file", "emptysymlink")
	if code != 0 || errb != "" {
		t.Fatalf("ln -s dangling: code=%d err=%q", code, errb)
	}
	if target, err := os.Readlink(filepath.Join(dir, "emptysymlink")); err != nil || target != "no-such-file" {
		t.Errorf("dangling readlink=%q err=%v", target, err)
	}
	if _, err := os.Stat(filepath.Join(dir, "emptysymlink")); !os.IsNotExist(err) {
		t.Errorf("dangling link unexpectedly resolves: %v", err)
	}
	// Self-loop: the destination does not exist yet, so the same-entry
	// diagnostic must not fire and the link is created.
	_, errb, code = runTool(t, dir, "-s", "a", "a")
	if code != 0 || errb != "" {
		t.Fatalf("ln -s a a (absent): code=%d err=%q", code, errb)
	}
	if target, err := os.Readlink(filepath.Join(dir, "a")); err != nil || target != "a" {
		t.Errorf("self-loop readlink=%q err=%v", target, err)
	}
}

// POSIX Issue 7: an existing destination without -f draws a diagnostic,
// processing of that source_file stops, remaining source_files are still
// processed, and the exit status is >0.
func TestLnExistingDestinationDiagnosesAndContinues(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, "s1"), "1")
	write(t, filepath.Join(dir, "s2"), "2")
	if err := os.Mkdir(filepath.Join(dir, "d"), 0o755); err != nil {
		t.Fatal(err)
	}
	write(t, filepath.Join(dir, "d", "s1"), "old")
	_, errb, code := runTool(t, dir, "s1", "s2", "d")
	if code != 1 || !strings.Contains(errb, "d/s1") {
		t.Fatalf("ln s1 s2 d: code=%d err=%q, want diagnostic naming d/s1 and exit 1", code, errb)
	}
	if got, _ := os.ReadFile(filepath.Join(dir, "d", "s1")); string(got) != "old" {
		t.Errorf("existing destination modified without -f: %q", got)
	}
	if got, err := os.ReadFile(filepath.Join(dir, "d", "s2")); err != nil || string(got) != "2" {
		t.Errorf("remaining source not processed: content=%q err=%v", got, err)
	}
}

func TestLnVerbose(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, "a"), "a")
	out, _, code := runTool(t, dir, "-v", "a", "b")
	if code != 0 || out != "'b' => 'a'\n" {
		t.Errorf("hard -v: code=%d out=%q", code, out)
	}
	requireSymlinks(t)
	out, _, code = runTool(t, dir, "-sv", "a", "l")
	if code != 0 || out != "'l' -> 'a'\n" {
		t.Errorf("sym -v: code=%d out=%q", code, out)
	}
}

func TestLnErrors(t *testing.T) {
	dir := t.TempDir()
	_, errb, code := runTool(t, dir)
	if code != 2 || !strings.Contains(errb, "missing file operand") {
		t.Errorf("no args: code=%d err=%q", code, errb)
	}
	write(t, filepath.Join(dir, "a"), "a")
	write(t, filepath.Join(dir, "b"), "b")
	_, errb, code = runTool(t, dir, "a", "b", "nodir")
	if code != 1 || !strings.Contains(errb, "is not a directory") {
		t.Errorf("last not dir: code=%d err=%q", code, errb)
	}
	_, errb, code = runTool(t, dir, "missing-target", "x")
	if code != 1 || !strings.Contains(errb, "failed to create hard link") {
		t.Errorf("missing target: code=%d err=%q", code, errb)
	}
	_, errb, code = runTool(t, dir, "--frobnicate", "a", "b")
	if code != 2 || !strings.Contains(errb, "frobnicate") || !strings.Contains(errb, "pure-Go") {
		t.Errorf("unknown flag: code=%d err=%q", code, errb)
	}
	_, errb, code = runTool(t, dir, "-t", "d", "-T", "a")
	if code != 2 || !strings.Contains(errb, "cannot combine -t and -T") {
		t.Errorf("ln -t -T: code=%d err=%q", code, errb)
	}
	_, errb, code = runTool(t, dir, "-r", "a", "c")
	if code != 2 || !strings.Contains(errb, "--relative can only be used") {
		t.Errorf("ln -r without -s: code=%d err=%q", code, errb)
	}
}

func TestLnSameFile(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, "a"), "keep")

	// Hard link to the same path without -f.
	_, errb, code := runTool(t, dir, "a", "a")
	if code != 1 || !strings.Contains(errb, "'a' and 'a' are the same file") {
		t.Errorf("ln a a: code=%d err=%q", code, errb)
	}
	if got, _ := os.ReadFile(filepath.Join(dir, "a")); string(got) != "keep" {
		t.Errorf("ln a a modified the file: %q", got)
	}

	// Hard link to the same path with -f: POSIX says do nothing and diagnose.
	_, errb, code = runTool(t, dir, "-f", "a", "a")
	if code != 1 || !strings.Contains(errb, "'a' and 'a' are the same file") {
		t.Errorf("ln -f a a: code=%d err=%q", code, errb)
	}
	if got, _ := os.ReadFile(filepath.Join(dir, "a")); string(got) != "keep" {
		t.Errorf("ln -f a a modified the file: %q", got)
	}
}

func TestLnSameFileThroughHardLinkAlias(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, "a"), "keep")
	if err := os.Link(filepath.Join(dir, "a"), filepath.Join(dir, "alias")); err != nil {
		t.Fatal(err)
	}
	_, errb, code := runTool(t, dir, "a", "alias")
	if code != 1 || !strings.Contains(errb, "are the same file") {
		t.Errorf("ln a alias: code=%d err=%q", code, errb)
	}
	_, errb, code = runTool(t, dir, "-f", "a", "alias")
	if code != 1 || !strings.Contains(errb, "are the same file") {
		t.Errorf("ln -f a alias: code=%d err=%q", code, errb)
	}
	if got, _ := os.ReadFile(filepath.Join(dir, "alias")); string(got) != "keep" {
		t.Errorf("alias modified: %q", got)
	}
}

func TestLnSymbolicSameFile(t *testing.T) {
	requireSymlinks(t)
	dir := t.TempDir()
	write(t, filepath.Join(dir, "a"), "keep")

	// Symbolic link to the same path without -f.
	_, errb, code := runTool(t, dir, "-s", "a", "a")
	if code != 1 || !strings.Contains(errb, "'a' and 'a' are the same file") {
		t.Errorf("ln -s a a: code=%d err=%q", code, errb)
	}
	if got, _ := os.ReadFile(filepath.Join(dir, "a")); string(got) != "keep" {
		t.Errorf("ln -s a a modified the file: %q", got)
	}

	// Symbolic link to the same path with -f: must not create a self-loop.
	_, errb, code = runTool(t, dir, "-sf", "a", "a")
	if code != 1 || !strings.Contains(errb, "'a' and 'a' are the same file") {
		t.Errorf("ln -sf a a: code=%d err=%q", code, errb)
	}
	if got, _ := os.ReadFile(filepath.Join(dir, "a")); string(got) != "keep" {
		t.Errorf("ln -sf a a modified the file: %q", got)
	}
}

func TestLnSameFileDirectoryForm(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, "a"), "keep")

	// Linking a file into the current directory produces the same destination.
	_, errb, code := runTool(t, dir, "a", ".")
	if code != 1 || !strings.Contains(errb, "'a' and 'a' are the same file") {
		t.Errorf("ln a .: code=%d err=%q", code, errb)
	}
	if got, _ := os.ReadFile(filepath.Join(dir, "a")); string(got) != "keep" {
		t.Errorf("ln a . modified the file: %q", got)
	}

	// Same through -t.
	_, errb, code = runTool(t, dir, "-t", ".", "a")
	if code != 1 || !strings.Contains(errb, "'a' and 'a' are the same file") {
		t.Errorf("ln -t . a: code=%d err=%q", code, errb)
	}
	if got, _ := os.ReadFile(filepath.Join(dir, "a")); string(got) != "keep" {
		t.Errorf("ln -t . a modified the file: %q", got)
	}
}

func TestLnHelpAndVersion(t *testing.T) {
	out, _, code := runTool(t, t.TempDir(), "--help")
	if code != 0 || !strings.Contains(out, "Usage: ln") {
		t.Errorf("--help: code=%d out=%q", code, out)
	}
	out, _, code = runTool(t, t.TempDir(), "--version")
	if code != 0 || !strings.Contains(out, "ln") {
		t.Errorf("--version: code=%d out=%q", code, out)
	}
	out, _, code = runTool(t, t.TempDir(), "-V")
	if code != 0 || !strings.Contains(out, "ln") {
		t.Errorf("-V: code=%d out=%q", code, out)
	}
}

// lnPanicReader proves a code path never reads standard input: POSIX Issue 7
// documents ln STDIN as "Not used" when -i is not specified.
type lnPanicReader struct{ t *testing.T }

func (r lnPanicReader) Read([]byte) (int, error) {
	r.t.Helper()
	r.t.Fatal("ln must not read standard input without -i")
	return 0, nil
}

// TestLnPOSIXStdinNotUsed covers the POSIX Issue 7 STDIN requirement ("Not
// used") across hard link, symbolic link, and error paths. Without the GNU -i
// extension, ln must never touch standard input.
func TestLnPOSIXStdinNotUsed(t *testing.T) {
	requireSymlinks(t)
	dir := t.TempDir()
	write(t, filepath.Join(dir, "a"), "a")
	write(t, filepath.Join(dir, "exist"), "old")

	run := func(args ...string) (stdout, stderr string, code int) {
		t.Helper()
		var out, errb bytes.Buffer
		rc := &tool.RunContext{
			Ctx:   context.Background(),
			Dir:   dir,
			Stdio: tool.Stdio{In: lnPanicReader{t}, Out: &out, Err: &errb},
		}
		code = cmd.Run(rc, args)
		return out.String(), errb.String(), code
	}
	// Successful hard link.
	if _, _, code := run("a", "b"); code != 0 {
		t.Errorf("hard link: code=%d", code)
	}
	// Successful symbolic link.
	if _, _, code := run("-s", "a", "sym"); code != 0 {
		t.Errorf("symbolic link: code=%d", code)
	}
	// Error: existing destination without -f.
	if _, errb, code := run("a", "exist"); code != 1 || errb == "" {
		t.Errorf("existing dest: code=%d err=%q", code, errb)
	}
	// Error: missing target.
	if _, errb, code := run("no-such", "out"); code != 1 || errb == "" {
		t.Errorf("missing target: code=%d err=%q", code, errb)
	}
}

// TestLnPOSIXStdoutNotUsed verifies that without the GNU -v extension, ln
// produces no standard output, matching the POSIX Issue 7 STDOUT requirement
// ("Not used").
func TestLnPOSIXStdoutNotUsed(t *testing.T) {
	requireSymlinks(t)
	dir := t.TempDir()
	write(t, filepath.Join(dir, "a"), "a")
	if err := os.Mkdir(filepath.Join(dir, "d"), 0o755); err != nil {
		t.Fatal(err)
	}
	// Hard link (two-operand form).
	out, _, code := runTool(t, dir, "a", "b")
	if code != 0 || out != "" {
		t.Errorf("hard link stdout=%q code=%d", out, code)
	}
	// Symbolic link (two-operand form).
	out, _, code = runTool(t, dir, "-s", "a", "sym")
	if code != 0 || out != "" {
		t.Errorf("sym link stdout=%q code=%d", out, code)
	}
	// Directory form (multi-operand).
	out, _, code = runTool(t, dir, "-sf", "a", "d")
	if code != 0 || out != "" {
		t.Errorf("dir form stdout=%q code=%d", out, code)
	}
}

// lnErrWriter simulates a broken standard error stream.
type lnErrWriter struct{}

func (lnErrWriter) Write([]byte) (int, error) {
	return 0, os.ErrClosed
}

// TestLnDiagnosticWriteFailureStillFails verifies that a broken standard
// error stream does not mask an operand failure: exit status must still
// reflect the failed link even though the diagnostic itself could not be
// written. This covers POSIX's "Consequences of Errors: Default."
func TestLnDiagnosticWriteFailureStillFails(t *testing.T) {
	dir := t.TempDir()
	rc := &tool.RunContext{
		Ctx:   context.Background(),
		Dir:   dir,
		Stdio: tool.Stdio{In: strings.NewReader(""), Out: &bytes.Buffer{}, Err: lnErrWriter{}},
	}
	code := cmd.Run(rc, []string{"no-such-target", "output"})
	if code != 1 {
		t.Errorf("ln with broken stderr: code=%d, want 1", code)
	}
}

// TestLnPOSIXMoreThanTwoOperandsNonDir covers the POSIX requirement: "if more
// than two operands are specified and the final is not an existing directory,
// an error shall result."
func TestLnPOSIXMoreThanTwoOperandsNonDir(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, "a"), "a")
	write(t, filepath.Join(dir, "b"), "b")
	_, errb, code := runTool(t, dir, "a", "b", "not-a-dir")
	if code != 1 || !strings.Contains(errb, "is not a directory") {
		t.Errorf("ln a b not-a-dir: code=%d err=%q", code, errb)
	}
}

// TestLnPOSIXForceUnlinkFailureDiagnosesAndContinues covers the POSIX step:
// "Actions shall be performed equivalent to the unlink() function ... If this
// fails for any reason, ln shall write a diagnostic message to standard error,
// do nothing more with the current source_file, and go on to any remaining
// source_files."
func TestLnPOSIXForceUnlinkFailureDiagnosesAndContinues(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, "src1"), "1")
	write(t, filepath.Join(dir, "src2"), "2")
	if err := os.Mkdir(filepath.Join(dir, "d"), 0o755); err != nil {
		t.Fatal(err)
	}
	// Create a destination directory at d/src1 — unlink will fail because
	// POSIX unlink does not remove directories.
	if err := os.MkdirAll(filepath.Join(dir, "d", "src1"), 0o755); err != nil {
		t.Fatal(err)
	}
	_, errb, code := runTool(t, dir, "-f", "src1", "src2", "d")
	// src1 fails (unlink of directory), src2 succeeds.
	if code != 1 || !strings.Contains(errb, "src1") {
		t.Errorf("code=%d err=%q", code, errb)
	}
	// src2 was still linked into d.
	if got, err := os.ReadFile(filepath.Join(dir, "d", "src2")); err != nil || string(got) != "2" {
		t.Errorf("remaining source not processed: content=%q err=%v", got, err)
	}
}
