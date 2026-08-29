package uuencodecmd

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/qiangli/coreutils/tool"
)

func runTool(t *testing.T, dir, stdin string, args ...string) (string, string, int) {
	return runToolEnv(t, dir, stdin, nil, args...)
}

func runToolEnv(t *testing.T, dir, stdin string, env []string, args ...string) (string, string, int) {
	t.Helper()
	var out, err bytes.Buffer
	rc := &tool.RunContext{Ctx: context.Background(), Dir: dir, Env: env, Stdio: tool.Stdio{In: strings.NewReader(stdin), Out: &out, Err: &err}}
	code := cmd.Run(rc, args)
	return out.String(), err.String(), code
}

func TestPOSIXOptionsEndAtFirstOperand(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "input"), []byte("Cat"), 0o640); err != nil {
		t.Fatal(err)
	}
	for _, remote := range []string{"-m", "--"} {
		out, errb, code := runToolEnv(t, dir, "", []string{"POSIXLY_CORRECT=1"}, "input", remote)
		if code != 0 || errb != "" || !strings.HasPrefix(out, "begin 640 "+remote+"\n") {
			t.Fatalf("post-operand %q: code=%d stdout=%q stderr=%q", remote, code, out, errb)
		}
	}
}

func TestEncodeKnownVectorFromStdin(t *testing.T) {
	out, err, code := runTool(t, t.TempDir(), "Cat", "cat.txt")
	if code != 0 || err != "" || out != "begin 666 cat.txt\n#0V%T\n \nend\n" {
		t.Fatalf("got (%q, %q, %d)", out, err, code)
	}
}

func TestClassicUsesSpacesForZeroSextets(t *testing.T) {
	out, errb, code := runTool(t, t.TempDir(), "\x00\x00\x00", "zeros")
	if code != 0 || errb != "" || out != "begin 666 zeros\n#    \n \nend\n" {
		t.Fatalf("got (%q, %q, %d)", out, errb, code)
	}
}

func TestEncodeFileAndMode(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "in")
	if err := os.WriteFile(path, []byte("hello\n"), 0o640); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0o640); err != nil {
		t.Fatal(err)
	}
	out, errb, code := runTool(t, dir, "ignored", "in", "remote name")
	if code != 0 || errb != "" || !strings.HasPrefix(out, "begin 640 remote name\n&:&5L;&\\*\n") {
		t.Fatalf("got (%q, %q, %d)", out, errb, code)
	}
}

func TestEncodeBase64AndModes(t *testing.T) {
	data := strings.Repeat("x", 58)
	out, errb, code := runTool(t, t.TempDir(), data, "-m", "remote")
	lines := strings.Split(strings.TrimSuffix(out, "\n"), "\n")
	if code != 0 || errb != "" || lines[0] != "begin-base64 666 remote" || len(lines[1]) != 76 || lines[3] != "====" {
		t.Fatalf("got (%q, %q, %d)", out, errb, code)
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "-"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	out, errb, code = runTool(t, dir, "ignored", "-", "remote")
	if code != 0 || errb != "" || !strings.HasPrefix(out, "begin 600 remote\n") {
		t.Fatalf("dash input got (%q,%q,%d)", out, errb, code)
	}
}

func TestStandardInputFileModeComesFromFstat(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "stdin")
	if err := os.WriteFile(path, []byte("Cat"), 0o751); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0o751); err != nil {
		t.Fatal(err)
	}
	input, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer input.Close()
	info, err := input.Stat()
	if err != nil {
		t.Fatal(err)
	}
	var out, errb bytes.Buffer
	rc := &tool.RunContext{Ctx: context.Background(), Dir: dir, Stdio: tool.Stdio{In: input, Out: &out, Err: &errb}}
	code := cmd.Run(rc, []string{"remote"})
	wantHeader := "begin " + fmt.Sprintf("%03o", info.Mode().Perm()) + " remote\n"
	if code != 0 || errb.String() != "" || !strings.HasPrefix(out.String(), wantHeader) {
		t.Fatalf("got (%q, %q, %d), want header %q", out.String(), errb.String(), code, wantHeader)
	}
}

func TestExplicitInputFileDoesNotInspectStandardInput(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "input")
	if err := os.WriteFile(path, []byte("Cat"), 0o640); err != nil {
		t.Fatal(err)
	}
	closed, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := closed.Close(); err != nil {
		t.Fatal(err)
	}
	var out, errb bytes.Buffer
	rc := &tool.RunContext{Ctx: context.Background(), Dir: dir, Stdio: tool.Stdio{In: closed, Out: &out, Err: &errb}}
	code := cmd.Run(rc, []string{"input", "remote"})
	if code != 0 || errb.String() != "" || !strings.HasPrefix(out.String(), "begin 640 remote\n") {
		t.Fatalf("got (%q, %q, %d)", out.String(), errb.String(), code)
	}
}

func TestErrors(t *testing.T) {
	for _, tc := range [][]string{{}, {"a", "b", "c"}, {"missing", "name"}} {
		_, err, code := runTool(t, t.TempDir(), "", tc...)
		if code == 0 || err == "" {
			t.Errorf("args %v: err=%q code=%d", tc, err, code)
		}
	}
}
