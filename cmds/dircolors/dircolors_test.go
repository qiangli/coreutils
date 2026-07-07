package dircolorscmd

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/qiangli/coreutils/tool"
)

func runTool(t *testing.T, dir string, env []string, args ...string) (stdout, stderr string, code int) {
	t.Helper()
	var out, errb bytes.Buffer
	rc := &tool.RunContext{
		Ctx:   context.Background(),
		Dir:   dir,
		Env:   env,
		Stdio: tool.Stdio{In: strings.NewReader(""), Out: &out, Err: &errb},
	}
	code = cmd.Run(rc, args)
	return out.String(), errb.String(), code
}

func TestDircolorsDefaultBourne(t *testing.T) {
	out, errb, code := runTool(t, t.TempDir(), nil)
	if code != 0 || errb != "" {
		t.Fatalf("code=%d err=%q", code, errb)
	}
	if !strings.HasPrefix(out, "LS_COLORS='") || !strings.Contains(out, "di=01;34") || !strings.Contains(out, "*.tar=01;31") || !strings.HasSuffix(out, "export LS_COLORS\n") {
		t.Fatalf("unexpected output: %q", out)
	}
}

func TestDircolorsCShellAndPrintDatabase(t *testing.T) {
	out, errb, code := runTool(t, t.TempDir(), nil, "-c")
	if code != 0 || errb != "" {
		t.Fatalf("-c code=%d err=%q", code, errb)
	}
	if !strings.HasPrefix(out, "setenv LS_COLORS '") || !strings.Contains(out, "ln=01;36") {
		t.Fatalf("unexpected c-shell output: %q", out)
	}

	out, errb, code = runTool(t, t.TempDir(), nil, "--print-database")
	if code != 0 || errb != "" {
		t.Fatalf("-p code=%d err=%q", code, errb)
	}
	if !strings.Contains(out, "DIR 01;34") || !strings.Contains(out, ".png 01;35") {
		t.Fatalf("unexpected database: %q", out)
	}
}

func TestDircolorsUutilsAliases(t *testing.T) {
	dir := t.TempDir()
	out, errb, code := runTool(t, dir, nil, "--csh")
	if code != 0 || errb != "" || !strings.HasPrefix(out, "setenv LS_COLORS '") {
		t.Fatalf("--csh: code=%d out=%q err=%q", code, out, errb)
	}
	out, errb, code = runTool(t, dir, nil, "--sh")
	if code != 0 || errb != "" || !strings.HasPrefix(out, "LS_COLORS='") {
		t.Fatalf("--sh: code=%d out=%q err=%q", code, out, errb)
	}
	out, errb, code = runTool(t, dir, nil, "--print-ls-colors")
	if code != 0 || errb != "" || !strings.Contains(out, "di=01;34") || strings.Contains(out, "LS_COLORS=") {
		t.Fatalf("--print-ls-colors: code=%d out=%q err=%q", code, out, errb)
	}
	out, _, code = runTool(t, dir, nil, "--help")
	for _, want := range []string{"--csh", "--sh", "--print-ls-colors", "-h, --help", "-V, --version"} {
		if code != 0 || !strings.Contains(out, want) {
			t.Fatalf("--help missing %q: code=%d out=%q", want, code, out)
		}
	}
}

func TestDircolorsParsesFile(t *testing.T) {
	dir := t.TempDir()
	db := strings.Join([]string{
		"# comment",
		"TERM xterm*",
		"DIR 33",
		"LINK 36",
		".go 01;32",
		"*.md 00;35",
		"",
	}, "\n")
	if err := os.WriteFile(filepath.Join(dir, "colors"), []byte(db), 0o644); err != nil {
		t.Fatal(err)
	}
	out, errb, code := runTool(t, dir, []string{"TERM=xterm-256color"}, "-b", "colors")
	if code != 0 || errb != "" {
		t.Fatalf("code=%d err=%q", code, errb)
	}
	for _, want := range []string{"di=33", "ln=36", "*.go=01;32", "*.md=00;35"} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing %q in %q", want, out)
		}
	}
}

func TestDircolorsTermMismatchProducesEmptyLSColors(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "colors"), []byte("TERM linux\nDIR 33\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	out, errb, code := runTool(t, dir, []string{"TERM=xterm"}, "colors")
	if code != 0 || errb != "" {
		t.Fatalf("code=%d err=%q", code, errb)
	}
	if out != "LS_COLORS='';\nexport LS_COLORS\n" {
		t.Fatalf("unexpected output: %q", out)
	}
}

func TestDircolorsErrorsAndHelp(t *testing.T) {
	dir := t.TempDir()
	_, errb, code := runTool(t, dir, nil, "-b", "-c")
	if code != 2 || !strings.Contains(errb, "mutually exclusive") {
		t.Fatalf("mutual exclusion: code=%d err=%q", code, errb)
	}
	_, errb, code = runTool(t, dir, nil, "a", "b")
	if code != 2 || !strings.Contains(errb, "extra operand 'b'") {
		t.Fatalf("extra operand: code=%d err=%q", code, errb)
	}
	_, errb, code = runTool(t, dir, nil, "missing")
	if code != 1 || !strings.Contains(errb, "missing") {
		t.Fatalf("missing file: code=%d err=%q", code, errb)
	}
	out, _, code := runTool(t, dir, nil, "--help")
	if code != 0 || !strings.Contains(out, "Usage: dircolors") {
		t.Fatalf("--help: code=%d out=%q", code, out)
	}
}

func TestDircolorsRejectsUnknownKeyword(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "colors"), []byte("TERM xterm*\nBOGUS 33\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, errb, code := runTool(t, dir, []string{"TERM=xterm"}, "colors")
	if code != 1 || !strings.Contains(errb, "colors:2: unrecognized keyword BOGUS") {
		t.Fatalf("code=%d err=%q", code, errb)
	}
}

func TestDircolorsRejectsMissingSecondToken(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "colors"), []byte("DIR\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, errb, code := runTool(t, dir, nil, "colors")
	if code != 1 || !strings.Contains(errb, "colors:1: invalid line; missing second token") {
		t.Fatalf("code=%d err=%q", code, errb)
	}
}

func TestDircolorsPrintDatabaseRejectsOperands(t *testing.T) {
	_, errb, code := runTool(t, t.TempDir(), nil, "-p", "somefile")
	if code != 2 || !strings.Contains(errb, "extra operand 'somefile'") {
		t.Fatalf("code=%d err=%q", code, errb)
	}
	_, errb, code = runTool(t, t.TempDir(), nil, "-p", "-b")
	if code != 2 || !strings.Contains(errb, "mutually exclusive") {
		t.Fatalf("-p -b: code=%d err=%q", code, errb)
	}
	_, errb, code = runTool(t, t.TempDir(), nil, "--print-ls-colors", "--csh")
	if code != 2 || !strings.Contains(errb, "mutually exclusive") {
		t.Fatalf("--print-ls-colors --csh: code=%d err=%q", code, errb)
	}
}

// Entries before any TERM line are global: they apply even when no
// TERM pattern matches; entries after a mismatched TERM do not.
func TestDircolorsGlobalEntriesPrecedeTermGating(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "colors"), []byte("DIR 33\nTERM linux\nLINK 36\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	out, errb, code := runTool(t, dir, []string{"TERM=xterm"}, "colors")
	if code != 0 || errb != "" {
		t.Fatalf("code=%d err=%q", code, errb)
	}
	if out != "LS_COLORS='di=33:';\nexport LS_COLORS\n" {
		t.Fatalf("unexpected output: %q", out)
	}
}

// GNU emits entries in database order without deduplication.
func TestDircolorsPreservesOrderAndDuplicates(t *testing.T) {
	dir := t.TempDir()
	db := "TERM xterm*\nLINK 36\nDIR 33\nDIR 44\n"
	if err := os.WriteFile(filepath.Join(dir, "colors"), []byte(db), 0o644); err != nil {
		t.Fatal(err)
	}
	out, errb, code := runTool(t, dir, []string{"TERM=xterm"}, "colors")
	if code != 0 || errb != "" {
		t.Fatalf("code=%d err=%q", code, errb)
	}
	if out != "LS_COLORS='ln=36:di=33:di=44:';\nexport LS_COLORS\n" {
		t.Fatalf("unexpected output: %q", out)
	}
}

func TestDircolorsColortermGating(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "colors"), []byte("COLORTERM ?*\nDIR 33\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	out, _, code := runTool(t, dir, []string{"COLORTERM=truecolor"}, "colors")
	if code != 0 || !strings.Contains(out, "di=33") {
		t.Fatalf("matched COLORTERM: code=%d out=%q", code, out)
	}
	out, _, code = runTool(t, dir, []string{"TERM=xterm"}, "colors")
	if code != 0 || strings.Contains(out, "di=33") {
		t.Fatalf("empty COLORTERM should gate entries: code=%d out=%q", code, out)
	}
}
