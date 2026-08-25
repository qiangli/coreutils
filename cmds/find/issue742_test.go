package findcmd

// Issue 742 / Sprint 79 focused POSIX Issue 7 evidence for find,
// complementing issue7_test.go: the POSIXLY_CORRECT path-operand gate
// (SYNOPSIS/OPERANDS), end-to-end LC_ALL category precedence for
// patterns and -ok (ENVIRONMENT_VARIABLES), -exec side effects and
// batched-invocation counting, and multi-operand exit-status
// aggregation (EXIT_STATUS). Unix-only ownership and traversal
// products live in issue742_unix_test.go.

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// TestFindIssue7POSIXModeRequiresPathOperand pins the OPERANDS clause:
// Issue 7 requires one or more path operands, so when POSIXLY_CORRECT
// selects POSIX mode — by presence alone, even to an empty value — a
// no-argument or expression-first invocation is diagnosed as a usage
// error instead of silently defaulting the start point to '.'.
func TestFindIssue7POSIXModeRequiresPathOperand(t *testing.T) {
	dir := setupTree(t)
	for _, val := range []string{"1", ""} {
		env := []string{"POSIXLY_CORRECT=" + val}

		// No operands at all: the missing path operand is named.
		out, errb, code := runFindEnv(t, dir, env)
		if code != 2 || out != "" || !strings.Contains(errb, "missing path operand") {
			t.Errorf("POSIXLY_CORRECT=%q, no operands: (%q, %q, %d), want usage error naming the missing path operand",
				val, out, errb, code)
		}

		// Expression first: the token that cannot start the expression
		// scan is named in the diagnostic.
		for _, args := range [][]string{
			{"-name", "a.txt"},
			{"(", "-name", "a.txt", ")"},
			{"!", "-name", "a.txt"},
		} {
			_, errb, code = runFindEnv(t, dir, env, args...)
			if code != 2 || !strings.Contains(errb, "paths must precede expression") ||
				!strings.Contains(errb, args[0]) {
				t.Errorf("POSIXLY_CORRECT=%q, expression-first %v: code=%d err=%q, want usage error naming %q",
					val, args, code, errb, args[0])
			}
		}

		// A "--" closes the leading options but is not a path operand.
		_, errb, code = runFindEnv(t, dir, env, "--")
		if code != 2 || !strings.Contains(errb, "missing path operand") {
			t.Errorf("POSIXLY_CORRECT=%q, only '--': code=%d err=%q, want missing path operand", val, code, errb)
		}

		// With a path operand the same environment walks normally:
		// POSIX mode changes operand requirements, nothing else.
		out, errb, code = runFindEnv(t, dir, env, ".", "-name", "a.txt")
		if code != 0 || errb != "" || out != "./a.txt\n" {
			t.Errorf("POSIXLY_CORRECT=%q, path given: (%q, %q, %d), want ./a.txt, exit 0", val, out, errb, code)
		}
	}
}

// TestFindIssue7NoPathDefaultIsExtensionOutsidePOSIXMode is the paired
// control for the gate above: without POSIXLY_CORRECT the documented
// no-path default of '.' stands, both for the bare invocation and for
// an expression-first one (a GNU-style extension this repo keeps).
func TestFindIssue7NoPathDefaultIsExtensionOutsidePOSIXMode(t *testing.T) {
	dir := setupTree(t)
	env := []string{"LANG=C", "LC_ALL=C"} // no POSIXLY_CORRECT entry at all
	out, errb, code := runFindEnv(t, dir, env, "-name", "a.txt")
	if code != 0 || errb != "" || out != "./a.txt\n" {
		t.Errorf("no POSIXLY_CORRECT, expression-first: (%q, %q, %d), want the '.' default to fire", out, errb, code)
	}
	out, errb, code = runFindEnv(t, dir, env)
	// The default start point '.': the walk begins at the '.' line and
	// reaches its files (the whole tree, not a diagnostic).
	if code != 0 || errb != "" || !strings.HasPrefix(out, ".\n./a.txt\n") {
		t.Errorf("no POSIXLY_CORRECT, no operands: (%q, %q, %d), want the '.' start point walked", out, errb, code)
	}
}

// TestFindIssue7LCAllPrecedenceForPatterns proves LC_ALL precedence for
// -name patterns end to end through a real walk: a non-empty LC_ALL
// selects the locale category over any LC_CTYPE/LC_COLLATE/LANG value,
// exactly as ENVIRONMENT_VARIABLES defines the precedence order. The
// provisioned de_DE ISO-8859-1 locale classifies the Latin-1 ä byte as
// alphabetic and places it in a's equivalence class; the C/POSIX locale
// does neither. The fixture needs a raw single byte 0xe4 as a filename;
// filesystems that reject non-UTF-8 names (Darwin/APFS, EILSEQ) skip —
// the matcher seam itself is already pinned on every platform by
// TestFindVSCLocalePrecedence and TestFindGermanLocaleCategories.
// -maxdepth is deliberately absent: it is a GNU extension, and the
// single-character patterns already isolate the file.
func TestFindIssue7LCAllPrecedenceForPatterns(t *testing.T) {
	dir := t.TempDir()
	latin1Aumlaut := string([]byte{0xe4})
	if err := os.WriteFile(filepath.Join(dir, latin1Aumlaut), []byte("x"), 0o644); err != nil {
		t.Skipf("a raw Latin-1 filename byte is not spellable on this filesystem: %v", err)
	}
	writeFile(t, dir, "plain", "x")

	// LC_ALL=de_DE wins over every category naming POSIX: the ä file
	// alone matches both a character class and an equivalence class.
	env := []string{
		"LC_ALL=de_DE.iso88591", "LC_CTYPE=POSIX", "LC_COLLATE=POSIX",
		"LC_MESSAGES=POSIX", "LANG=POSIX",
	}
	for _, pat := range []string{"[[:alpha:]]", "[[=a=]]"} {
		out, errb, code := runFindEnv(t, dir, env, ".", "-name", pat)
		if code != 0 || errb != "" || out != "./"+latin1Aumlaut+"\n" {
			t.Errorf("LC_ALL=de_DE -name %q: (%q, %q, %d), want the ä file only", pat, out, errb, code)
		}
	}

	// The categories naming de_DE all lose to a C/POSIX LC_ALL.
	env = []string{
		"LC_ALL=POSIX", "LC_CTYPE=de_DE.iso88591", "LC_COLLATE=de_DE.iso88591",
		"LANG=de_DE.iso88591",
	}
	for _, pat := range []string{"[[:alpha:]]", "[[=a=]]"} {
		out, errb, code := runFindEnv(t, dir, env, ".", "-name", pat)
		if code != 0 || errb != "" || out != "" {
			t.Errorf("LC_ALL=POSIX -name %q: (%q, %q, %d), want no match (LC_ALL outranks the categories)", pat, out, errb, code)
		}
	}
}

// TestFindIssue7LCAllPrecedenceForOKAffirmative proves LC_MESSAGES
// precedence end to end for -ok: LC_ALL selects the affirmative-reply
// expression over LC_MESSAGES, and a C/POSIX LC_ALL suppresses a German
// LC_MESSAGES selection. Reading the reply from stdin is STDIN's only
// documented use in find.
func TestFindIssue7LCAllPrecedenceForOKAffirmative(t *testing.T) {
	dir := setupTree(t)
	bin := helperBin(t)

	// LC_ALL=de_DE beats LC_MESSAGES=POSIX: "ja" affirms, "y" does not.
	env := append(helperEnv(), "LC_ALL=de_DE.iso88591", "LC_MESSAGES=POSIX")
	out, errb, code := runFindExec(t, dir, "ja\n", env,
		".", "-name", "a.txt", "-ok", bin, "{}", ";")
	if code != 0 || out != "./a.txt\n" {
		t.Errorf("LC_ALL=de_DE -ok \"ja\": (%q, %q, %d), want the utility run", out, errb, code)
	}
	out, _, code = runFindExec(t, dir, "y\n", env,
		".", "-name", "a.txt", "-ok", bin, "{}", ";", "-print")
	if code != 0 || out != "" {
		t.Errorf("LC_ALL=de_DE -ok \"y\": (%q, %d), want declined", out, code)
	}

	// LC_MESSAGES=de_DE loses to LC_ALL=POSIX: "ja" declines, "y" affirms.
	env = append(helperEnv(), "LC_MESSAGES=de_DE.iso88591", "LC_ALL=POSIX")
	out, _, code = runFindExec(t, dir, "ja\n", env,
		".", "-name", "a.txt", "-ok", bin, "{}", ";", "-print")
	if code != 0 || out != "" {
		t.Errorf("LC_ALL=POSIX -ok \"ja\": (%q, %d), want declined (LC_ALL outranks LC_MESSAGES)", out, code)
	}
	out, _, code = runFindExec(t, dir, "y\n", env,
		".", "-name", "a.txt", "-ok", bin, "{}", ";")
	if code != 0 || out != "./a.txt\n" {
		t.Errorf("LC_ALL=POSIX -ok \"y\": (%q, %d), want the utility run", out, code)
	}
}

// TestFindIssue7ExecSideEffectsAndBatching pins the EFFECTS clause
// through real filesystem mutation by the spawned children: the ';'
// form runs once per matched path (one MARK before every path in the
// log) and the batched '{} +' form runs once per batch, appending every
// path to a single invocation (MARK appears exactly once, then all
// paths). FIND_HELPER_LOG makes each helper child append its argv
// elements to one file — the side effect the parent then counts.
func TestFindIssue7ExecSideEffectsAndBatching(t *testing.T) {
	dir := setupTree(t)
	bin := helperBin(t)
	paths := []string{"./a.txt", "./b.go", "./empty.txt", "./skipme/e.txt", "./sub/c.txt", "./sub/deep/d.go"}

	logLines := func(t *testing.T, path string) []string {
		t.Helper()
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("no child side effect: %v", err)
		}
		return strings.Split(strings.TrimRight(string(data), "\n"), "\n")
	}

	// ';' form: one invocation per path — MARK precedes every path.
	log := filepath.Join(t.TempDir(), "per-file.log")
	out, errb, code := runFindExec(t, dir, "", helperEnv("FIND_HELPER_QUIET=1", "FIND_HELPER_LOG="+log),
		".", "-type", "f", "-exec", bin, "MARK", "{}", ";")
	if code != 0 || errb != "" || out != "" {
		t.Fatalf("-exec ;: out=%q err=%q code=%d", out, errb, code)
	}
	want := make([]string, 0, 2*len(paths))
	for _, p := range paths {
		want = append(want, "MARK", p)
	}
	if got := logLines(t, log); !reflect.DeepEqual(got, want) {
		t.Errorf("-exec ; side effects:\n got %v\nwant %v", got, want)
	}

	// '{}' + form: one invocation for the whole batch — MARK once, then
	// every path appended in traversal order.
	log = filepath.Join(t.TempDir(), "batch.log")
	out, errb, code = runFindExec(t, dir, "", helperEnv("FIND_HELPER_QUIET=1", "FIND_HELPER_LOG="+log),
		".", "-type", "f", "-exec", bin, "MARK", "{}", "+")
	if code != 0 || errb != "" || out != "" {
		t.Fatalf("-exec {} +: out=%q err=%q code=%d", out, errb, code)
	}
	want = append([]string{"MARK"}, paths...)
	if got := logLines(t, log); !reflect.DeepEqual(got, want) {
		t.Errorf("-exec {} + batching:\n got %v\nwant %v", got, want)
	}
	if n := strings.Count(strings.Join(logLines(t, log), "\n")+"\n", "MARK\n"); n != 1 {
		t.Errorf("-exec {} + ran %d invocations, want exactly 1", n)
	}
}

// TestFindIssue7StatusAggregation pins the EXIT_STATUS clause across
// operands and action classes: a start point that cannot be descended
// is diagnosed and makes the run exit non-zero while later operands are
// still processed; a declined -ok is not an error; and a non-zero
// batched -exec {} + invocation makes find itself exit non-zero even
// though every path was already printed.
func TestFindIssue7StatusAggregation(t *testing.T) {
	dir := setupTree(t)

	// Bad start point first: the good operand still runs and reports.
	out, errb, code := runFind(t, dir, "no-such-start", "sub", "-name", "*.txt")
	if code != 1 || out != "sub/c.txt\n" || !strings.Contains(errb, "no-such-start") {
		t.Errorf("missing start point first: (%q, %q, %d), want sub/c.txt printed, diagnostic, exit 1", out, errb, code)
	}
	// Good operand first: same aggregation, either order.
	out, errb, code = runFind(t, dir, "sub", "no-such-start", "-name", "*.txt")
	if code != 1 || out != "sub/c.txt\n" || !strings.Contains(errb, "no-such-start") {
		t.Errorf("missing start point last: (%q, %q, %d), want sub/c.txt printed, diagnostic, exit 1", out, errb, code)
	}
	// Every operand processed: exit 0.
	out, errb, code = runFind(t, dir, "skipme", "sub", "-name", "*.txt")
	if code != 0 || errb != "" || out != "skipme/e.txt\nsub/c.txt\n" {
		t.Errorf("all-good operands: (%q, %q, %d), want exit 0", out, errb, code)
	}

	// A declined -ok is not an error: exit stays 0 (EXIT_STATUS: "0
	// when every operand was processed, including when an -ok reply
	// declines an invocation").
	bin := helperBin(t)
	out, _, code = runFindExec(t, dir, "n\n", helperEnv(),
		".", "-name", "a.txt", "-ok", bin, "{}", ";", "-print")
	if code != 0 || out != "" {
		t.Errorf("declined -ok: (%q, %d), want exit 0 with no output", out, code)
	}

	// A failing batched {} + invocation sets exit 1 while the paths it
	// aggregated were still printed.
	out, _, code = runFindExec(t, dir, "", helperEnv("FIND_HELPER_QUIET=1", "FIND_HELPER_EXIT=3"),
		".", "-name", "a.txt", "-exec", bin, "{}", "+", "-print")
	if code != 1 || out != "./a.txt\n" {
		t.Errorf("failing -exec {} +: (%q, %d), want ./a.txt printed and exit 1", out, code)
	}
}
