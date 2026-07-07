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
