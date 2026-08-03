package findcmd

import (
	"bufio"
	"bytes"
	"context"
	"os"
	"os/user"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/qiangli/coreutils/tool"
)

// runFind is the canonical test harness shape for cmds packages.
func runFind(t *testing.T, dir string, args ...string) (stdout, stderr string, code int) {
	t.Helper()
	if dir == "" {
		dir = t.TempDir()
	}
	var out, errb bytes.Buffer
	rc := &tool.RunContext{
		Ctx:   context.Background(),
		Dir:   dir,
		Stdio: tool.Stdio{In: strings.NewReader(""), Out: &out, Err: &errb},
	}
	code = cmd.Run(rc, args)
	return out.String(), errb.String(), code
}

func writeFile(t *testing.T, dir, name, content string) {
	t.Helper()
	full := filepath.Join(dir, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func setupTree(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	writeFile(t, dir, "a.txt", "hello")      // 5 bytes
	writeFile(t, dir, "b.go", "package b\n") // 10 bytes
	writeFile(t, dir, "empty.txt", "")
	writeFile(t, dir, "skipme/e.txt", "x\n")
	writeFile(t, dir, "sub/c.txt", "0123456789") // 10 bytes
	writeFile(t, dir, "sub/deep/d.go", "package d\n")
	return dir
}

func TestFindDefaultPrintLexical(t *testing.T) {
	dir := setupTree(t)
	out, _, code := runFind(t, dir, ".")
	want := ".\n./a.txt\n./b.go\n./empty.txt\n./skipme\n./skipme/e.txt\n./sub\n./sub/c.txt\n./sub/deep\n./sub/deep/d.go\n"
	if out != want || code != 0 {
		t.Errorf("find . = (%q, %d), want (%q, 0)", out, code, want)
	}
}

func TestFindTests(t *testing.T) {
	dir := setupTree(t)
	cases := []struct {
		args []string
		want string
	}{
		{[]string{".", "-name", "*.go"}, "./b.go\n./sub/deep/d.go\n"},
		{[]string{".", "-iname", "A.TXT"}, "./a.txt\n"},
		{[]string{".", "-type", "d"}, ".\n./skipme\n./sub\n./sub/deep\n"},
		{[]string{".", "-type", "f", "-name", "c.txt"}, "./sub/c.txt\n"},
		{[]string{".", "-maxdepth", "1", "-type", "f"}, "./a.txt\n./b.go\n./empty.txt\n"},
		{[]string{".", "-mindepth", "2", "-name", "*.txt"}, "./skipme/e.txt\n./sub/c.txt\n"},
		{[]string{".", "-maxdepth", "0"}, ".\n"},
		{[]string{".", "-path", "*deep*"}, "./sub/deep\n./sub/deep/d.go\n"},
		{[]string{".", "-path", "./sub/*", "-type", "f"}, "./sub/c.txt\n./sub/deep/d.go\n"},
		{[]string{".", "-type", "f", "-size", "5c"}, "./a.txt\n"},
		{[]string{".", "-type", "f", "-size", "+5c"}, "./b.go\n./sub/c.txt\n./sub/deep/d.go\n"},
		{[]string{".", "-size", "-6c", "-type", "f", "-name", "*.txt", "!", "-empty"},
			"./a.txt\n./skipme/e.txt\n"},
		// GNU round-up gotcha: -size -1k matches only 0-byte files
		{[]string{".", "-type", "f", "-size", "-1k"}, "./empty.txt\n"},
		{[]string{".", "-empty"}, "./empty.txt\n"},
		// default block unit: every small file is 1 512-block
		{[]string{".", "-type", "f", "-size", "1", "-name", "a*"}, "./a.txt\n"},
		// operators
		{[]string{".", "-name", "skipme", "-prune", "-o", "-name", "*.txt", "-print"},
			"./a.txt\n./empty.txt\n./sub/c.txt\n"},
		{[]string{".", "-type", "f", "!", "-name", "*.txt"}, "./b.go\n./sub/deep/d.go\n"},
		{[]string{".", "-type", "f", "-not", "-name", "*.txt"}, "./b.go\n./sub/deep/d.go\n"},
		{[]string{".", "(", "-name", "a.txt", "-o", "-name", "b.go", ")"}, "./a.txt\n./b.go\n"},
		{[]string{".", "-name", "a.txt", "-a", "-type", "f"}, "./a.txt\n"},
		// explicit operand path prefixes output
		{[]string{"sub", "-name", "*.go"}, "sub/deep/d.go\n"},
		{[]string{"sub/", "-type", "d"}, "sub/\nsub/deep\n"},
		// multiple start points
		{[]string{"skipme", "sub", "-name", "*.txt"}, "skipme/e.txt\nsub/c.txt\n"},
	}
	for _, c := range cases {
		out, errb, code := runFind(t, dir, c.args...)
		if out != c.want || code != 0 {
			t.Errorf("find %v = (%q, %d, err=%q), want (%q, 0)", c.args, out, code, errb, c.want)
		}
	}
}

func TestFindDefaultPath(t *testing.T) {
	dir := setupTree(t)
	out, _, code := runFind(t, dir, "-name", "a.txt")
	if out != "./a.txt\n" || code != 0 {
		t.Errorf("default path: out=%q code=%d", out, code)
	}
}

func TestFindPrint0(t *testing.T) {
	dir := setupTree(t)
	out, _, code := runFind(t, dir, ".", "-name", "*.go", "-print0")
	if out != "./b.go\x00./sub/deep/d.go\x00" || code != 0 {
		t.Errorf("-print0: out=%q code=%d", out, code)
	}
}

func TestFindMtimeAndNewer(t *testing.T) {
	dir := setupTree(t)
	old := filepath.Join(dir, "old.txt")
	writeFile(t, dir, "old.txt", "old\n")
	threeDays := time.Now().Add(-72*time.Hour - time.Hour)
	if err := os.Chtimes(old, threeDays, threeDays); err != nil {
		t.Fatal(err)
	}

	out, _, code := runFind(t, dir, ".", "-type", "f", "-mtime", "+2")
	if out != "./old.txt\n" || code != 0 {
		t.Errorf("-mtime +2: out=%q code=%d", out, code)
	}
	out, _, _ = runFind(t, dir, ".", "-name", "old.txt", "-mtime", "0")
	if out != "" {
		t.Errorf("-mtime 0 matched old file: out=%q", out)
	}
	out, _, _ = runFind(t, dir, ".", "-name", "old.txt", "-mtime", "3")
	if out != "./old.txt\n" {
		t.Errorf("-mtime 3: out=%q", out)
	}

	out, _, code = runFind(t, dir, ".", "-type", "f", "!", "-newer", "old.txt")
	if out != "./old.txt\n" || code != 0 {
		t.Errorf("! -newer: out=%q code=%d", out, code)
	}
	out, _, _ = runFind(t, dir, ".", "-newer", "old.txt", "-name", "*.go")
	if out != "./b.go\n./sub/deep/d.go\n" {
		t.Errorf("-newer: out=%q", out)
	}
}

func TestFindTypeSymlink(t *testing.T) {
	dir := setupTree(t)
	if err := os.Symlink(filepath.Join(dir, "a.txt"), filepath.Join(dir, "link")); err != nil {
		t.Skipf("symlink not supported: %v", err)
	}
	out, _, code := runFind(t, dir, ".", "-type", "l")
	if out != "./link\n" || code != 0 {
		t.Errorf("-type l: out=%q code=%d", out, code)
	}
	// -type f must not match the symlink (lstat semantics, -P default)
	out, _, _ = runFind(t, dir, ".", "-name", "link", "-type", "f")
	if out != "" {
		t.Errorf("-type f matched symlink: out=%q", out)
	}
}

func TestFindErrors(t *testing.T) {
	dir := setupTree(t)

	_, errb, code := runFind(t, dir, "missing")
	if code != 1 || !strings.Contains(errb, "missing") {
		t.Errorf("missing start point: code=%d err=%q", code, errb)
	}
	_, errb, code = runFind(t, dir, ".", "-frobnicate")
	if code != 2 || !strings.Contains(errb, "unknown predicate '-frobnicate'") {
		t.Errorf("unknown predicate: code=%d err=%q", code, errb)
	}
	_, errb, code = runFind(t, dir, ".", "-name")
	if code != 2 || !strings.Contains(errb, "missing argument to '-name'") {
		t.Errorf("missing argument: code=%d err=%q", code, errb)
	}
	_, errb, code = runFind(t, dir, ".", "-name", "x", "stray")
	if code != 2 || !strings.Contains(errb, "paths must precede expression") {
		t.Errorf("stray operand: code=%d err=%q", code, errb)
	}
	_, errb, code = runFind(t, dir, ".", "(", "-name", "x")
	if code != 2 || !strings.Contains(errb, "expected ')'") {
		t.Errorf("unmatched paren: code=%d err=%q", code, errb)
	}
	_, errb, code = runFind(t, dir, ".", "-type", "q")
	if code != 2 || !strings.Contains(errb, "unknown argument to -type") {
		t.Errorf("bad type: code=%d err=%q", code, errb)
	}
	_, errb, code = runFind(t, dir, ".", "-mtime", "x")
	if code != 2 || !strings.Contains(errb, "-mtime") {
		t.Errorf("bad mtime: code=%d err=%q", code, errb)
	}
	for _, action := range []string{"-execdir", "-okdir", "-delete"} {
		_, errb, code = runFind(t, dir, ".", action)
		if code != 2 || !strings.Contains(errb, "not supported") {
			t.Errorf("%s: code=%d err=%q", action, code, errb)
		}
	}
	for _, args := range [][]string{
		{".", "-exec"},
		{".", "-exec", "echo", "{}"}, // no terminating ';'
		{".", "-exec", ";"},          // no utility name
		{".", "-ok", "echo", "{}"},
	} {
		_, errb, code = runFind(t, dir, args...)
		if code != 2 || !strings.Contains(errb, "missing argument") {
			t.Errorf("find %v: code=%d err=%q, want exit 2 missing-argument", args, code, errb)
		}
	}
	_, errb, code = runFind(t, dir, ".", "-perm", "97")
	if code != 2 || !strings.Contains(errb, "invalid mode") {
		t.Errorf("-perm 97: code=%d err=%q", code, errb)
	}
	if runtime.GOOS != "windows" {
		_, errb, code = runFind(t, dir, ".", "-user", "no-such-user-xyz-12345")
		if code != 2 || !strings.Contains(errb, "not the name of a known user") {
			t.Errorf("-user unknown: code=%d err=%q", code, errb)
		}
	}
}

func TestFindDepthOrder(t *testing.T) {
	dir := setupTree(t)
	out, _, code := runFind(t, dir, ".", "-depth")
	want := "./a.txt\n./b.go\n./empty.txt\n./skipme/e.txt\n./skipme\n./sub/c.txt\n./sub/deep/d.go\n./sub/deep\n./sub\n.\n"
	if out != want || code != 0 {
		t.Errorf("-depth = (%q, %d), want (%q, 0)", out, code, want)
	}
}

func TestFindXdev(t *testing.T) {
	if runtime.GOOS == "windows" {
		dir := setupTree(t)
		_, errb, code := runFind(t, dir, ".", "-xdev")
		if code != 2 || !strings.Contains(errb, "not supported") {
			t.Errorf("-xdev on windows: code=%d err=%q", code, errb)
		}
		return
	}
	dir := setupTree(t)
	plain, _, _ := runFind(t, dir, ".")
	out, _, code := runFind(t, dir, ".", "-xdev")
	if out != plain || code != 0 {
		t.Errorf("-xdev on one filesystem: out=%q code=%d, want same as plain %q", out, code, plain)
	}
}

func TestFindPerm(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("windows has no full unix permission bits")
	}
	dir := setupTree(t)
	chmod := func(name string, m os.FileMode) {
		if err := os.Chmod(filepath.Join(dir, name), m); err != nil {
			t.Fatal(err)
		}
	}
	chmod("a.txt", 0o644)
	chmod("b.go", 0o755)
	chmod("empty.txt", 0o600)
	cases := []struct {
		perm string
		want string
	}{
		{"644", "./a.txt\n"},
		{"-755", "./b.go\n"},
		{"-u+w", "./a.txt\n./b.go\n./empty.txt\n"}, // symbolic, all-bits-set
		{"u=rw", "./empty.txt\n"},                  // symbolic exact (0600)
		{"/g+r", "./a.txt\n./b.go\n"},              // any-bit
	}
	for _, c := range cases {
		out, errb, code := runFind(t, dir, ".", "-maxdepth", "1", "-type", "f", "-perm", c.perm)
		if out != c.want || code != 0 {
			t.Errorf("-perm %s = (%q, %d, err=%q), want (%q, 0)", c.perm, out, code, errb, c.want)
		}
	}
}

func TestFindLinks(t *testing.T) {
	if runtime.GOOS == "windows" {
		dir := setupTree(t)
		_, errb, code := runFind(t, dir, ".", "-links", "1")
		if code != 2 || !strings.Contains(errb, "not supported") {
			t.Errorf("-links on windows: code=%d err=%q", code, errb)
		}
		return
	}
	dir := setupTree(t)
	if err := os.Link(filepath.Join(dir, "a.txt"), filepath.Join(dir, "hard.txt")); err != nil {
		t.Skipf("hard links not supported: %v", err)
	}
	out, _, code := runFind(t, dir, ".", "-type", "f", "-links", "2")
	if out != "./a.txt\n./hard.txt\n" || code != 0 {
		t.Errorf("-links 2: out=%q code=%d", out, code)
	}
	out, _, _ = runFind(t, dir, ".", "-maxdepth", "1", "-type", "f", "-links", "1")
	if out != "./b.go\n./empty.txt\n" {
		t.Errorf("-links 1: out=%q", out)
	}
	out, _, _ = runFind(t, dir, ".", "-type", "f", "-links", "+1")
	if out != "./a.txt\n./hard.txt\n" {
		t.Errorf("-links +1: out=%q", out)
	}
}

func TestFindUserGroup(t *testing.T) {
	if runtime.GOOS == "windows" {
		dir := setupTree(t)
		for _, args := range [][]string{{".", "-user", "x"}, {".", "-nouser"}} {
			_, errb, code := runFind(t, dir, args...)
			if code != 2 || !strings.Contains(errb, "not supported") {
				t.Errorf("find %v on windows: code=%d err=%q", args, code, errb)
			}
		}
		return
	}
	dir := setupTree(t)
	me, err := user.Current()
	if err != nil {
		t.Skipf("no current user: %v", err)
	}
	all, _, _ := runFind(t, dir, ".")
	// Everything in a fresh tempdir belongs to us: by name and by ID.
	for _, spec := range []string{me.Username, me.Uid} {
		out, _, code := runFind(t, dir, ".", "-user", spec)
		if out != all || code != 0 {
			t.Errorf("-user %s: out=%q code=%d, want all files", spec, out, code)
		}
	}
	out, _, code := runFind(t, dir, ".", "-group", me.Gid)
	if out != all || code != 0 {
		t.Errorf("-group %s: out=%q code=%d, want all files", me.Gid, out, code)
	}
	// Our uid/gid are known, so -nouser/-nogroup match nothing.
	for _, pred := range []string{"-nouser", "-nogroup"} {
		out, _, code := runFind(t, dir, ".", pred)
		if out != "" || code != 0 {
			t.Errorf("%s: out=%q code=%d, want no matches", pred, out, code)
		}
	}
	out, _, _ = runFind(t, dir, ".", "!", "-nouser")
	if out != all {
		t.Errorf("! -nouser: out=%q, want all files", out)
	}
}

func TestFindAtimeCtime(t *testing.T) {
	if runtime.GOOS != "linux" && runtime.GOOS != "darwin" {
		t.Skip("-atime/-ctime wired on linux and darwin only")
	}
	dir := setupTree(t)
	old := filepath.Join(dir, "a.txt")
	threeDays := time.Now().Add(-72*time.Hour - time.Hour)
	if err := os.Chtimes(old, threeDays, time.Now()); err != nil {
		t.Fatal(err)
	}
	out, _, code := runFind(t, dir, ".", "-type", "f", "-atime", "+2")
	if out != "./a.txt\n" || code != 0 {
		t.Errorf("-atime +2: out=%q code=%d", out, code)
	}
	// Every file was just created: ctime is within the current 24h period.
	out, _, code = runFind(t, dir, ".", "-name", "b.go", "-ctime", "0")
	if out != "./b.go\n" || code != 0 {
		t.Errorf("-ctime 0: out=%q code=%d", out, code)
	}
	out, _, _ = runFind(t, dir, ".", "-name", "b.go", "-ctime", "+1")
	if out != "" {
		t.Errorf("-ctime +1 matched a fresh file: out=%q", out)
	}
}

func TestFindSymlinkFollow(t *testing.T) {
	dir := setupTree(t)
	if err := os.Symlink("sub", filepath.Join(dir, "linkdir")); err != nil {
		t.Skipf("symlink not supported: %v", err)
	}
	if err := os.Symlink("missing", filepath.Join(dir, "dangle")); err != nil {
		t.Fatal(err)
	}

	// -P (default): symlinks are never followed.
	out, _, code := runFind(t, dir, ".", "-name", "c.txt")
	if out != "./sub/c.txt\n" || code != 0 {
		t.Errorf("-P -name c.txt: out=%q code=%d", out, code)
	}
	// -L follows: the tree under linkdir appears too.
	out, _, code = runFind(t, dir, "-L", ".", "-name", "c.txt")
	if out != "./linkdir/c.txt\n./sub/c.txt\n" || code != 0 {
		t.Errorf("-L -name c.txt: out=%q code=%d", out, code)
	}
	// -L: a followed dir link is a directory; a dangling link stays 'l'
	// (POSIX: broken links are evaluated as the link itself) — silently,
	// with a success exit.
	out, errb, code := runFind(t, dir, "-L", ".", "-type", "l")
	if out != "./dangle\n" || code != 0 || errb != "" {
		t.Errorf("-L -type l: out=%q code=%d err=%q", out, code, errb)
	}
	out, _, _ = runFind(t, dir, "-L", ".", "-name", "dangle", "-type", "f")
	if out != "" {
		t.Errorf("-L -type f matched dangling link: out=%q", out)
	}
	// -H: follow the link only when it is the start point itself.
	out, _, code = runFind(t, dir, "-H", "linkdir", "-name", "c.txt")
	if out != "linkdir/c.txt\n" || code != 0 {
		t.Errorf("-H linkdir: out=%q code=%d", out, code)
	}
	out, _, _ = runFind(t, dir, "-H", ".", "-name", "c.txt")
	if out != "./sub/c.txt\n" {
		t.Errorf("-H . followed a non-operand symlink: out=%q", out)
	}
	// Last of -H/-L/-P wins.
	out, _, _ = runFind(t, dir, "-L", "-P", ".", "-name", "c.txt")
	if out != "./sub/c.txt\n" {
		t.Errorf("-L -P: out=%q", out)
	}
}

func TestFindSymlinkLoop(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "f.txt", "x")
	if err := os.Symlink(".", filepath.Join(dir, "loop")); err != nil {
		t.Skipf("symlink not supported: %v", err)
	}
	// Must terminate, diagnose the loop, and exit non-zero.
	out, errb, code := runFind(t, dir, "-L", ".")
	if code != 1 || !strings.Contains(errb, "loop") {
		t.Errorf("-L on loop: code=%d err=%q", code, errb)
	}
	if out != ".\n./f.txt\n" {
		t.Errorf("-L on loop: out=%q", out)
	}
	// A self-referential link cannot resolve (ELOOP): diagnosed, then
	// evaluated as the link itself — not fatal, not silent.
	if err := os.Symlink("self", filepath.Join(dir, "self")); err == nil {
		out, errb, code = runFind(t, dir, "-L", ".", "-name", "self", "-type", "l")
		if code != 1 || !strings.Contains(errb, "self") || out != "./self\n" {
			t.Errorf("-L self-loop: out=%q code=%d err=%q", out, code, errb)
		}
	}
}

func TestFindHelpAndVersion(t *testing.T) {
	out, _, code := runFind(t, "", "--help")
	if code != 0 || !strings.Contains(out, "Usage: find") {
		t.Errorf("--help: code=%d out=%q", code, out)
	}
	out, _, code = runFind(t, "", "--version")
	if code != 0 || !strings.Contains(out, "find") {
		t.Errorf("--version: code=%d out=%q", code, out)
	}
}

func TestFnmatch(t *testing.T) {
	cases := []struct {
		pat, s string
		fold   bool
		want   bool
	}{
		{"*.go", "main.go", false, true},
		{"*.go", "main.gox", false, false},
		{"a?c", "abc", false, true},
		{"[a-c]x", "bx", false, true},
		{"[!a-c]x", "bx", false, false},
		{"[!a-c]x", "dx", false, true},
		{"[]x]", "]", false, true},
		{"*deep*", "./sub/deep/d.go", false, true}, // '*' crosses '/'
		{"a/?/b", "a/x/b", false, true},            // '?' crosses '/' too
		{`a\*b`, "a*b", false, true},
		{`a\*b`, "axb", false, false},
		{"ABC", "abc", true, true},
		{"[A-Z]x", "bx", true, true},
		{"[[:digit:]]*", "7z", false, true},
		{"[[:digit:]]*", "z7", false, false},
		// VSC/PCTS find TP34: equivalence classes are bracket elements,
		// equal to their single member in the C locale.
		{"*[[=C=]b]*", "Abc", false, true},
		{"*[[=a=]]*", "aBc", false, true},
		{"[[=a=]]", "a", false, true},
		{"[[=a=]]", "A", false, false},
		{"[[=a=]]", "A", true, true},
		// VSC/PCTS find TP38: a named collating symbol denotes the literal
		// byte; edge-position hyphens remain literal too.
		{"a[[.-.]]b", "a-b", false, true},
		{"a[-xy]b", "a-b", false, true},
		{"a[xy-]b", "a-b", false, true},
		{"a[!-]b", "a-b", false, false},
	}
	for _, c := range cases {
		if got := fnmatch(c.pat, c.s, c.fold); got != c.want {
			t.Errorf("fnmatch(%q, %q, fold=%v) = %v, want %v", c.pat, c.s, c.fold, got, c.want)
		}
	}
}

func TestFindGermanLocaleCategories(t *testing.T) {
	latin1Aumlaut := string([]byte{0xe4})
	german := findLocaleFromEnv([]string{"LANG=POSIX", "LC_CTYPE=de_DE.iso88591", "LC_COLLATE=de_DE.iso88591"})
	if !fnmatchLocale("[[:alpha:]]", latin1Aumlaut, false, german) {
		t.Error("de_DE LC_CTYPE did not classify Latin-1 ä as alphabetic")
	}
	if !fnmatchLocale("[[=a=]]", latin1Aumlaut, false, german) {
		t.Error("de_DE LC_COLLATE did not place Latin-1 ä in a's equivalence class")
	}
	posix := findLocaleFromEnv([]string{"LANG=de_DE.iso88591", "LC_CTYPE=de_DE.iso88591", "LC_COLLATE=de_DE.iso88591", "LC_ALL=POSIX"})
	if fnmatchLocale("[[:alpha:]]", latin1Aumlaut, false, posix) || fnmatchLocale("[[=a=]]", latin1Aumlaut, false, posix) {
		t.Error("non-empty LC_ALL did not override lower locale categories")
	}
	emptyHigher := findLocaleFromEnv([]string{"LANG=de_DE.iso88591", "LC_CTYPE=", "LC_COLLATE=", "LC_ALL="})
	if !fnmatchLocale("[[:alpha:]]", latin1Aumlaut, false, emptyHigher) || !fnmatchLocale("[[=a=]]", latin1Aumlaut, false, emptyHigher) {
		t.Error("empty higher-priority variables did not fall back to LANG")
	}
}

func TestFindGermanAffirmativeResponse(t *testing.T) {
	confirm := func(env []string, input string) bool {
		var errb bytes.Buffer
		w := &walker{
			rc:     &tool.RunContext{Stdio: tool.Stdio{Err: &errb}},
			stdin:  bufio.NewReader(strings.NewReader(input)),
			locale: findLocaleFromEnv(env),
		}
		return w.confirm("echo", "file")
	}
	if !confirm([]string{"LC_MESSAGES=de_DE.iso88591"}, "ja\n") {
		t.Error("de_DE LC_MESSAGES did not accept German affirmative response")
	}
	if confirm([]string{"LC_MESSAGES=de_DE.iso88591"}, "yes\n") {
		t.Error("de_DE LC_MESSAGES incorrectly used the POSIX yes expression")
	}
	if !confirm([]string{"LANG=POSIX"}, "yes\n") {
		t.Error("POSIX locale did not accept y/Y affirmative response")
	}
}

// TestFindVSCBracketPatterns exercises the exact directory names, patterns,
// and printed paths from certification TP34 and TP38. POSIX equivalence-class
// and collating-symbol syntax denotes one element of the surrounding bracket
// expression; a hyphen in either edge position is literal.
func TestFindVSCBracketPatterns(t *testing.T) {
	cases := []struct {
		name, start, pattern, want string
		files                      []string
	}{
		{"equiv-C-or-b", "find_dir_35", "*[[=C=]b]*", "find_dir_35/Abc\n", []string{"Abc"}},
		{"equiv-a-anywhere", "find_dir_35", "*[[=a=]]*", "find_dir_35/a\nfind_dir_35/aBc\n", []string{"a", "aBc"}},
		{"equiv-a-whole", "find_dir_35", "[[=a=]]", "find_dir_35/a\n", []string{"a", "aBc"}},
		{"hyphen-leading", "find_dir_40", "a[-xy]b", "find_dir_40/a-b\n", []string{"a-b"}},
		{"hyphen-negated", "find_dir_40", "a[!-]b", "", []string{"a-b"}},
		{"hyphen-trailing", "find_dir_40", "a[xy-]b", "find_dir_40/a-b\n", []string{"a-b"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			for _, name := range tc.files {
				writeFile(t, dir, filepath.Join(tc.start, name), "")
			}
			out, errb, code := runFind(t, dir, tc.start, "-name", tc.pattern, "-print")
			if code != 0 || errb != "" || out != tc.want {
				t.Errorf("find %s -name %q = (%q, %q, %d), want (%q, \"\", 0)",
					tc.start, tc.pattern, out, errb, code, tc.want)
			}
		})
	}
}

// TestFindVSCLocalePrecedence maps certification TP145-149 onto the three
// locale-sensitive behaviors find exercises: LC_COLLATE equivalence classes,
// LC_CTYPE character classes, and LC_MESSAGES affirmative responses. The
// campaign locale is de_DE ISO-8859-1, so the test uses its literal ä byte.
func TestFindVSCLocalePrecedence(t *testing.T) {
	latin1Aumlaut := string([]byte{0xe4})
	cases := []struct {
		name                     string
		env                      []string
		collate, ctype, messages bool
	}{
		{"tp145-lc-all", []string{"LC_ALL=de_DE.iso88591", "LC_COLLATE=POSIX", "LC_CTYPE=POSIX", "LC_MESSAGES=POSIX", "LANG=POSIX"}, true, true, true},
		{"tp146-lc-ctype", []string{"LC_CTYPE=de_DE.iso88591", "LANG=POSIX"}, false, true, false},
		{"tp147-lc-messages", []string{"LC_MESSAGES=de_DE.iso88591", "LANG=POSIX"}, false, false, true},
		{"tp148-lc-collate", []string{"LC_COLLATE=de_DE.iso88591", "LANG=POSIX"}, true, false, false},
		{"tp149-lang", []string{"LANG=de_DE.iso88591"}, true, true, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			loc := findLocaleFromEnv(tc.env)
			if got := fnmatchLocale("[[=a=]]", latin1Aumlaut, false, loc); got != tc.collate {
				t.Errorf("LC_COLLATE match = %v, want %v", got, tc.collate)
			}
			if got := fnmatchLocale("[[:alpha:]]", latin1Aumlaut, false, loc); got != tc.ctype {
				t.Errorf("LC_CTYPE match = %v, want %v", got, tc.ctype)
			}
			w := &walker{
				rc:     &tool.RunContext{Stdio: tool.Stdio{Err: &bytes.Buffer{}}},
				stdin:  bufio.NewReader(strings.NewReader("ja\n")),
				locale: loc,
			}
			if got := w.confirm("echo", "file"); got != tc.messages {
				t.Errorf("LC_MESSAGES affirmative = %v, want %v", got, tc.messages)
			}
		})
	}
}
