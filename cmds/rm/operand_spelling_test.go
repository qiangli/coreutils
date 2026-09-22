package rmcmd

// Story #682 (S245.5f), item 4: an applet's diagnostics name the operand in
// the spelling the CALLER used — bash's fixtures diff those messages. rm
// builds the name of a descendant by joining the operand with the directory
// entry, and on Windows filepath.Join would produce d\f for an operand the
// caller spelled d, in a shell whose every other path is /-spelled.

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/qiangli/coreutils/tool"
)

func TestVerboseNamesDescendantsInOperandSpelling(t *testing.T) {
	defer tool.SetOperandWindows(true)()

	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "d"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "d", "f"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	out, errb, code := runTool(t, dir, "-rv", "d")
	if code != 0 || errb != "" {
		t.Fatalf("rm -rv d: code=%d err=%q", code, errb)
	}
	want := "removed 'd/f'\nremoved directory 'd'\n"
	if out != want {
		t.Errorf("rm -rv output = %q; want %q", out, want)
	}
}
