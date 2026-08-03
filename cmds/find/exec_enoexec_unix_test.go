//go:build unix

package findcmd

// POSIX execvp semantics for -exec/-ok's utility: a file that is present
// and executable but not a recognized binary (ENOEXEC — the classic case
// is a shebang-less shell script) is retried through the shell as
// `sh <file> [args...]`. GNU find gets this for free from execvp; these
// tests pin the explicit fallback runArgv reproduces.
//
// Unix only: the fallback shells out to /bin/sh, which is a POSIX concept
// (shellPath is "" on Windows, so there is no retry to exercise).

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeScript drops a shebang-LESS, executable shell script at dir/name.
// Because it has no #! line it is not a valid binary: a raw execve of it
// fails with ENOEXEC, which is exactly the condition the shell fallback
// exists to handle.
func writeScript(t *testing.T, dir, name, body string) {
	t.Helper()
	full := filepath.Join(dir, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
}

// TestFindExecShebanglessScript is the core regression: -exec of a
// shebang-less script must run it (via the shell) and substitute {}, not
// fail with "Exec format error". The path to the utility is spelled the
// several ways the POSIX/VSC utility_name assertions use (GA61/64/65/66/69):
// a parent-relative path, a subdirectory path, a "/./" path, a "/../"
// round-trip, a doubled-slash path, and a "./" path. Every spelling must
// resolve to the same script and run it.
func TestFindExecShebanglessScript(t *testing.T) {
	dir := setupTree(t)
	// The script echoes its single argument so we can prove {} reached it.
	writeScript(t, dir, "sub/tracer", "echo TRACED \"$1\"\n")

	for _, util := range []string{
		"sub/tracer",        // GA64: relative subpath
		"sub/./tracer",      // GA65: '/./' in the path
		"sub/../sub/tracer", // GA66: '/../' round-trip
		"sub//tracer",       // GA66: doubled slash
		"./sub/tracer",      // GA69: leading './'
	} {
		out, errb, code := runFindExec(t, dir, "", os.Environ(),
			".", "-name", "a.txt", "-exec", util, "{}", ";")
		if code != 0 || out != "TRACED ./a.txt\n" || errb != "" {
			t.Errorf("-exec %s: (out=%q err=%q code=%d), want (%q, \"\", 0)",
				util, out, errb, code, "TRACED ./a.txt\n")
		}
	}
}

// TestFindOkShebanglessPathResolution applies the VSC GA60/61/64/65/66/69
// utility-name spellings to -ok itself. The tracer deliberately has no
// shebang: after an affirmative reply, find must preserve the spelling's
// pathname semantics and take the same ENOEXEC shell fallback as -exec.
// A negative reply must not execute the tracer for the second match.
func TestFindOkShebanglessPathResolution(t *testing.T) {
	root := t.TempDir()
	work := filepath.Join(root, "work")
	if err := os.Mkdir(work, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(work, "find_dir_ok"), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"A", "B"} {
		writeScript(t, filepath.Join(work, "find_dir_ok"), name, "")
	}

	// Use only shell parameter expansion and redirection in the tracer, so
	// this remains hermetic and does not depend on basename(1) or touch(1).
	body := "name=${1##*/}\n: > \"$name.found\"\n"
	writeScript(t, root, "tracer", body)
	writeScript(t, work, "tracer", body)
	writeScript(t, work, "find_dir_94/tracer", body)
	writeScript(t, work, "find_dir_95/tracer", body)
	writeScript(t, work, "find_dir_96/tracer", body)

	abs, err := filepath.Abs(filepath.Join(work, "tracer"))
	if err != nil {
		t.Fatal(err)
	}
	utilities := []string{
		abs,                                 // GA60: absolute pathname
		"../tracer",                         // GA61: parent-relative pathname
		"find_dir_94/tracer",                // GA64: relative subpath
		"find_dir_95/./tracer",              // GA65: embedded dot component
		"find_dir_96/../find_dir_96/tracer", // GA66: parent round-trip
		"find_dir_96//tracer",               // GA66: doubled slash
		"./tracer",                          // GA69: explicit current directory
	}
	for _, utility := range utilities {
		t.Run(utility, func(t *testing.T) {
			for _, found := range []string{"A.found", "B.found"} {
				if err := os.Remove(filepath.Join(work, found)); err != nil && !os.IsNotExist(err) {
					t.Fatal(err)
				}
			}
			out, errb, code := runFindExec(t, work, "y\nn\n", os.Environ(),
				"find_dir_ok", "-name", "?", "-ok", utility, "{}", ";")
			if code != 0 || out != "" {
				t.Fatalf("-ok %q: out=%q err=%q code=%d, want empty output and exit 0", utility, out, errb, code)
			}
			if _, err := os.Stat(filepath.Join(work, "A.found")); err != nil {
				t.Errorf("-ok %q affirmative match was not executed: %v; prompt=%q", utility, err, errb)
			}
			if _, err := os.Stat(filepath.Join(work, "B.found")); !os.IsNotExist(err) {
				t.Errorf("-ok %q negative match executed; stat error=%v; prompt=%q", utility, err, errb)
			}
			if !strings.Contains(errb, "find_dir_ok/A") || !strings.Contains(errb, "find_dir_ok/B") {
				t.Errorf("-ok %q prompts do not identify both paths: %q", utility, errb)
			}
		})
	}
}

// TestFindExecShebanglessParentRelative covers GA61: the utility named by
// a path that climbs out of the start directory ("../tracer"). Run find
// from a subdirectory so the script sits one level up.
func TestFindExecShebanglessParentRelative(t *testing.T) {
	dir := setupTree(t)
	writeScript(t, dir, "tracer", "echo HIT \"$1\"\n")

	sub := filepath.Join(dir, "sub")
	out, errb, code := runFindExec(t, sub, "", os.Environ(),
		".", "-name", "c.txt", "-exec", "../tracer", "{}", ";")
	if code != 0 || out != "HIT ./c.txt\n" || errb != "" {
		t.Errorf("-exec ../tracer: (out=%q err=%q code=%d), want (%q, \"\", 0)",
			out, errb, code, "HIT ./c.txt\n")
	}
}

// TestFindExecShebanglessEmbeddedBrace pins that {} substitution feeds the
// shell fallback too: an embedded {} inside a fixed argument is expanded
// before the utility runs, exactly as for a binary utility.
func TestFindExecShebanglessEmbeddedBrace(t *testing.T) {
	dir := setupTree(t)
	writeScript(t, dir, "tracer", "echo \"$1\"\n")

	out, _, code := runFindExec(t, dir, "", os.Environ(),
		".", "-name", "a.txt", "-exec", "./tracer", "pre[{}]post", ";")
	if code != 0 || out != "pre[./a.txt]post\n" {
		t.Errorf("embedded {} via shell fallback: out=%q code=%d, want %q",
			out, code, "pre[./a.txt]post\n")
	}
}

// TestFindExecShebanglessStatus checks the utility's exit status still
// drives the primary's truth value once it runs through the shell: a
// zero-exit script makes -exec true (chained -print fires), a non-zero
// one makes it false (no -print), and find's own exit stays 0 for the ;
// form either way.
func TestFindExecShebanglessStatus(t *testing.T) {
	dir := setupTree(t)

	writeScript(t, dir, "yes", "exit 0\n")
	out, _, code := runFindExec(t, dir, "", os.Environ(),
		".", "-name", "a.txt", "-exec", "./yes", "{}", ";", "-print")
	if code != 0 || out != "./a.txt\n" {
		t.Errorf("zero-exit script: out=%q code=%d, want %q exit 0", out, code, "./a.txt\n")
	}

	writeScript(t, dir, "no", "exit 3\n")
	out, _, code = runFindExec(t, dir, "", os.Environ(),
		".", "-name", "a.txt", "-exec", "./no", "{}", ";", "-print")
	if code != 0 || out != "" {
		t.Errorf("non-zero script: out=%q code=%d, want no output exit 0", out, code)
	}
}

// TestFindExecShebanglessPlus pins the batched '{} +' form through the
// shell fallback: one invocation gets every path appended, and a non-zero
// batch makes find itself exit 1 (unlike the ; form).
func TestFindExecShebanglessPlus(t *testing.T) {
	dir := setupTree(t)
	// Print every positional argument, one per line.
	writeScript(t, dir, "tracer", "for a in \"$@\"; do echo \"$a\"; done\n")

	out, errb, code := runFindExec(t, dir, "", os.Environ(),
		".", "-name", "*.go", "-exec", "./tracer", "MARK", "{}", "+")
	want := "MARK\n./b.go\n./sub/deep/d.go\n"
	if code != 0 || out != want || errb != "" {
		t.Errorf("-exec ... +: (out=%q err=%q code=%d), want (%q, \"\", 0)", out, errb, code, want)
	}

	writeScript(t, dir, "fail", "exit 5\n")
	_, _, code = runFindExec(t, dir, "", os.Environ(),
		".", "-type", "f", "-exec", "./fail", "{}", "+")
	if code != 1 {
		t.Errorf("-exec fail +: code=%d, want 1", code)
	}
}

// TestFindExecBinaryUnaffected is a guard: a genuine binary (this test
// executable) still runs by direct execve, and a name that truly does not
// exist still fails loudly — the fallback fires only on ENOEXEC, never
// masking a not-found or turning every utility into a shell script.
func TestFindExecBinaryUnaffected(t *testing.T) {
	dir := setupTree(t)
	bin := helperBin(t)
	out, errb, code := runFindExec(t, dir, "", helperEnv(),
		".", "-name", "a.txt", "-exec", bin, "{}", ";")
	if code != 0 || out != "./a.txt\n" {
		t.Errorf("binary utility: out=%q code=%d err=%q", out, code, errb)
	}

	out, errb, code = runFindExec(t, dir, "", os.Environ(),
		".", "-maxdepth", "0", "-exec", "./does-not-exist-zzz", "{}", ";", "-print")
	if code != 1 || out != "" || !strings.Contains(errb, "does-not-exist-zzz") {
		t.Errorf("missing utility: out=%q err=%q code=%d, want not-found exit 1", out, errb, code)
	}
}
