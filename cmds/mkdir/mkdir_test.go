package mkdircmd

import (
	"bytes"
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/qiangli/coreutils/tool"
)

type mkdirModeFileInfo struct {
	os.FileInfo
	mode os.FileMode
}

func (fi mkdirModeFileInfo) Mode() os.FileMode { return fi.mode }

// runTool is the canonical test harness shape for cmds packages:
// output is captured after Run returns.
func runTool(t *testing.T, dir string, args ...string) (stdout, stderr string, code int) {
	t.Helper()
	return runToolWithContext(t, &tool.RunContext{
		Ctx:   context.Background(),
		Dir:   dir,
		Stdio: tool.Stdio{In: strings.NewReader(""), Out: &bytes.Buffer{}, Err: &bytes.Buffer{}},
	}, args...)
}

func runToolWithContext(t *testing.T, rc *tool.RunContext, args ...string) (stdout, stderr string, code int) {
	t.Helper()
	var out, errb bytes.Buffer
	if rc.Ctx == nil {
		rc.Ctx = context.Background()
	}
	rc.Stdio = tool.Stdio{In: strings.NewReader(""), Out: &out, Err: &errb}
	code = cmd.Run(rc, args)
	return out.String(), errb.String(), code
}

func TestMkdirSimple(t *testing.T) {
	dir := t.TempDir()
	out, errb, code := runTool(t, dir, "d")
	if code != 0 || out != "" || errb != "" {
		t.Fatalf("mkdir d: code=%d out=%q err=%q", code, out, errb)
	}
	fi, err := os.Stat(filepath.Join(dir, "d"))
	if err != nil || !fi.IsDir() {
		t.Errorf("directory not created: %v", err)
	}
}

func TestMkdirVerbose(t *testing.T) {
	dir := t.TempDir()
	out, _, code := runTool(t, dir, "-v", "d")
	if code != 0 || out != "mkdir: created directory 'd'\n" {
		t.Errorf("mkdir -v: code=%d out=%q", code, out)
	}
}

func TestMkdirExisting(t *testing.T) {
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "d"), 0o755); err != nil {
		t.Fatal(err)
	}
	_, errb, code := runTool(t, dir, "d")
	if code != 1 || !strings.Contains(errb, "cannot create directory 'd'") ||
		!strings.Contains(strings.ToLower(errb), "exists") {
		t.Errorf("code=%d err=%q", code, errb)
	}
	// -p: existing directory is not an error
	out, errb, code := runTool(t, dir, "-p", "d")
	if code != 0 || out != "" || errb != "" {
		t.Errorf("mkdir -p existing: code=%d out=%q err=%q", code, out, errb)
	}
}

func TestMkdirParents(t *testing.T) {
	dir := t.TempDir()
	nested := filepath.Join("a", "b", "c")
	out, errb, code := runTool(t, dir, "-pv", nested)
	if code != 0 {
		t.Fatalf("mkdir -pv: code=%d err=%q", code, errb)
	}
	want := "mkdir: created directory 'a'\n" +
		"mkdir: created directory '" + filepath.Join("a", "b") + "'\n" +
		"mkdir: created directory '" + nested + "'\n"
	if out != want {
		t.Errorf("out=%q want %q", out, want)
	}
	fi, err := os.Stat(filepath.Join(dir, "a", "b", "c"))
	if err != nil || !fi.IsDir() {
		t.Error("nested directory not created")
	}
}

// TestMkdirParentsTrailingSlash pins the cross-platform half of the
// POSIX pathname rule: "-p dir/" creates dir, and "-p existing/" is
// ignored without error.
func TestMkdirParentsTrailingSlash(t *testing.T) {
	dir := t.TempDir()
	out, errb, code := runTool(t, dir, "-p", "a/b/")
	if code != 0 || out != "" || errb != "" {
		t.Fatalf("mkdir -p a/b/: code=%d out=%q err=%q", code, out, errb)
	}
	fi, err := os.Stat(filepath.Join(dir, "a", "b"))
	if err != nil || !fi.IsDir() {
		t.Fatalf("a/b not created: %v", err)
	}
	out, errb, code = runTool(t, dir, "-p", "a/b/")
	if code != 0 || out != "" || errb != "" {
		t.Errorf("mkdir -p existing with trailing slash: code=%d out=%q err=%q", code, out, errb)
	}
}

// TestMkdirContinuesAfterOperandError pins the Issue 7 multi-operand
// contract: a failing dir operand is diagnosed, the remaining operands
// are still processed, and the final status is >0.
func TestMkdirContinuesAfterOperandError(t *testing.T) {
	dir := t.TempDir()
	_, errb, code := runTool(t, dir, filepath.Join("missing", "child"), "ok")
	if code != 1 {
		t.Fatalf("code=%d, want 1", code)
	}
	if !strings.Contains(errb, "cannot create directory") {
		t.Errorf("missing diagnostic: %q", errb)
	}
	if fi, err := os.Stat(filepath.Join(dir, "ok")); err != nil || !fi.IsDir() {
		t.Errorf("later operand 'ok' not created after earlier failure: %v", err)
	}
}

func TestMkdirMissingParentWithoutP(t *testing.T) {
	dir := t.TempDir()
	_, errb, code := runTool(t, dir, filepath.Join("a", "b"))
	if code != 1 || !strings.Contains(errb, "cannot create directory") {
		t.Errorf("code=%d err=%q", code, errb)
	}
}

func TestMkdirMode(t *testing.T) {
	dir := t.TempDir()
	if runtime.GOOS == "windows" {
		_, errb, code := runTool(t, dir, "-m", "700", "d")
		if code != 2 || !strings.Contains(errb, "not supported") {
			t.Errorf("windows -m: code=%d err=%q", code, errb)
		}
		return
	}
	_, errb, code := runTool(t, dir, "-m", "700", "d")
	if code != 0 {
		t.Fatalf("mkdir -m 700: code=%d err=%q", code, errb)
	}
	fi, err := os.Stat(filepath.Join(dir, "d"))
	if err != nil {
		t.Fatal(err)
	}
	if fi.Mode().Perm() != 0o700 {
		t.Errorf("mode = %o, want 700", fi.Mode().Perm())
	}
	// -m applies to the final directory only (-p intermediates get
	// defaults). 0o505 has no owner-write bit; a default-created
	// intermediate always keeps owner-write (umask does not mask the
	// owner bits in any sane configuration).
	_, errb, code = runTool(t, dir, "-p", "-m", "505", filepath.Join("x", "y"))
	if code != 0 {
		t.Fatalf("mkdir -p -m: code=%d err=%q", code, errb)
	}
	yfi, err := os.Stat(filepath.Join(dir, "x", "y"))
	if err != nil {
		t.Fatal(err)
	}
	if yfi.Mode().Perm() != 0o505 {
		t.Errorf("final mode = %o, want 505", yfi.Mode().Perm())
	}
	xfi, err := os.Stat(filepath.Join(dir, "x"))
	if err != nil {
		t.Fatal(err)
	}
	if xfi.Mode().Perm()&0o200 == 0 {
		t.Errorf("intermediate mode = %o; -m must apply to the final dir only", xfi.Mode().Perm())
	}
}

func TestMkdirModeErrors(t *testing.T) {
	dir := t.TempDir()
	if runtime.GOOS == "windows" {
		t.Skipf("-m is refused before value validation on windows")
	}
	_, errb, code := runTool(t, dir, "-m", "999", "d")
	if code != 2 || !strings.Contains(errb, "invalid mode '999'") {
		t.Errorf("-m 999: code=%d err=%q", code, errb)
	}
	_, errb, code = runTool(t, dir, "-m", "u+q", "d")
	if code != 2 || !strings.Contains(errb, "invalid mode 'u+q'") {
		t.Errorf("-m u+q: code=%d err=%q", code, errb)
	}
	_, errb, code = runTool(t, dir, "-m", "10000", "d")
	if code != 2 || !strings.Contains(errb, "invalid mode '10000'") {
		t.Errorf("-m 10000: code=%d err=%q", code, errb)
	}
}

func TestMkdirLeadingPlusNumericModeExtension(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("-m is loudly unsupported on Windows")
	}
	dir := t.TempDir()
	_, errb, code := runTool(t, dir, "-m", "+777", "d")
	if code != 0 {
		t.Fatalf("mkdir -m +777: code=%d err=%q", code, errb)
	}
	fi, err := os.Stat(filepath.Join(dir, "d"))
	if err != nil {
		t.Fatal(err)
	}
	if got := fi.Mode().Perm(); got != 0o777 {
		t.Fatalf("mkdir -m +777 mode=%o, want 777", got)
	}
}

func TestMkdirSymbolicMode(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX mode bits")
	}
	dir := t.TempDir()
	_, errb, code := runTool(t, dir, "-m", "u=rwx,go=rx", "d")
	if code != 0 {
		t.Fatalf("symbolic mode: code=%d err=%q", code, errb)
	}
	fi, err := os.Stat(filepath.Join(dir, "d"))
	if err != nil {
		t.Fatal(err)
	}
	if fi.Mode().Perm() != 0o755 {
		t.Errorf("mode=%o, want 755", fi.Mode().Perm())
	}
}

func TestMkdirSymbolicModeStartsAtDefault(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX mode bits")
	}
	dir := t.TempDir()
	for _, args := range [][]string{
		{"-m", "+x", "plain"},
		{"-p", "-m", "+x", filepath.Join("a", "b")},
	} {
		_, errb, code := runTool(t, dir, args...)
		if code != 0 {
			t.Fatalf("mkdir %v: code=%d err=%q", args, code, errb)
		}
	}
	for _, name := range []string{"plain", filepath.Join("a", "b")} {
		fi, err := os.Stat(filepath.Join(dir, name))
		if err != nil {
			t.Fatal(err)
		}
		// +x must add execute bits to the 0777 creation default. The
		// process umask is accounted for by mkdirMode.apply, rather than
		// being applied a second time to an already-created directory.
		if got := fi.Mode().Perm(); got != 0o777 {
			t.Errorf("%s mode=%o, want 777", name, got)
		}
	}
}

func TestMkdirSymbolicModeSubtractsFromDefault(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX mode bits")
	}
	dir := t.TempDir()
	_, errb, code := runTool(t, dir, "-m", "a-x", "d")
	if code != 0 {
		t.Fatalf("mkdir -m a-x: code=%d err=%q", code, errb)
	}
	fi, err := os.Stat(filepath.Join(dir, "d"))
	if err != nil {
		t.Fatal(err)
	}
	if got := fi.Mode().Perm(); got != 0o666 {
		t.Fatalf("mode=%o, want 666", got)
	}
}

func TestMkdirSymbolicModeApply(t *testing.T) {
	cases := []struct {
		mode string
		old  uint32
		um   uint32
		want uint32
	}{
		{"u=rw,go=r", 0o777, 0, 0o644},
		{"g=u", 0o741, 0, 0o771},
		{"o=g", 0o754, 0, 0o755},
		{"u+s,g+s,+t", 0o755, 0, 0o7755},
		{"a+X", 0o644, 0, 0o755}, // mkdir always operates on a directory.
		{"+w", 0o444, 0o22, 0o644},
		{"=rwx", 0o644, 0o22, 0o755},
		{"u+rw-x", 0o111, 0, 0o611},
	}
	for _, tc := range cases {
		m, ok := parseSymbolicMode(tc.mode)
		if !ok {
			t.Fatalf("parseSymbolicMode(%q) rejected valid mode", tc.mode)
		}
		if got := m.apply(tc.old, tc.um); got != tc.want {
			t.Errorf("%q on %04o with umask %03o = %04o, want %04o", tc.mode, tc.old, tc.um, got, tc.want)
		}
	}
}

func TestMkdirSymbolicModeInvalid(t *testing.T) {
	for _, mode := range []string{"", "u", "u+q", "u~x", "rwx", "u=gw", ","} {
		if _, ok := parseSymbolicMode(mode); ok {
			t.Errorf("parseSymbolicMode(%q): accepted invalid mode", mode)
		}
	}
}

func TestMkdirUsageErrors(t *testing.T) {
	dir := t.TempDir()
	_, errb, code := runTool(t, dir)
	if code != 2 || !strings.Contains(errb, "missing operand") {
		t.Errorf("no args: code=%d err=%q", code, errb)
	}
	_, errb, code = runTool(t, dir, "--frobnicate", "d")
	if code != 2 || !strings.Contains(errb, "frobnicate") || !strings.Contains(errb, "pure-Go") {
		t.Errorf("unknown flag: code=%d err=%q", code, errb)
	}
}

func TestMkdirDashOperandIsPathname(t *testing.T) {
	dir := t.TempDir()
	_, errb, code := runTool(t, dir, "--", "-")
	if code != 0 || errb != "" {
		t.Fatalf("mkdir -- -: code=%d err=%q", code, errb)
	}
	if fi, err := os.Stat(filepath.Join(dir, "-")); err != nil || !fi.IsDir() {
		t.Fatalf("dash pathname not created: %v", err)
	}
}

func TestMkdirEmptyOperandFailsAndContinues(t *testing.T) {
	dir := t.TempDir()
	_, errb, code := runTool(t, dir, "", "ok")
	if code != 1 || !strings.Contains(errb, "cannot create directory ''") {
		t.Fatalf("empty operand: code=%d err=%q", code, errb)
	}
	if fi, err := os.Stat(filepath.Join(dir, "ok")); err != nil || !fi.IsDir() {
		t.Fatalf("later operand not created after empty pathname: %v", err)
	}
}

func TestMkdirVirtualUmaskDefaultAndParents(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX mode bits")
	}
	dir := t.TempDir()
	rc := &tool.RunContext{Ctx: context.Background(), Dir: dir, Umask: 0o027, UmaskSet: true}
	_, errb, code := runToolWithContext(t, rc, "plain")
	if code != 0 {
		t.Fatalf("mkdir with virtual umask: code=%d err=%q", code, errb)
	}
	fi, err := os.Stat(filepath.Join(dir, "plain"))
	if err != nil {
		t.Fatal(err)
	}
	if got := fi.Mode().Perm(); got != 0o750 {
		t.Fatalf("plain mode=%o, want 750", got)
	}

	_, errb, code = runToolWithContext(t, rc, "-p", filepath.Join("a", "b"))
	if code != 0 {
		t.Fatalf("mkdir -p with virtual umask: code=%d err=%q", code, errb)
	}
	afi, err := os.Stat(filepath.Join(dir, "a"))
	if err != nil {
		t.Fatal(err)
	}
	if got := afi.Mode().Perm(); got != 0o750 {
		t.Fatalf("intermediate mode=%o, want 750", got)
	}
	bfi, err := os.Stat(filepath.Join(dir, "a", "b"))
	if err != nil {
		t.Fatal(err)
	}
	if got := bfi.Mode().Perm(); got != 0o750 {
		t.Fatalf("final mode=%o, want 750", got)
	}
}

func TestMkdirVirtualUmaskSymbolicImplicitWho(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX mode bits")
	}
	dir := t.TempDir()
	rc := &tool.RunContext{Ctx: context.Background(), Dir: dir, Umask: 0o077, UmaskSet: true}
	_, errb, code := runToolWithContext(t, rc, "-m", "=rwx", "d")
	if code != 0 {
		t.Fatalf("mkdir -m =rwx: code=%d err=%q", code, errb)
	}
	fi, err := os.Stat(filepath.Join(dir, "d"))
	if err != nil {
		t.Fatal(err)
	}
	if got := fi.Mode().Perm(); got != 0o700 {
		t.Fatalf("mode=%o, want 700", got)
	}
}

func TestMkdirVirtualUmaskRestrictsInitialMkdirModes(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX mode bits")
	}
	dir := t.TempDir()
	var got = make(map[string]os.FileMode)
	m := &maker{
		rc: &tool.RunContext{
			Ctx: context.Background(), Dir: dir, Umask: 0o077, UmaskSet: true,
			Stdio: tool.Stdio{Err: &bytes.Buffer{}},
		},
		parents: true,
		deps:    defaultMkdirDeps,
	}
	m.deps.mkdir = func(path string, mode os.FileMode) error {
		got[filepath.Base(path)] = mode
		return os.Mkdir(path, mode)
	}
	m.make(filepath.Join("a", "b"))
	if m.failed {
		t.Fatal("mkdir -p failed")
	}
	for _, name := range []string{"a", "b"} {
		if mode, ok := got[name]; !ok || mode.Perm() != 0o700 {
			t.Errorf("initial mkdir mode for %s = %o (present=%v), want 700", name, mode.Perm(), ok)
		}
	}
}

func TestMkdirVirtualUmaskCorrectionPreservesInheritedSpecialBits(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX mode bits")
	}
	dir := t.TempDir()
	var corrected os.FileMode
	m := &maker{
		rc: &tool.RunContext{
			Ctx: context.Background(), Dir: dir, Umask: 0o022, UmaskSet: true,
			Stdio: tool.Stdio{Err: &bytes.Buffer{}},
		},
		deps: defaultMkdirDeps,
	}
	m.deps.mkdir = func(path string, mode os.FileMode) error {
		// Model a host umask stricter than the virtual one.
		return os.Mkdir(path, 0o700)
	}
	m.deps.stat = func(path string) (os.FileInfo, error) {
		fi, err := os.Stat(path)
		if err != nil {
			return nil, err
		}
		return mkdirModeFileInfo{FileInfo: fi, mode: os.ModeDir | os.ModeSetgid | os.ModeSticky | 0o700}, nil
	}
	m.deps.chmod = func(_ string, mode os.FileMode) error {
		corrected = mode
		return nil
	}
	m.make("d")
	if m.failed {
		t.Fatal("mkdir failed")
	}
	want := os.FileMode(0o755) | os.ModeSetgid | os.ModeSticky
	const controlled = os.ModePerm | os.ModeSetgid | os.ModeSticky
	if corrected&controlled != want {
		t.Fatalf("corrective mode=%v, want permissions 0755 with inherited setgid and sticky", corrected)
	}
}

func TestMkdirInjectedFilesystemErrors(t *testing.T) {
	dir := t.TempDir()
	var errb bytes.Buffer
	m := &maker{
		rc: &tool.RunContext{
			Ctx:   context.Background(),
			Dir:   dir,
			Stdio: tool.Stdio{Err: &errb},
		},
		deps: defaultMkdirDeps,
	}
	m.deps.mkdir = func(path string, mode os.FileMode) error {
		if filepath.Base(path) == "bad" {
			return &os.PathError{Op: "mkdir", Path: path, Err: fs.ErrPermission}
		}
		return os.Mkdir(path, mode)
	}
	m.make("bad")
	m.make("ok")
	if !m.failed || !strings.Contains(errb.String(), "cannot create directory 'bad'") {
		t.Fatalf("missing injected mkdir failure: failed=%v err=%q", m.failed, errb.String())
	}
	if fi, err := os.Stat(filepath.Join(dir, "ok")); err != nil || !fi.IsDir() {
		t.Fatalf("later operand not created after injected failure: %v", err)
	}
}

func TestMkdirInjectedChmodErrorLeavesCreatedDirectoryAndFails(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX mode bits")
	}
	dir := t.TempDir()
	var errb bytes.Buffer
	m := &maker{
		rc: &tool.RunContext{
			Ctx:   context.Background(),
			Dir:   dir,
			Stdio: tool.Stdio{Err: &errb},
		},
		useMode: true,
		mode:    0o700,
		deps:    defaultMkdirDeps,
	}
	m.deps.chmod = func(path string, mode os.FileMode) error {
		return &os.PathError{Op: "chmod", Path: path, Err: fs.ErrPermission}
	}
	// Force the post-create correction path regardless of the host umask.
	m.deps.mkdir = func(path string, mode os.FileMode) error {
		return os.Mkdir(path, 0)
	}
	m.make("d")
	if !m.failed || !strings.Contains(errb.String(), "cannot set permissions of 'd'") {
		t.Fatalf("missing injected chmod failure: failed=%v err=%q", m.failed, errb.String())
	}
	if fi, err := os.Stat(filepath.Join(dir, "d")); err != nil || !fi.IsDir() {
		t.Fatalf("directory should remain after chmod failure: %v", err)
	}
}

func TestMkdirContextMayBeUsedWithoutArgument(t *testing.T) {
	dir := t.TempDir()
	_, errb, code := runTool(t, dir, "-Z", "d")
	if code != 0 || errb != "" {
		t.Fatalf("mkdir -Z: code=%d err=%q", code, errb)
	}
	if fi, err := os.Stat(filepath.Join(dir, "d")); err != nil || !fi.IsDir() {
		t.Fatalf("mkdir -Z did not create directory: %v", err)
	}
}

func TestMkdirHelpAndVersion(t *testing.T) {
	dir := t.TempDir()
	out, _, code := runTool(t, dir, "--help")
	if code != 0 || !strings.Contains(out, "Usage: mkdir") {
		t.Errorf("--help: code=%d out=%q", code, out)
	}
	out, _, code = runTool(t, dir, "--version")
	if code != 0 || !strings.Contains(out, "mkdir") {
		t.Errorf("--version: code=%d out=%q", code, out)
	}
}
