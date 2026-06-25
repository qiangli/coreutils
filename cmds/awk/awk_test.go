package awkcmd

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/qiangli/coreutils/tool"
)

func runTool(t *testing.T, input string, args ...string) (stdout, stderr string, code int) {
	t.Helper()
	var out, errb bytes.Buffer
	rc := &tool.RunContext{
		Ctx:   context.Background(),
		Dir:   t.TempDir(),
		Stdio: tool.Stdio{In: strings.NewReader(input), Out: &out, Err: &errb},
	}
	code = cmd.Run(rc, args)
	return out.String(), errb.String(), code
}

func TestAwk(t *testing.T) {
	cases := []struct {
		name  string
		input string
		args  []string
		want  string
	}{
		{"field", "a b c\n", []string{`{print $2}`}, "b\n"},
		{"sum", "1\n2\n3\n", []string{`{s+=$1} END{print s}`}, "6\n"},
		{"separator", "x:y\n", []string{"-F", ":", `{print $1}`}, "x\n"},
		{"var", "", []string{"-v", "n=5", `BEGIN{print n*2}`}, "10\n"},
		{"record", "one\ntwo\nthree\n", []string{`NR==2`}, "two\n"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			out, errb, code := runTool(t, c.input, c.args...)
			if out != c.want || errb != "" || code != 0 {
				t.Errorf("awk %v = (%q, %q, %d), want (%q, %q, 0)", c.args, out, errb, code, c.want, "")
			}
		})
	}
}

func TestAwkProgramFile(t *testing.T) {
	var out, errb bytes.Buffer
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "print2.awk"), []byte("{print $2}\n"), 0o666); err != nil {
		t.Fatal(err)
	}
	rc := &tool.RunContext{
		Ctx:   context.Background(),
		Dir:   dir,
		Stdio: tool.Stdio{In: strings.NewReader("a b c\n"), Out: &out, Err: &errb},
	}
	code := cmd.Run(rc, []string{"-f", "print2.awk"})
	if out.String() != "b\n" || errb.String() != "" || code != 0 {
		t.Errorf("awk -f = (%q, %q, %d), want (%q, %q, 0)", out.String(), errb.String(), code, "b\n", "")
	}
}
