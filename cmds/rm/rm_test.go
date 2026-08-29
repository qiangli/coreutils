package rmcmd

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/qiangli/coreutils/cmds/internal/rootguard"
	"github.com/qiangli/coreutils/tool"
)

// runTool is the canonical test harness shape for cmds packages:
// output is captured after Run returns.
func runTool(t *testing.T, dir string, args ...string) (stdout, stderr string, code int) {
	t.Helper()
	return runToolIn(t, dir, "", args...)
}

func runToolIn(t *testing.T, dir, stdin string, args ...string) (stdout, stderr string, code int) {
	t.Helper()
	var out, errb bytes.Buffer
	rc := &tool.RunContext{
		Ctx:   context.Background(),
		Dir:   dir,
		Stdio: tool.Stdio{In: strings.NewReader(stdin), Out: &out, Err: &errb},
	}
	code = cmd.Run(rc, args)
	return out.String(), errb.String(), code
}

func write(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestRmFile(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, "a"), "x")
	out, errb, code := runTool(t, dir, "a")
	if code != 0 || out != "" || errb != "" {
		t.Fatalf("rm a: code=%d out=%q err=%q", code, out, errb)
	}
	if _, err := os.Lstat(filepath.Join(dir, "a")); !os.IsNotExist(err) {
		t.Error("file still exists")
	}
}

func TestRmStopsOptionRecognitionAtFirstOperand(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"ordinary", "-r", "--", "-i"} {
		write(t, filepath.Join(dir, name), "x")
	}

	// Arguments beginning with '-' after the first operand are pathnames,
	// including "--"; -i must not turn prompting back on in this position.
	_, errb, code := runToolIn(t, dir, "", "ordinary", "-r", "--", "-i")
	if code != 0 || errb != "" {
		t.Fatalf("rm operand -r -- -i: code=%d err=%q", code, errb)
	}
	for _, name := range []string{"ordinary", "-r", "--", "-i"} {
		if _, err := os.Lstat(filepath.Join(dir, name)); !os.IsNotExist(err) {
			t.Errorf("operand %q was not removed: %v", name, err)
		}
	}
}

func TestRmVerbose(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, "a"), "x")
	out, _, code := runTool(t, dir, "-v", "a")
	if code != 0 || out != "removed 'a'\n" {
		t.Errorf("rm -v: code=%d out=%q", code, out)
	}
}

func TestRmMissing(t *testing.T) {
	dir := t.TempDir()
	_, errb, code := runTool(t, dir, "nope")
	if code != 1 || !strings.Contains(errb, "cannot remove 'nope'") {
		t.Errorf("code=%d err=%q", code, errb)
	}
	// -f silences nonexistent operands
	out, errb, code := runTool(t, dir, "-f", "nope")
	if code != 0 || out != "" || errb != "" {
		t.Errorf("rm -f nope: code=%d out=%q err=%q", code, out, errb)
	}
}

func TestRmDirWithoutR(t *testing.T) {
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "d"), 0o755); err != nil {
		t.Fatal(err)
	}
	_, errb, code := runTool(t, dir, "d")
	if code != 1 || !strings.Contains(errb, "cannot remove 'd': Is a directory") {
		t.Errorf("code=%d err=%q", code, errb)
	}
	if _, err := os.Stat(filepath.Join(dir, "d")); err != nil {
		t.Error("directory was removed without -r")
	}
}

func TestRmDirFlagRemovesEmptyDirectory(t *testing.T) {
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "d"), 0o755); err != nil {
		t.Fatal(err)
	}
	_, errb, code := runTool(t, dir, "-d", "d")
	if code != 0 {
		t.Fatalf("rm -d: code=%d err=%q", code, errb)
	}
	if _, err := os.Lstat(filepath.Join(dir, "d")); !os.IsNotExist(err) {
		t.Error("empty directory still exists")
	}
}

func TestRmRecursive(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, "d", "f"), "x")
	write(t, filepath.Join(dir, "d", "sub", "g"), "y")
	_, errb, code := runTool(t, dir, "-r", "d")
	if code != 0 {
		t.Fatalf("rm -r: code=%d err=%q", code, errb)
	}
	if _, err := os.Lstat(filepath.Join(dir, "d")); !os.IsNotExist(err) {
		t.Error("tree still exists")
	}
}

func TestRmRecursiveVerbosePostOrder(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, "d", "f"), "x")
	out, _, code := runTool(t, dir, "-rv", "d")
	want := "removed '" + filepath.Join("d", "f") + "'\nremoved directory 'd'\n"
	if code != 0 || out != want {
		t.Errorf("rm -rv: code=%d out=%q want %q", code, out, want)
	}
}

func TestRmCapitalRAlias(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, "d", "f"), "x")
	_, errb, code := runTool(t, dir, "-R", "d")
	if code != 0 {
		t.Fatalf("rm -R: code=%d err=%q", code, errb)
	}
	if _, err := os.Lstat(filepath.Join(dir, "d")); !os.IsNotExist(err) {
		t.Error("-R did not remove recursively")
	}
}

func TestRmInteractivePrompt(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, "a"), "x")
	_, errb, code := runTool(t, dir, "-i", "a")
	if code != 0 || !strings.Contains(errb, "remove 'a'?") {
		t.Fatalf("rm -i no input: code=%d err=%q", code, errb)
	}
	if _, err := os.Stat(filepath.Join(dir, "a")); err != nil {
		t.Fatal("rm -i without yes removed the file")
	}
	_, errb, code = runToolIn(t, dir, "y\n", "-i", "a")
	if code != 0 || !strings.Contains(errb, "remove 'a'?") {
		t.Fatalf("rm -i yes: code=%d err=%q", code, errb)
	}
	if _, err := os.Lstat(filepath.Join(dir, "a")); !os.IsNotExist(err) {
		t.Fatal("rm -i yes did not remove the file")
	}
}

func TestRmCompatibilityNoOps(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, "a"), "x")
	_, errb, code := runTool(t, dir, "-go", "a")
	if code != 0 || errb != "" {
		t.Fatalf("compat flags: code=%d err=%q", code, errb)
	}
	if _, err := os.Lstat(filepath.Join(dir, "a")); !os.IsNotExist(err) {
		t.Fatal("file still exists")
	}

	out, _, code := runTool(t, dir, "--help")
	if code != 0 || !strings.Contains(out, "-g, --progress") || !strings.Contains(out, "-o, --one-file-system") {
		t.Fatalf("help missing compatibility aliases: code=%d out=%q", code, out)
	}
}

func TestRmRootRefused(t *testing.T) {
	dir := t.TempDir()
	guarded := filepath.Join(dir, "guarded", "child", "target")
	deep := filepath.Join(dir, "guarded", "child", "deep")
	write(t, filepath.Join(guarded, "sentinel"), "keep")
	if err := os.MkdirAll(deep, 0o755); err != nil {
		t.Fatal(err)
	}

	old := isFilesystemRoot
	isFilesystemRoot = func(path string, followFinal bool) bool {
		return rootguard.SameFile(path, guarded, followFinal)
	}
	t.Cleanup(func() { isFilesystemRoot = old })

	alias := filepath.Join("guarded", "child", "deep", "..", "target")
	_, errb, code := runTool(t, dir, "-rf", alias)
	if code != 1 || !strings.Contains(errb, "it is dangerous to operate recursively on") {
		t.Fatalf("rm identity guard: code=%d err=%q", code, errb)
	}
	if want := "(same as '" + rootguard.RootPath(guarded) + "')"; !strings.Contains(errb, want) {
		t.Fatalf("identity guard diagnostic=%q, want %q", errb, want)
	}
	if _, err := os.Stat(filepath.Join(guarded, "sentinel")); err != nil {
		t.Fatalf("guarded tree was modified: %v", err)
	}
}

func TestRmPreserveRootFinalSymlinkPolicy(t *testing.T) {
	dir := t.TempDir()
	guarded := filepath.Join(dir, "guarded")
	write(t, filepath.Join(guarded, "sentinel"), "keep")

	old := isFilesystemRoot
	isFilesystemRoot = func(path string, followFinal bool) bool {
		return rootguard.SameFile(path, guarded, followFinal)
	}
	t.Cleanup(func() { isFilesystemRoot = old })

	link := filepath.Join(dir, "link")
	if err := os.Symlink(guarded, link); err != nil {
		t.Skipf("symlinks are unavailable: %v", err)
	}
	_, errb, code := runTool(t, dir, "-rf", "link")
	if code != 0 || errb != "" {
		t.Fatalf("unfollowed symlink: code=%d err=%q", code, errb)
	}
	if _, err := os.Stat(filepath.Join(guarded, "sentinel")); err != nil {
		t.Fatalf("symlink referent was modified: %v", err)
	}

	if err := os.Symlink(guarded, link); err != nil {
		t.Fatal(err)
	}
	_, errb, code = runTool(t, dir, "-rf", "link"+string(filepath.Separator))
	if code != 1 || !strings.Contains(errb, "dangerous to operate recursively") {
		t.Fatalf("trailing-separator symlink: code=%d err=%q", code, errb)
	}
	if _, err := os.Lstat(link); err != nil {
		t.Fatalf("guarded symlink was modified: %v", err)
	}
}

func TestRmOperandErrors(t *testing.T) {
	dir := t.TempDir()
	_, errb, code := runTool(t, dir)
	if code != 2 || !strings.Contains(errb, "missing operand") {
		t.Errorf("no args: code=%d err=%q", code, errb)
	}
	// GNU: rm -f with no operands exits 0
	out, errb, code := runTool(t, dir, "-f")
	if code != 0 || out != "" || errb != "" {
		t.Errorf("rm -f: code=%d out=%q err=%q", code, out, errb)
	}
	_, errb, code = runTool(t, dir, "--frobnicate", "x")
	if code != 2 || !strings.Contains(errb, "frobnicate") || !strings.Contains(errb, "pure-Go") {
		t.Errorf("unknown flag: code=%d err=%q", code, errb)
	}
}

func TestRmContinuesPastErrors(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, "b"), "x")
	_, errb, code := runTool(t, dir, "nope", "b")
	if code != 1 || !strings.Contains(errb, "cannot remove 'nope'") {
		t.Errorf("code=%d err=%q", code, errb)
	}
	if _, err := os.Lstat(filepath.Join(dir, "b")); !os.IsNotExist(err) {
		t.Error("later operand not removed after earlier failure")
	}
}

func TestRmHelpAndVersion(t *testing.T) {
	dir := t.TempDir()
	out, _, code := runTool(t, dir, "--help")
	if code != 0 || !strings.Contains(out, "Usage: rm") {
		t.Errorf("--help: code=%d out=%q", code, out)
	}
	out, _, code = runTool(t, dir, "--version")
	if code != 0 || !strings.Contains(out, "rm") {
		t.Errorf("--version: code=%d out=%q", code, out)
	}
}

func TestRmRecursiveInteractivePrompts(t *testing.T) {
	dir := t.TempDir()
	d := filepath.Join(dir, "d")
	os.MkdirAll(d, 0755)
	write(t, filepath.Join(d, "f"), "x")

	// Answer 'y' to all prompts
	input := "y\ny\ny\n"
	_, errb, code := runToolIn(t, dir, input, "-ri", "d")
	if code != 0 {
		t.Errorf("expected 0 exit code, got %d", code)
	}

	// Should prompt for:
	// 1. descend into directory (or just "remove 'd'?" based on our generic prompt)
	// 2. remove file 'd/f'?
	// 3. remove directory 'd'?
	if strings.Count(errb, "remove '") != 3 {
		t.Errorf("expected 3 prompts, got %d in %q", strings.Count(errb, "remove '"), errb)
	}
	if !strings.Contains(errb, filepath.Join("d", "f")) {
		t.Errorf("did not prompt for file inside directory: %q", errb)
	}

	if _, err := os.Lstat(d); !os.IsNotExist(err) {
		t.Error("directory not removed after 'y' answers")
	}
}

func TestRmRecursiveInteractiveDeclinedBranch(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, "d", "a"), "x")
	write(t, filepath.Join(dir, "d", "sub", "nested"), "x")

	_, errb, code := runToolIn(t, dir, "y\ny\nn\ny\n", "-ri", "d")
	if code != 1 {
		t.Fatalf("rm -ri with declined branch: code=%d err=%q", code, errb)
	}
	for _, prompt := range []string{
		"remove 'd'?",
		"remove '" + filepath.Join("d", "a") + "'?",
		"remove '" + filepath.Join("d", "sub") + "'?",
	} {
		if !strings.Contains(errb, prompt) {
			t.Fatalf("missing prompt %q in %q", prompt, errb)
		}
	}
	if strings.Contains(errb, filepath.Join("d", "sub", "nested")) {
		t.Fatalf("descended into declined branch: %q", errb)
	}
	if _, err := os.Lstat(filepath.Join(dir, "d", "a")); !os.IsNotExist(err) {
		t.Fatal("accepted child file was not removed")
	}
	if _, err := os.Lstat(filepath.Join(dir, "d", "sub", "nested")); err != nil {
		t.Fatalf("declined branch was removed: %v", err)
	}
}

func TestRmRejectsDotAndDotDotOperands(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, "keep"), "x")
	for _, operand := range []string{".", "./.", "..", "child/.."} {
		_, errb, code := runTool(t, dir, "-r", operand)
		if code != 1 || !strings.Contains(errb, "refusing to remove") {
			t.Errorf("rm -r %q: code=%d err=%q", operand, code, errb)
		}
	}
	if _, err := os.Lstat(filepath.Join(dir, "keep")); err != nil {
		t.Fatalf("dot operand removed contents: %v", err)
	}
}

func TestRmRejectsDotComponentsBeforeTraversal(t *testing.T) {
	for _, tc := range []struct {
		name      string
		operand   string
		protected string
	}{
		{name: "dot", operand: "d/.", protected: "d/keep"},
		{name: "dot trailing separator", operand: "d/./", protected: "d/keep"},
		{name: "dot dot", operand: "d/..", protected: "keep"},
		{name: "dot dot trailing separator", operand: "d/../", protected: "keep"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			if err := os.Mkdir(filepath.Join(dir, "d"), 0o755); err != nil {
				t.Fatal(err)
			}
			write(t, filepath.Join(dir, "d", "keep"), "inside")
			write(t, filepath.Join(dir, "keep"), "outside")

			out, errb, code := runTool(t, dir, "y\n", "-ri", tc.operand)
			if code != 1 || out != "" || !strings.Contains(errb, "refusing to remove") {
				t.Fatalf("rm -ri %q = (%q, %q, %d), want refusal and exit 1", tc.operand, out, errb, code)
			}
			if strings.Contains(errb, "? ") {
				t.Errorf("rm -ri %q prompted before mandatory refusal: %q", tc.operand, errb)
			}
			if got, err := os.ReadFile(filepath.Join(dir, tc.protected)); err != nil || string(got) == "" {
				t.Errorf("protected file after rm -ri %q = (%q, %v), want preserved", tc.operand, got, err)
			}
		})
	}
}

func TestRmAllowsDotComponentsBeforeFinalName(t *testing.T) {
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "d"), 0o755); err != nil {
		t.Fatal(err)
	}
	write(t, filepath.Join(dir, "victim"), "outside")
	write(t, filepath.Join(dir, "d", "victim"), "inside")

	for _, operand := range []string{"d/./victim", "d/../victim"} {
		_, errb, code := runTool(t, dir, "-f", operand)
		if code != 0 || errb != "" {
			t.Fatalf("rm -f %q = (_, %q, %d), want success", operand, errb, code)
		}
	}
}

func TestRmLastPromptOptionWins(t *testing.T) {
	dir := t.TempDir()
	for _, tc := range []struct {
		name, input string
		args        []string
		wantPresent bool
	}{
		{"force after interactive", "", []string{"-i", "-f", "a"}, false},
		{"interactive after force", "n\n", []string{"-f", "-i", "a"}, true},
		{"cluster force then interactive", "n\n", []string{"-fi", "a"}, true},
		{"cluster interactive then force", "", []string{"-if", "a"}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			write(t, filepath.Join(dir, "a"), "x")
			_, _, code := runToolIn(t, dir, tc.input, tc.args...)
			if code != 0 {
				t.Fatalf("code=%d", code)
			}
			_, err := os.Lstat(filepath.Join(dir, "a"))
			if (err == nil) != tc.wantPresent {
				t.Fatalf("present=%v, want %v", err == nil, tc.wantPresent)
			}
		})
	}
}

func TestRmInteractiveLeadingWhitespaceIsNotAffirmative(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, "victim"), "keep")
	_, errb, code := runToolIn(t, dir, " yes\n", "-i", "victim")
	if code != 0 || !strings.Contains(errb, "remove 'victim'?") {
		t.Fatalf("leading-space reply = (_, %q, %d)", errb, code)
	}
	if _, err := os.Lstat(filepath.Join(dir, "victim")); err != nil {
		t.Fatalf("leading-space reply removed victim: %v", err)
	}
}

func TestRmInteractiveUsesGermanYesexpr(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, "victim"), "remove")
	var out, errb bytes.Buffer
	rc := &tool.RunContext{Ctx: context.Background(), Dir: dir, Env: []string{"LC_MESSAGES=de_DE.UTF-8"}, Stdio: tool.Stdio{In: strings.NewReader("1\n"), Out: &out, Err: &errb}}
	if code := cmd.Run(rc, []string{"-i", "victim"}); code != 0 {
		t.Fatalf("German yesexpr = (_, %q, %d)", errb.String(), code)
	}
	if _, err := os.Lstat(filepath.Join(dir, "victim")); !os.IsNotExist(err) {
		t.Fatalf("German affirmative did not remove victim: %v", err)
	}
}

func TestRmInteractiveRejectsUnsupportedLCMessages(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, "victim"), "keep")
	var out, errb bytes.Buffer
	rc := &tool.RunContext{Ctx: context.Background(), Dir: dir, Env: []string{"LC_MESSAGES=en_US.UTF-8"}, Stdio: tool.Stdio{In: strings.NewReader("yes\n"), Out: &out, Err: &errb}}
	if code := cmd.Run(rc, []string{"-i", "victim"}); code != 1 || !strings.Contains(errb.String(), "unsupported LC_MESSAGES locale") {
		t.Fatalf("unsupported LC_MESSAGES = (_, %q, %d)", errb.String(), code)
	}
	if _, err := os.Lstat(filepath.Join(dir, "victim")); err != nil {
		t.Fatalf("unsupported locale removed victim: %v", err)
	}
}

func TestRmImplicitPromptForUnwritable(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "unwritable")
	write(t, file, "x")
	if err := os.Chmod(file, 0o444); err != nil {
		t.Fatal(err)
	}

	oldIsTerminal := isTerminal
	oldWritableForPrompt := writableForPrompt
	t.Cleanup(func() {
		isTerminal = oldIsTerminal
		writableForPrompt = oldWritableForPrompt
	})
	isTerminal = func(r io.Reader) bool { return true }
	writableForPrompt = func(string) bool { return false }

	// Test 1: terminal is true, file unwritable, no -f, input 'y' -> removes
	_, errb, code := runToolIn(t, dir, "y\n", "unwritable")
	if code != 0 {
		t.Errorf("expected code 0, got %d", code)
	}
	if !strings.Contains(errb, "remove 'unwritable'?") {
		t.Errorf("expected prompt for unwritable file, got %q", errb)
	}
	if _, err := os.Stat(file); err == nil {
		t.Error("file not removed after 'y' to implicit prompt")
	}

	// Test 2: terminal is true, unwritable, input 'n' -> doesn't remove
	file2 := filepath.Join(dir, "unwritable2")
	write(t, file2, "x")
	os.Chmod(file2, 0o444)
	_, errb, code = runToolIn(t, dir, "n\n", "unwritable2")
	if code != 0 {
		t.Errorf("expected code 0 on decline, got %d", code)
	}
	if !strings.Contains(errb, "remove 'unwritable2'?") {
		t.Errorf("expected prompt for unwritable file, got %q", errb)
	}
	if _, err := os.Stat(file2); os.IsNotExist(err) {
		t.Error("file removed after 'n' to implicit prompt")
	}
	// Test 3: non-terminal, unwritable -> NO PROMPT, removes directly
	file3 := filepath.Join(dir, "unwritable3")
	write(t, file3, "x")
	os.Chmod(file3, 0o444)
	isTerminal = func(r io.Reader) bool { return false }
	_, errb, code = runToolIn(t, dir, "", "unwritable3")
	if code != 0 {
		t.Errorf("expected code 0 for non-terminal, got %d", code)
	}
	if strings.Contains(errb, "remove") {
		t.Errorf("unexpected prompt for non-terminal: %q", errb)
	}
	if _, err := os.Stat(file3); err == nil {
		t.Error("file not removed without prompt")
	}

	// Test 4: terminal is true, unwritable directory, input 'n'
	d := filepath.Join(dir, "udir")
	os.Mkdir(d, 0o555)
	isTerminal = func(r io.Reader) bool { return true }
	_, errb, code = runToolIn(t, dir, "n\n", "-r", "udir")
	if code != 0 {
		t.Errorf("expected code 0, got %d", code)
	}
	if !strings.Contains(errb, "remove 'udir'?") {
		t.Errorf("expected prompt for unwritable directory, got %q", errb)
	}
	if _, err := os.Stat(d); os.IsNotExist(err) {
		t.Error("directory removed after 'n' to implicit prompt")
	}
}

func TestRmImplicitDirectoryPromptPrecedesDescent(t *testing.T) {
	dir := t.TempDir()
	protected := filepath.Join(dir, "protected")
	if err := os.Mkdir(protected, 0o755); err != nil {
		t.Fatal(err)
	}
	child := filepath.Join(protected, "child")
	write(t, child, "must survive")
	if err := os.Chmod(protected, 0o555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(protected, 0o755) })

	oldIsTerminal := isTerminal
	oldWritableForPrompt := writableForPrompt
	t.Cleanup(func() {
		isTerminal = oldIsTerminal
		writableForPrompt = oldWritableForPrompt
	})
	isTerminal = func(io.Reader) bool { return true }
	writableForPrompt = func(path string) bool { return path != protected }

	_, errb, code := runToolIn(t, dir, "n\n", "-r", "protected")
	if code != 0 {
		t.Fatalf("declining implicit directory prompt: code=%d stderr=%q", code, errb)
	}
	if !strings.Contains(errb, "remove 'protected'?") {
		t.Fatalf("missing pre-descent prompt: %q", errb)
	}
	if got, err := os.ReadFile(child); err != nil || string(got) != "must survive" {
		t.Fatalf("child changed before declined directory prompt: content=%q err=%v", got, err)
	}
}

// POSIX Issue 7 (2016 edition): "if an operand resolves to the root
// directory, rm shall write a diagnostic message to standard error and do
// nothing more with such operands" — the -d removal path must hit the same
// failsafe as -r, not attempt rmdir() on the root.
func TestRmPreserveRootGuardsDashDRemoval(t *testing.T) {
	dir := t.TempDir()
	guarded := filepath.Join(dir, "guarded")
	if err := os.Mkdir(guarded, 0o755); err != nil {
		t.Fatal(err)
	}

	old := isFilesystemRoot
	isFilesystemRoot = func(path string, followFinal bool) bool {
		return rootguard.SameFile(path, guarded, followFinal)
	}
	t.Cleanup(func() { isFilesystemRoot = old })

	_, errb, code := runTool(t, dir, "-d", "guarded")
	if code != 1 || !strings.Contains(errb, "it is dangerous to operate recursively on 'guarded'") {
		t.Fatalf("rm -d root operand: code=%d err=%q", code, errb)
	}
	if fi, err := os.Stat(guarded); err != nil || !fi.IsDir() {
		t.Fatalf("root-resolving directory was removed by -d: %v", err)
	}
}

// The implicit write-protection prompt (unwritable + terminal, no -f) must
// not fire for symlink operands: unlink() removes the link itself, whose own
// permissions never deny writing, and access(2) would consult the referent
// (or fail outright for a dangling link).
func TestRmImplicitPromptSkipsSymlinkOperands(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, "referent"), "keep")
	link := filepath.Join(dir, "link")
	if err := os.Symlink("referent", link); err != nil {
		t.Skipf("symlinks are unavailable: %v", err)
	}
	dangling := filepath.Join(dir, "dangling")
	if err := os.Symlink("nowhere", dangling); err != nil {
		t.Fatal(err)
	}

	oldIsTerminal := isTerminal
	oldWritableForPrompt := writableForPrompt
	t.Cleanup(func() {
		isTerminal = oldIsTerminal
		writableForPrompt = oldWritableForPrompt
	})
	isTerminal = func(io.Reader) bool { return true }
	writableForPrompt = func(string) bool { return false }

	_, errb, code := runToolIn(t, dir, "", "link", "dangling")
	if code != 0 || errb != "" {
		t.Fatalf("rm symlinks with unwritable referents: code=%d err=%q", code, errb)
	}
	for _, path := range []string{link, dangling} {
		if _, err := os.Lstat(path); !os.IsNotExist(err) {
			t.Fatalf("symlink %q survived without a prompt path: %v", path, err)
		}
	}
	if got, err := os.ReadFile(filepath.Join(dir, "referent")); err != nil || string(got) != "keep" {
		t.Fatalf("referent disturbed: %q %v", got, err)
	}
}

func TestRmInteractiveAfterForceNeedsOperand(t *testing.T) {
	dir := t.TempDir()
	_, errb, code := runTool(t, dir, "-f", "-i")
	if code != 2 || !strings.Contains(errb, "missing operand") {
		t.Errorf("rm -f -i: code=%d err=%q", code, errb)
	}
}
