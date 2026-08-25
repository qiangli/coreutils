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

const catFixture = "noise before header\nbegin 640 cat.txt\n#0V%T\n \nend\n"

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

func TestHeaderPathnamesAndSymbolicModes(t *testing.T) {
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "nested"), 0o755); err != nil {
		t.Fatal(err)
	}
	_, errb, code := runTool(t, dir, "begin u=rw,g=r,o= nested/safe\n#0V%T\n \nend\n")
	if code != 0 || errb != "" {
		t.Fatalf("err=%q code=%d", errb, code)
	}
	path := filepath.Join(dir, "nested", "safe")
	if b, err := os.ReadFile(path); err != nil || string(b) != "Cat" {
		t.Fatalf("safe=%q err=%v", b, err)
	}
	if info, err := os.Stat(path); err != nil || info.Mode().Perm() != 0o640 {
		t.Fatalf("mode=%v err=%v", info.Mode().Perm(), err)
	}
	// An absolute decode pathname is likewise passed to normal pathname handling.
	abs := filepath.Join(dir, "absolute")
	_, errb, code = runTool(t, dir, "begin 600 "+abs+"\n \nend\n")
	if code != 0 || errb != "" {
		t.Fatalf("absolute err=%q code=%d", errb, code)
	}
	if _, err := os.Stat(abs); err != nil {
		t.Fatal(err)
	}
}

func TestMalformedAndUnsupportedInputs(t *testing.T) {
	for _, in := range []string{"", "begin nope x\n \nend\n", "begin 600 x\n#0V\n \nend\n", "begin 600 x\n#0~%T\n \nend\n", "begin 600 x\n#0V%T\n"} {
		_, errb, code := runTool(t, t.TempDir(), in, "-o", "-")
		if code == 0 || errb == "" {
			t.Errorf("input=%q err=%q code=%d", in, errb, code)
		}
	}
}

func TestClassicBacktickExtensionIsAccepted(t *testing.T) {
	out, errb, code := runTool(t, t.TempDir(), "begin 600 ignored\n#0V%T\n`\nend\n", "-o", "-")
	if code != 0 || errb != "" || out != "Cat" {
		t.Fatalf("got (%q,%q,%d)", out, errb, code)
	}
}

func TestDecodeBase64AndHeaderScanning(t *testing.T) {
	dir := t.TempDir()
	input := "preamble\nbegin 640 first\n#0V%T\n \nend\ntrailer\nbegin-base64 600 second\nRG9uZQ==\n====\n"
	_, errb, code := runTool(t, dir, input)
	if code != 0 || errb != "" {
		t.Fatalf("err=%q code=%d", errb, code)
	}
	for name, want := range map[string]string{"first": "Cat", "second": "Done"} {
		got, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil || string(got) != want {
			t.Errorf("%s = %q, %v; want %q", name, got, err, want)
		}
	}
}

func TestDecodeMultipleInputFilesAndOutputConflict(t *testing.T) {
	dir := t.TempDir()
	for name, data := range map[string]string{
		"one.uue": "begin 600 one\n#0V%T\n \nend\n",
		"two.uue": "begin-base64 600 two\nRG9uZQ==\n====\n",
	} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(data), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	_, errb, code := runTool(t, dir, "", "one.uue", "two.uue")
	if code != 0 || errb != "" {
		t.Fatalf("err=%q code=%d", errb, code)
	}
	_, errb, code = runTool(t, dir, "", "-o", "result", "one.uue", "two.uue")
	if code != 2 || !strings.Contains(errb, "output-file") {
		t.Fatalf("-o with multiple inputs: err=%q code=%d", errb, code)
	}
}

func TestDecodeOutputLifecycleAndMode(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "out")
	if err := os.WriteFile(path, []byte("keep"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, errb, code := runTool(t, dir, "begin-base64 777 out\nRG9uZ\n====\n")
	if code == 0 || errb == "" {
		t.Fatalf("malformed input: err=%q code=%d", errb, code)
	}
	if got, err := os.ReadFile(path); err != nil || string(got) != "keep" {
		t.Fatalf("malformed decode changed target: %q, %v", got, err)
	}

	_, errb, code = runTool(t, dir, "begin-base64 777 out\nRG9uZQ==\n====\n")
	if code != 0 || errb != "" {
		t.Fatalf("err=%q code=%d", errb, code)
	}
	if info, err := os.Stat(path); err != nil || info.Mode().Perm() != 0o777 {
		t.Fatalf("mode=%v err=%v", info.Mode().Perm(), err)
	}
}

func TestDecodeBase64MalformedDataRules(t *testing.T) {
	dir := t.TempDir()
	// POSIX requires characters outside the base64 alphabet to be ignored.
	_, errb, code := runTool(t, dir, "begin-base64 600 filtered\nQ2?F0\n====\n")
	if code != 0 || errb != "" {
		t.Fatalf("filtered base64: err=%q code=%d", errb, code)
	}
	if got, err := os.ReadFile(filepath.Join(dir, "filtered")); err != nil || string(got) != "Cat" {
		t.Fatalf("filtered = %q, %v", got, err)
	}
	for _, input := range []string{
		"begin-base64 600 bad\nQ2F0====\n====\n", // padding is only valid in the final quantum
		"begin-base64 600 bad\nQ2F0\n",           // the framing terminator is required
	} {
		path := filepath.Join(dir, "bad")
		if err := os.WriteFile(path, []byte("keep"), 0o600); err != nil {
			t.Fatal(err)
		}
		_, errb, code := runTool(t, dir, input)
		if code == 0 || errb == "" {
			t.Errorf("input=%q err=%q code=%d", input, errb, code)
		}
		if got, err := os.ReadFile(path); err != nil || string(got) != "keep" {
			t.Errorf("bad target changed to %q, %v", got, err)
		}
	}
}

func TestHeaderStdoutNames(t *testing.T) {
	for _, name := range []string{"-", "/dev/stdout"} {
		out, errb, code := runTool(t, t.TempDir(), "begin-base64 600 "+name+"\nQ2F0\n====\n")
		if code != 0 || errb != "" || out != "Cat" {
			t.Errorf("name=%q got (%q,%q,%d)", name, out, errb, code)
		}
	}
}
