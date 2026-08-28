package wccmd

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
	for name, data := range map[string]string{"input": "hello\n", "-w": "two words\n", "--wor": "abc\n"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(data), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	var out, errOut bytes.Buffer
	rc := &tool.RunContext{Ctx: context.Background(), Dir: dir, Env: []string{"POSIXLY_CORRECT=1"}, Stdio: tool.Stdio{In: strings.NewReader(""), Out: &out, Err: &errOut}}
	if code := cmd.Run(rc, []string{"-c", "input", "-w", "--wor"}); code != 0 || out.String() != " 6 input\n10 -w\n 4 --wor\n20 total\n" || errOut.Len() != 0 {
		t.Fatalf("wc post-operand -w = (%q, %q, %d)", out.String(), errOut.String(), code)
	}
}
