package tarcmd

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/qiangli/coreutils/tool"
)

// runTool is the canonical test harness shape for cmds packages
// (output captured AFTER Run).
func runTool(t *testing.T, dir string, stdin io.Reader, args ...string) (stdout, stderr string, code int) {
	t.Helper()
	if stdin == nil {
		stdin = strings.NewReader("")
	}
	var out, errb bytes.Buffer
	rc := &tool.RunContext{
		Ctx:   context.Background(),
		Dir:   dir,
		Stdio: tool.Stdio{In: stdin, Out: &out, Err: &errb},
	}
	code = cmd.Run(rc, args)
	return out.String(), errb.String(), code
}

// makeTree builds the source tree used by the roundtrip tests and
// returns the fixed mtime stamped on src/a.txt.
func makeTree(t *testing.T, dir string) time.Time {
	t.Helper()
	src := filepath.Join(dir, "src")
	if err := os.MkdirAll(filepath.Join(src, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(src, "a.txt"), []byte("alpha\n"), 0o640); err != nil {
		t.Fatal(err)
	}
	// WriteFile perms are umask-filtered; pin the mode explicitly.
	if err := os.Chmod(filepath.Join(src, "a.txt"), 0o640); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(src, "sub", "b.txt"), []byte("beta\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	mtime := time.Date(2024, 3, 14, 15, 9, 26, 0, time.UTC)
	if err := os.Chtimes(filepath.Join(src, "a.txt"), mtime, mtime); err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" {
		if err := os.Symlink("a.txt", filepath.Join(src, "link")); err != nil {
			t.Fatal(err)
		}
	}
	return mtime
}

func TestCreateListExtractRoundtrip(t *testing.T) {
	dir := t.TempDir()
	mtime := makeTree(t, dir)

	_, errb, code := runTool(t, dir, nil, "-cf", "a.tar", "src")
	if code != 0 {
		t.Fatalf("create: code=%d err=%q", code, errb)
	}

	out, _, code := runTool(t, dir, nil, "-tf", "a.tar")
	if code != 0 {
		t.Fatalf("list: code=%d", code)
	}
	for _, want := range []string{"src/\n", "src/a.txt\n", "src/sub/\n", "src/sub/b.txt\n"} {
		if !strings.Contains(out, want) {
			t.Errorf("list missing %q in:\n%s", want, out)
		}
	}
	if runtime.GOOS != "windows" && !strings.Contains(out, "src/link\n") {
		t.Errorf("list missing symlink entry:\n%s", out)
	}

	dest := filepath.Join(dir, "dest")
	if err := os.Mkdir(dest, 0o755); err != nil {
		t.Fatal(err)
	}
	_, errb, code = runTool(t, dir, nil, "-xf", "a.tar", "-C", "dest")
	if code != 0 {
		t.Fatalf("extract: code=%d err=%q", code, errb)
	}
	got, err := os.ReadFile(filepath.Join(dest, "src", "a.txt"))
	if err != nil || string(got) != "alpha\n" {
		t.Fatalf("extracted a.txt = %q, %v", got, err)
	}
	got, err = os.ReadFile(filepath.Join(dest, "src", "sub", "b.txt"))
	if err != nil || string(got) != "beta\n" {
		t.Fatalf("extracted b.txt = %q, %v", got, err)
	}
	fi, err := os.Stat(filepath.Join(dest, "src", "a.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if !fi.ModTime().Equal(mtime) {
		t.Errorf("mtime not preserved: got %v want %v", fi.ModTime(), mtime)
	}
	if runtime.GOOS != "windows" {
		if perm := fi.Mode().Perm(); perm != 0o640 {
			t.Errorf("mode not preserved: got %o want 640", perm)
		}
		target, err := os.Readlink(filepath.Join(dest, "src", "link"))
		if err != nil || target != "a.txt" {
			t.Errorf("symlink not preserved: %q, %v", target, err)
		}
	}
}

func TestGzipRoundtripAndOldStyle(t *testing.T) {
	dir := t.TempDir()
	makeTree(t, dir)

	// old option style: bundled letters without dash, f consumes operand
	_, errb, code := runTool(t, dir, nil, "czf", "a.tgz", "src")
	if code != 0 {
		t.Fatalf("old-style czf: code=%d err=%q", code, errb)
	}
	out, _, code := runTool(t, dir, nil, "tzf", "a.tgz")
	if code != 0 || !strings.Contains(out, "src/a.txt\n") {
		t.Fatalf("old-style tzf: code=%d out=%q", code, out)
	}

	// gzip auto-detection on read (no -z)
	out, _, code = runTool(t, dir, nil, "-tf", "a.tgz")
	if code != 0 || !strings.Contains(out, "src/a.txt\n") {
		t.Fatalf("auto-detect gzip: code=%d out=%q", code, out)
	}

	dest := filepath.Join(dir, "dest")
	_, errb, code = runTool(t, dir, nil, "-xzf", "a.tgz", "-C", "dest")
	if code == 0 {
		t.Fatalf("extract to missing -C dir should fail")
	}
	if err := os.Mkdir(dest, 0o755); err != nil {
		t.Fatal(err)
	}
	_, errb, code = runTool(t, dir, nil, "-xzf", "a.tgz", "-C", "dest")
	if code != 0 {
		t.Fatalf("extract -z: code=%d err=%q", code, errb)
	}
	if got, err := os.ReadFile(filepath.Join(dest, "src", "sub", "b.txt")); err != nil || string(got) != "beta\n" {
		t.Fatalf("extracted b.txt = %q, %v", got, err)
	}
}

func TestListFromStdinAndCreateToStdout(t *testing.T) {
	dir := t.TempDir()
	makeTree(t, dir)

	// create to stdout (-f -); verbose goes to stderr then
	out, errb, code := runTool(t, dir, nil, "-cvf", "-", "src")
	if code != 0 {
		t.Fatalf("create to stdout: code=%d", code)
	}
	if !strings.Contains(errb, "src/a.txt\n") {
		t.Errorf("-cv with stdout archive should list on stderr, got %q", errb)
	}

	// list the bytes back via stdin
	lst, _, code := runTool(t, dir, strings.NewReader(out), "-tf", "-")
	if code != 0 || !strings.Contains(lst, "src/a.txt\n") {
		t.Fatalf("list from stdin: code=%d out=%q", code, lst)
	}
}

func TestVerboseListing(t *testing.T) {
	dir := t.TempDir()
	makeTree(t, dir)
	if _, _, code := runTool(t, dir, nil, "-cf", "a.tar", "src"); code != 0 {
		t.Fatal("create failed")
	}
	out, _, code := runTool(t, dir, nil, "-tvf", "a.tar")
	if code != 0 {
		t.Fatalf("tv: code=%d", code)
	}
	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
	var aline string
	for _, l := range lines {
		if strings.HasSuffix(l, " src/a.txt") {
			aline = l
		}
		if strings.HasPrefix(l, "d") && !strings.Contains(l, "drwx") {
			t.Errorf("dir line lacks d-perms: %q", l)
		}
	}
	if aline == "" {
		t.Fatalf("no -tv line for src/a.txt in:\n%s", out)
	}
	// permissions owner/group size date name
	if runtime.GOOS != "windows" && !strings.HasPrefix(aline, "-rw-r-----") {
		t.Errorf("perm string: %q", aline)
	}
	if !strings.Contains(aline, " 6 ") { // len("alpha\n")
		t.Errorf("size missing in %q", aline)
	}
	if !strings.Contains(aline, "/") || !strings.Contains(aline, "-03-14 ") && !strings.Contains(aline, "2024-") {
		t.Errorf("date/owner missing in %q", aline)
	}
}

func TestStripComponents(t *testing.T) {
	dir := t.TempDir()
	makeTree(t, dir)
	if _, _, code := runTool(t, dir, nil, "-cf", "a.tar", "src"); code != 0 {
		t.Fatal("create failed")
	}
	dest := filepath.Join(dir, "flat")
	if err := os.Mkdir(dest, 0o755); err != nil {
		t.Fatal(err)
	}
	_, errb, code := runTool(t, dir, nil, "-xf", "a.tar", "-C", "flat", "--strip-components=1")
	if code != 0 {
		t.Fatalf("strip: code=%d err=%q", code, errb)
	}
	if got, err := os.ReadFile(filepath.Join(dest, "a.txt")); err != nil || string(got) != "alpha\n" {
		t.Fatalf("stripped a.txt = %q, %v", got, err)
	}
	if _, err := os.Stat(filepath.Join(dest, "src")); !os.IsNotExist(err) {
		t.Errorf("src/ should not exist after --strip-components=1")
	}
	// only valid with -x
	_, errb, code = runTool(t, dir, nil, "-tf", "a.tar", "--strip-components=1")
	if code != 2 || !strings.Contains(errb, "strip-components") {
		t.Errorf("strip with -t: code=%d err=%q", code, errb)
	}
}

func TestMemberSelection(t *testing.T) {
	dir := t.TempDir()
	makeTree(t, dir)
	if _, _, code := runTool(t, dir, nil, "-cf", "a.tar", "src"); code != 0 {
		t.Fatal("create failed")
	}
	out, _, code := runTool(t, dir, nil, "-tf", "a.tar", "src/a.txt")
	if code != 0 || strings.TrimSpace(out) != "src/a.txt" {
		t.Errorf("member select: code=%d out=%q", code, out)
	}
	// directory operand selects everything beneath
	out, _, code = runTool(t, dir, nil, "-tf", "a.tar", "src/sub")
	if code != 0 || !strings.Contains(out, "src/sub/b.txt\n") {
		t.Errorf("dir member select: code=%d out=%q", code, out)
	}
	_, errb, code := runTool(t, dir, nil, "-tf", "a.tar", "nope")
	if code != 1 || !strings.Contains(errb, "Not found in archive") {
		t.Errorf("missing member: code=%d err=%q", code, errb)
	}
}

// writeRawArchive writes a tar with arbitrary member names — used to
// craft hostile archives.
func writeRawArchive(t *testing.T, path string, names map[string]string) {
	t.Helper()
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	for name, content := range names {
		hdr := &tar.Header{Name: name, Mode: 0o644, Size: int64(len(content)), Typeflag: tar.TypeReg}
		if err := tw.WriteHeader(hdr); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write([]byte(content)); err != nil {
			t.Fatal(err)
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, buf.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestPathTraversalRefused(t *testing.T) {
	dir := t.TempDir()
	dest := filepath.Join(dir, "dest")
	if err := os.Mkdir(dest, 0o755); err != nil {
		t.Fatal(err)
	}
	writeRawArchive(t, filepath.Join(dir, "evil.tar"), map[string]string{
		"../evil.txt":      "pwned",
		"ok/../../zap.txt": "pwned",
		"safe.txt":         "fine",
	})
	_, errb, code := runTool(t, dir, nil, "-xf", "evil.tar", "-C", "dest")
	if code != 1 {
		t.Fatalf("traversal extract: code=%d err=%q", code, errb)
	}
	if !strings.Contains(errb, "refusing to extract") {
		t.Errorf("missing refusal diagnostic: %q", errb)
	}
	if _, err := os.Stat(filepath.Join(dir, "evil.txt")); !os.IsNotExist(err) {
		t.Fatalf("evil.txt escaped the target directory")
	}
	if _, err := os.Stat(filepath.Join(dir, "zap.txt")); !os.IsNotExist(err) {
		t.Fatalf("zap.txt escaped the target directory")
	}
	if got, err := os.ReadFile(filepath.Join(dest, "safe.txt")); err != nil || string(got) != "fine" {
		t.Fatalf("safe member should still extract: %q, %v", got, err)
	}
}

func TestAbsoluteMemberRefusedOnExtract(t *testing.T) {
	dir := t.TempDir()
	dest := filepath.Join(dir, "dest")
	if err := os.Mkdir(dest, 0o755); err != nil {
		t.Fatal(err)
	}
	writeRawArchive(t, filepath.Join(dir, "abs.tar"), map[string]string{
		"/abs.txt": "pwned",
	})
	_, errb, code := runTool(t, dir, nil, "-xf", "abs.tar", "-C", "dest")
	if code != 1 || !strings.Contains(errb, "absolute") {
		t.Errorf("absolute member: code=%d err=%q", code, errb)
	}
}

func TestLeadingSlashStrippedOnCreate(t *testing.T) {
	dir := t.TempDir()
	abs := filepath.Join(dir, "f.txt")
	if err := os.WriteFile(abs, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS == "windows" {
		t.Skip("absolute-operand member naming differs with drive letters")
	}
	_, errb, code := runTool(t, dir, nil, "-cf", "a.tar", abs)
	if code != 0 || !strings.Contains(errb, "Removing leading '/'") {
		t.Fatalf("create abs: code=%d err=%q", code, errb)
	}
	out, _, _ := runTool(t, dir, nil, "-tf", "a.tar")
	if strings.HasPrefix(out, "/") {
		t.Errorf("member name kept leading slash: %q", out)
	}
}

func TestUsageErrors(t *testing.T) {
	dir := t.TempDir()
	cases := []struct {
		args []string
		want string
	}{
		{[]string{"-f", "a.tar"}, "You must specify one of"},
		{[]string{"-ctf", "a.tar"}, "may not specify more than one"},
		{[]string{"-c", "x"}, "no archive file specified"},
		{[]string{"-cf", "a.tar"}, "Cowardly refusing to create an empty archive"},
	}
	for _, c := range cases {
		_, errb, code := runTool(t, dir, nil, c.args...)
		if code != 2 || !strings.Contains(errb, c.want) {
			t.Errorf("tar %v: code=%d err=%q (want %q)", c.args, code, errb, c.want)
		}
	}
	// unknown flag: contract error
	_, errb, code := runTool(t, dir, nil, "--frobnicate")
	if code != 2 || !strings.Contains(errb, "frobnicate") || !strings.Contains(errb, "pure-Go") {
		t.Errorf("unknown flag: code=%d err=%q", code, errb)
	}
	// missing archive file
	_, errb, code = runTool(t, dir, nil, "-tf", "nope.tar")
	if code != 1 || !strings.Contains(errb, "Cannot open") {
		t.Errorf("missing archive: code=%d err=%q", code, errb)
	}
}

func TestHelpAndVersion(t *testing.T) {
	out, _, code := runTool(t, t.TempDir(), nil, "--help")
	if code != 0 || !strings.Contains(out, "Usage: tar") {
		t.Errorf("--help: code=%d out=%q", code, out)
	}
	out, _, code = runTool(t, t.TempDir(), nil, "--version")
	if code != 0 || !strings.Contains(out, "tar") {
		t.Errorf("--version: code=%d out=%q", code, out)
	}
}

func TestExtractVerbosePrintsNames(t *testing.T) {
	dir := t.TempDir()
	makeTree(t, dir)
	if _, _, code := runTool(t, dir, nil, "-cf", "a.tar", "src"); code != 0 {
		t.Fatal("create failed")
	}
	dest := filepath.Join(dir, "dest")
	if err := os.Mkdir(dest, 0o755); err != nil {
		t.Fatal(err)
	}
	out, _, code := runTool(t, dir, nil, "-xvf", "a.tar", "-C", "dest")
	if code != 0 || !strings.Contains(out, "src/a.txt\n") {
		t.Errorf("-xv: code=%d out=%q", code, out)
	}
}

func TestNotGzipWithZ(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "junk.tgz"), []byte("not gzip at all"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, errb, code := runTool(t, dir, nil, "-tzf", "junk.tgz")
	if code != 1 || !strings.Contains(errb, "not in gzip format") {
		t.Errorf("bad gzip: code=%d err=%q", code, errb)
	}
}

// sanity: the gzip stream a -z archive produces really is gzip
func TestCreateZProducesGzip(t *testing.T) {
	dir := t.TempDir()
	makeTree(t, dir)
	if _, _, code := runTool(t, dir, nil, "-czf", "a.tgz", "src"); code != 0 {
		t.Fatal("create -z failed")
	}
	f, err := os.Open(filepath.Join(dir, "a.tgz"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	zr, err := gzip.NewReader(f)
	if err != nil {
		t.Fatalf("not gzip: %v", err)
	}
	tr := tar.NewReader(zr)
	if _, err := tr.Next(); err != nil {
		t.Fatalf("not a tar inside gzip: %v", err)
	}
}
