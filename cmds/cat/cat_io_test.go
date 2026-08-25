//go:build !windows

package catcmd

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/qiangli/coreutils/tool"
)

// failReader emits one good chunk and then a persistent read error,
// exercising the mid-stream input-failure branch of each catStream loop.
type failReader struct {
	chunk []byte
	done  bool
}

var errInjectedRead = errors.New("injected read failure")

func (r *failReader) Read(p []byte) (int, error) {
	if r.done {
		return 0, errInjectedRead
	}
	r.done = true
	n := copy(p, r.chunk)
	return n, nil
}

// TestCatInjectedReadErrorContinues proves that an input read failure on
// standard input is diagnosed with the operand name, sets a failing exit
// status, and does not stop later operands from being processed. Both the
// plain copy loop and the option-processing line loop are covered.
func TestCatInjectedReadErrorContinues(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "after.txt", "still catted\n")

	for _, tc := range []struct {
		name string
		args []string
	}{
		{"plain copy loop", nil},
		{"line loop with -n", []string{"-n"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var out, errb bytes.Buffer
			rc := &tool.RunContext{
				Ctx: context.Background(),
				Dir: dir,
				Stdio: tool.Stdio{
					In:  &failReader{chunk: []byte("partial line\n")},
					Out: &out,
					Err: &errb,
				},
			}
			code := cmd.Run(rc, append(tc.args, "-", "after.txt"))
			if code != 1 {
				t.Errorf("exit = %d, want 1", code)
			}
			if !strings.Contains(errb.String(), "cat: -: Injected read failure") {
				t.Errorf("stderr = %q, want read-error diagnostic naming -", errb.String())
			}
			if !strings.Contains(out.String(), "partial line\n") ||
				!strings.Contains(out.String(), "still catted\n") {
				t.Errorf("stdout = %q, want bytes read before the failure and the later operand", out.String())
			}
		})
	}
}

// TestCatSpecialFileOperands proves that any input file type is accepted:
// directories and dangling symlinks fail only their own operand with the
// required diagnostic and status, and character devices like /dev/null read
// to EOF successfully, including interleaved with regular files.
func TestCatSpecialFileOperands(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "plain.txt", "plain\n")
	if err := os.Mkdir(filepath.Join(dir, "subdir"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(dir, "subdir"), filepath.Join(dir, "dirlink")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(dir, "missing"), filepath.Join(dir, "dangling")); err != nil {
		t.Fatal(err)
	}

	// A directory operand is a mid-stream read failure on a successfully
	// opened descriptor: diagnosis, status 1, and continuation.
	out, errb, code := runTool(t, dir, "", "subdir", "plain.txt")
	if code != 1 || !strings.Contains(errb, "cat: subdir: Is a directory") {
		t.Errorf("directory operand = (%q, %q, %d), want diagnostic and status 1", out, errb, code)
	}
	if out != "plain\n" {
		t.Errorf("directory operand output = %q, want later operand catted", out)
	}

	// A symlink to a directory follows the same path.
	_, errb, code = runTool(t, dir, "", "dirlink")
	if code != 1 || !strings.Contains(errb, "cat: dirlink: Is a directory") {
		t.Errorf("symlinked directory = (%q, %d), want diagnostic and status 1", errb, code)
	}

	// A dangling symlink fails to open and continues.
	_, errb, code = runTool(t, dir, "", "dangling", "plain.txt")
	if code != 1 || !strings.Contains(errb, "cat: dangling:") {
		t.Errorf("dangling symlink = (%q, %d), want open diagnostic and status 1", errb, code)
	}

	// /dev/null reads to EOF with no output and success status.
	out, errb, code = runTool(t, "", "", "/dev/null")
	if code != 0 || out != "" || errb != "" {
		t.Errorf("/dev/null = (%q, %q, %d), want silent success", out, errb, code)
	}

	// /dev/null interleaved with regular files is byte-transparent.
	out, _, code = runTool(t, dir, "", "plain.txt", "/dev/null", "plain.txt")
	if code != 0 || out != "plain\nplain\n" {
		t.Errorf("/dev/null interleaved = (%q, %d), want both operands catted", out, code)
	}
}
