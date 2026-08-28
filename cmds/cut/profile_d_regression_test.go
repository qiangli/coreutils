package cutcmd

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/qiangli/coreutils/tool"
)

func TestPOSIXStopsOptionParsingAtFirstOperand(t *testing.T) {
	dir := t.TempDir()
	for name, data := range map[string]string{"input": "abc\n", "-b": "def\n", "--byt": "ghi\n"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(data), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	var out, errOut bytes.Buffer
	rc := &tool.RunContext{Ctx: context.Background(), Dir: dir, Env: []string{"POSIXLY_CORRECT=1"}, Stdio: tool.Stdio{In: strings.NewReader(""), Out: &out, Err: &errOut}}
	if code := cmd.Run(rc, []string{"-b", "1", "input", "-b", "--byt"}); code != 0 || out.String() != "a\nd\ng\n" || errOut.Len() != 0 {
		t.Fatalf("cut post-operand -b = (%q, %q, %d)", out.String(), errOut.String(), code)
	}
}
