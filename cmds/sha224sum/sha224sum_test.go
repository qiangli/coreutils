package sha224sumcmd

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/qiangli/coreutils/tool"
)

// sha224("abc") — FIPS 180 test vector.
const abcDigest = "23097d223405d8228642a477bda255b32aadbce4bda0b3f7e36c9da7"

// runTool is the canonical test harness shape for cmds packages,
// extended with stdin content and an explicit working directory.
func runTool(t *testing.T, dir, stdin string, args ...string) (stdout, stderr string, code int) {
	t.Helper()
	if dir == "" {
		dir = t.TempDir()
	}
	var out, errb bytes.Buffer
	rc := &tool.RunContext{
		Ctx:   context.Background(),
		Dir:   dir,
		Stdio: tool.Stdio{In: strings.NewReader(stdin), Out: &out, Err: &errb},
	}
	code = cmd.Run(rc, args)
	return out.String(), errb.String(), code
}

func TestCompute(t *testing.T) {
	out, _, code := runTool(t, "", "abc")
	if out != abcDigest+"  -\n" || code != 0 {
		t.Errorf("stdin: got (%q, %d)", out, code)
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "f.txt"), []byte("abc"), 0o644); err != nil {
		t.Fatal(err)
	}
	out, _, code = runTool(t, dir, "", "f.txt")
	if out != abcDigest+"  f.txt\n" || code != 0 {
		t.Errorf("file: got (%q, %d)", out, code)
	}
	out, _, code = runTool(t, dir, "", "-b", "f.txt")
	if out != abcDigest+" *f.txt\n" || code != 0 {
		t.Errorf("-b: got (%q, %d)", out, code)
	}
	out, _, code = runTool(t, dir, "", "--tag", "f.txt")
	if out != "SHA224 (f.txt) = "+abcDigest+"\n" || code != 0 {
		t.Errorf("--tag: got (%q, %d)", out, code)
	}
}

func TestCheck(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "f.txt"), []byte("abc"), 0o644); err != nil {
		t.Fatal(err)
	}
	sums := abcDigest + "  f.txt\n" + "SHA224 (f.txt) = " + abcDigest + "\n"
	if err := os.WriteFile(filepath.Join(dir, "sums.txt"), []byte(sums), 0o644); err != nil {
		t.Fatal(err)
	}
	out, errb, code := runTool(t, dir, "", "-c", "sums.txt")
	if out != "f.txt: OK\nf.txt: OK\n" || errb != "" || code != 0 {
		t.Errorf("-c ok: out=%q err=%q code=%d", out, errb, code)
	}
	// mismatch fails with the GNU warning
	bad := strings.Repeat("0", len(abcDigest)) + "  f.txt\n"
	if err := os.WriteFile(filepath.Join(dir, "bad.txt"), []byte(bad), 0o644); err != nil {
		t.Fatal(err)
	}
	out, errb, code = runTool(t, dir, "", "-c", "bad.txt")
	if out != "f.txt: FAILED\n" || code != 1 ||
		!strings.Contains(errb, "sha224sum: WARNING: 1 computed checksum did NOT match") {
		t.Errorf("-c failed: out=%q err=%q code=%d", out, errb, code)
	}
}

func TestErrors(t *testing.T) {
	_, errb, code := runTool(t, "", "", "nope.txt")
	if code != 1 || !strings.Contains(errb, "sha224sum: nope.txt: No such file or directory") {
		t.Errorf("missing operand file: err=%q code=%d", errb, code)
	}
	_, errb, code = runTool(t, "", "", "--tag", "-c", "x")
	if code != 2 || !strings.Contains(errb, "meaningless when verifying checksums") {
		t.Errorf("--tag -c: err=%q code=%d", errb, code)
	}
	_, errb, code = runTool(t, "", "", "--frobnicate")
	if code != 2 || !strings.Contains(errb, "frobnicate") || !strings.Contains(errb, "pure-Go") {
		t.Errorf("unknown flag: err=%q code=%d", errb, code)
	}
}

func TestHelpAndVersion(t *testing.T) {
	out, _, code := runTool(t, "", "", "--help")
	if code != 0 || !strings.Contains(out, "Usage: sha224sum") {
		t.Errorf("--help: code=%d out=%q", code, out)
	}
	out, _, code = runTool(t, "", "", "--version")
	if code != 0 || !strings.Contains(out, "sha224sum") {
		t.Errorf("--version: code=%d out=%q", code, out)
	}
}
