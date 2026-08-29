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

func TestPOSIXInputPathResolution(t *testing.T) {
	root := t.TempDir()
	work := filepath.Join(root, "work")
	if err := os.Mkdir(work, 0o755); err != nil {
		t.Fatal(err)
	}
	input := filepath.Join(root, "input")
	if err := os.WriteFile(input, []byte("hello\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	for _, tc := range []struct {
		name    string
		operand string
	}{
		{name: "absolute", operand: input},
		{name: "parent", operand: filepath.Join("..", "input")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var out, errOut bytes.Buffer
			rc := &tool.RunContext{
				Ctx: context.Background(), Dir: work,
				Env:   []string{"POSIXLY_CORRECT=1"},
				Stdio: tool.Stdio{In: strings.NewReader(""), Out: &out, Err: &errOut},
			}
			want := "6 " + tc.operand + "\n"
			code := cmd.Run(rc, []string{"-c", tc.operand})
			if code != 0 || errOut.Len() != 0 || out.String() != want {
				t.Fatalf("wc %s input = (stdout %q, stderr %q, status %d), want %q", tc.name, out.String(), errOut.String(), code, want)
			}
		})
	}
}
