package uudecodecmd

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/qiangli/coreutils/tool"
)

func runTool(t *testing.T, dir, stdin string, args ...string) (string, string, int) {
	t.Helper()
	var out, err bytes.Buffer
	rc := &tool.RunContext{Ctx: context.Background(), Dir: dir, Stdio: tool.Stdio{In: strings.NewReader(stdin), Out: &out, Err: &err}}
	code := cmd.Run(rc, args)
	return out.String(), err.String(), code
}

const catFixture = "noise before header\nbegin 640 cat.txt\n#0V%T\n`\nend\n"

func TestDecodeHeaderOutputAndMode(t *testing.T) {
	dir := t.TempDir()
	_, errb, code := runTool(t, dir, catFixture)
	if code != 0 || errb != "" {
		t.Fatalf("err=%q code=%d", errb, code)
	}
	b, err := os.ReadFile(filepath.Join(dir, "cat.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(b) != "Cat" {
		t.Fatalf("content %q", b)
	}
	if info, err := os.Stat(filepath.Join(dir, "cat.txt")); err != nil || info.Mode().Perm() != 0o640 {
		t.Fatalf("mode=%v err=%v", info.Mode().Perm(), err)
	}
}

func TestDecodeToStdoutAndFileOperand(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "in.uue"), []byte(catFixture), 0o600); err != nil {
		t.Fatal(err)
	}
	out, errb, code := runTool(t, dir, "ignored", "-o", "-", "in.uue")
	if out != "Cat" || errb != "" || code != 0 {
		t.Fatalf("got (%q,%q,%d)", out, errb, code)
	}
}

func TestUnsafeHeaderNamesRejectedUnlessOverridden(t *testing.T) {
	for _, name := range []string{"../escape", "/tmp/escape", `dir\\escape`} {
		dir := t.TempDir()
		fixture := "begin 600 " + name + "\n`\nend\n"
		_, errb, code := runTool(t, dir, fixture)
		if code != 1 || !strings.Contains(errb, "unsafe output name") {
			t.Errorf("name=%q err=%q code=%d", name, errb, code)
		}
	}
	dir := t.TempDir()
	_, errb, code := runTool(t, dir, "begin 600 ../escape\n#0V%T\n`\nend\n", "-o", "safe")
	if code != 0 || errb != "" {
		t.Fatalf("override err=%q code=%d", errb, code)
	}
	if b, err := os.ReadFile(filepath.Join(dir, "safe")); err != nil || string(b) != "Cat" {
		t.Fatalf("safe=%q err=%v", b, err)
	}
}

func TestMalformedAndUnsupportedInputs(t *testing.T) {
	for _, in := range []string{"", "begin-base64 600 x\nQ2F0====\n====\n", "begin nope x\n`\nend\n", "begin 600 x\n#0V\n`\nend\n", "begin 600 x\n#0~%T\n`\nend\n", "begin 600 x\n#0V%T\n"} {
		_, errb, code := runTool(t, t.TempDir(), in, "-o", "-")
		if code == 0 || errb == "" {
			t.Errorf("input=%q err=%q code=%d", in, errb, code)
		}
	}
}
