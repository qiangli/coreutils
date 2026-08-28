package pastecmd

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPOSIXStopsOptionParsingAtFirstOperand(t *testing.T) {
	dir := writeFiles(t, map[string]string{"input": "name\nrole\n", "-s": "alice\nadmin\n", "--ser": "lead\nstaff\n"})
	out, errOut, code := runToolDirEnv(t, dir, []string{"POSIXLY_CORRECT=1"}, "", "input", "-s", "--ser")
	if code != 0 || out != "name\talice\tlead\nrole\tadmin\tstaff\n" || errOut != "" {
		t.Fatalf("paste post-operand -s = (%q, %q, %d)", out, errOut, code)
	}
}

// TestPasteResolvesAbsoluteAndParentOperands is a public pathname reducer:
// correctness is established by the bytes read, rather than by observing an
// access-time change (which is not guaranteed on relatime filesystems).
func TestPasteResolvesAbsoluteAndParentOperands(t *testing.T) {
	root := writeFiles(t, map[string]string{"input": "alpha\nbeta\n"})
	work := filepath.Join(root, "work")
	if err := os.Mkdir(work, 0o755); err != nil {
		t.Fatal(err)
	}

	for _, tc := range []struct {
		name    string
		dir     string
		operand string
	}{
		{name: "absolute", dir: root, operand: filepath.Join(root, "input")},
		{name: "parent", dir: work, operand: filepath.Join("..", "input")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, errOut, code := runToolDir(t, tc.dir, "", "-s", tc.operand)
			if code != 0 || out != "alpha\tbeta\n" || errOut != "" {
				t.Fatalf("paste -s %q = (%q, %q, %d)", tc.operand, out, errOut, code)
			}
		})
	}
}
