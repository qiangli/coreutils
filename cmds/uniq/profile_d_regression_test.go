package uniqcmd

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/qiangli/coreutils/tool"
)

func TestPOSIXStopsOptionAndLegacyParsingAtFirstOperand(t *testing.T) {
	for _, output := range []string{"-f", "+2", "--skip-fie"} {
		t.Run(output, func(t *testing.T) {
			dir := t.TempDir()
			if err := os.WriteFile(filepath.Join(dir, "input"), []byte("a\na\nb\n"), 0o644); err != nil {
				t.Fatal(err)
			}
			var out, errOut bytes.Buffer
			rc := &tool.RunContext{Ctx: context.Background(), Dir: dir, Env: []string{"POSIXLY_CORRECT=1"}, Stdio: tool.Stdio{In: strings.NewReader(""), Out: &out, Err: &errOut}}
			code := cmd.Run(rc, []string{"input", output})
			if code != 0 || errOut.Len() != 0 {
				t.Fatalf("uniq output %q = (%q, %d)", output, errOut.String(), code)
			}
			got, err := os.ReadFile(filepath.Join(dir, output))
			if err != nil || string(got) != "a\nb\n" {
				t.Fatalf("uniq output %q contents = (%q, %v)", output, got, err)
			}
		})
	}
}

func TestPOSIXInputPathResolution(t *testing.T) {
	root := t.TempDir()
	work := filepath.Join(root, "work")
	if err := os.Mkdir(work, 0o755); err != nil {
		t.Fatal(err)
	}
	input := filepath.Join(root, "input")
	if err := os.WriteFile(input, []byte("a\na\nb\n"), 0o644); err != nil {
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
			code := cmd.Run(rc, []string{tc.operand})
			if code != 0 || errOut.Len() != 0 || out.String() != "a\nb\n" {
				t.Fatalf("uniq %s input = (stdout %q, stderr %q, status %d)", tc.name, out.String(), errOut.String(), code)
			}
		})
	}
}
