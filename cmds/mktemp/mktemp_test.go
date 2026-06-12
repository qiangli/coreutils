package mktempcmd

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"

	"github.com/qiangli/coreutils/tool"
)

// runTool is the canonical test harness shape for cmds packages,
// extended with env because mktemp honors $TMPDIR from rc.Env.
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

func TestMktempDefaultTemplate(t *testing.T) {
	tmp := t.TempDir()
	out, errb, code := runTool(t, t.TempDir(), []string{"TMPDIR=" + tmp})
	if code != 0 || errb != "" {
		t.Fatalf("mktemp: code=%d err=%q", code, errb)
	}
	name := strings.TrimSuffix(out, "\n")
	if filepath.Dir(name) != tmp {
		t.Errorf("created outside $TMPDIR: %q", name)
	}
	base := filepath.Base(name)
	if !regexp.MustCompile(`^tmp\.[0-9A-Za-z]{10}$`).MatchString(base) {
		t.Errorf("name %q does not match default template", base)
	}
	fi, err := os.Stat(name)
	if err != nil {
		t.Fatalf("file not created: %v", err)
	}
	if runtime.GOOS != "windows" && fi.Mode().Perm() != 0o600 {
		t.Errorf("file mode = %v, want 0600", fi.Mode().Perm())
	}
}

func TestMktempTemplate(t *testing.T) {
	dir := t.TempDir()
	out, errb, code := runTool(t, dir, nil, "fooXXXX")
	if code != 0 || errb != "" {
		t.Fatalf("mktemp fooXXXX: code=%d err=%q", code, errb)
	}
	name := strings.TrimSuffix(out, "\n")
	if !regexp.MustCompile(`^foo[0-9A-Za-z]{4}$`).MatchString(name) {
		t.Errorf("name %q does not match template", name)
	}
	if _, err := os.Stat(filepath.Join(dir, name)); err != nil {
		t.Errorf("not created in invocation dir: %v", err)
	}
}

func TestMktempImpliedSuffix(t *testing.T) {
	dir := t.TempDir()
	out, _, code := runTool(t, dir, nil, "fooXXXXbar")
	if code != 0 {
		t.Fatalf("mktemp fooXXXXbar: code=%d", code)
	}
	name := strings.TrimSuffix(out, "\n")
	if !regexp.MustCompile(`^foo[0-9A-Za-z]{4}bar$`).MatchString(name) {
		t.Errorf("name %q does not keep the implied suffix", name)
	}
}

func TestMktempDirectory(t *testing.T) {
	dir := t.TempDir()
	out, _, code := runTool(t, dir, nil, "-d", "dXXXX")
	if code != 0 {
		t.Fatalf("mktemp -d: code=%d", code)
	}
	name := strings.TrimSuffix(out, "\n")
	fi, err := os.Stat(filepath.Join(dir, name))
	if err != nil || !fi.IsDir() {
		t.Errorf("directory not created: fi=%v err=%v", fi, err)
	}
	if runtime.GOOS != "windows" && fi.Mode().Perm() != 0o700 {
		t.Errorf("dir mode = %v, want 0700", fi.Mode().Perm())
	}
}

func TestMktempDryRun(t *testing.T) {
	dir := t.TempDir()
	out, _, code := runTool(t, dir, nil, "-u", "fooXXXX")
	if code != 0 || out == "" {
		t.Fatalf("mktemp -u: code=%d out=%q", code, out)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Errorf("-u created something: %v", entries)
	}
}

func TestMktempTmpdirFlag(t *testing.T) {
	dir := t.TempDir()
	sub := filepath.Join(dir, "sub")
	if err := os.Mkdir(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	out, _, code := runTool(t, dir, nil, "-p", "sub", "fooXXXX")
	if code != 0 {
		t.Fatalf("mktemp -p: code=%d", code)
	}
	name := strings.TrimSuffix(out, "\n")
	if filepath.Dir(name) != "sub" {
		t.Errorf("printed name %q not under sub", name)
	}
	if _, err := os.Stat(filepath.Join(dir, name)); err != nil {
		t.Errorf("not created under -p dir: %v", err)
	}
}

func TestMktempErrors(t *testing.T) {
	dir := t.TempDir()
	_, errb, code := runTool(t, dir, nil, "fooXX")
	if code != 1 || !strings.Contains(errb, "too few X's") {
		t.Errorf("too few X's: code=%d err=%q", code, errb)
	}
	abs := filepath.Join(dir, "absXXXX")
	_, errb, code = runTool(t, dir, nil, "-p", dir, abs)
	if code != 1 || !strings.Contains(errb, "may not be absolute") {
		t.Errorf("absolute with -p: code=%d err=%q", code, errb)
	}
	_, errb, code = runTool(t, dir, nil, "aXXXX", "bXXXX")
	if code != 2 || !strings.Contains(errb, "too many templates") {
		t.Errorf("two templates: code=%d err=%q", code, errb)
	}
	_, errb, code = runTool(t, dir, nil, "--frobnicate")
	if code != 2 || !strings.Contains(errb, "frobnicate") || !strings.Contains(errb, "pure-Go") {
		t.Errorf("unknown flag: code=%d err=%q", code, errb)
	}
}

func TestMktempHelpAndVersion(t *testing.T) {
	out, _, code := runTool(t, t.TempDir(), nil, "--help")
	if code != 0 || !strings.Contains(out, "Usage: mktemp") {
		t.Errorf("--help: code=%d out=%q", code, out)
	}
	out, _, code = runTool(t, t.TempDir(), nil, "--version")
	if code != 0 || !strings.Contains(out, "mktemp") {
		t.Errorf("--version: code=%d out=%q", code, out)
	}
}
