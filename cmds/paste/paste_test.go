package pastecmd

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/qiangli/coreutils/tool"
)

func runToolDir(t *testing.T, dir, stdin string, args ...string) (stdout, stderr string, code int) {
	t.Helper()
	var out, errb bytes.Buffer
	rc := &tool.RunContext{
		Ctx:   context.Background(),
		Dir:   dir,
		Stdio: tool.Stdio{In: strings.NewReader(stdin), Out: &out, Err: &errb},
	}
	code = cmd.Run(rc, args)
	return out.String(), errb.String(), code
}

func writeFiles(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func TestPasteParallel(t *testing.T) {
	cases := []struct {
		name  string
		files map[string]string
		args  []string
		want  string
	}{
		{
			"two files default tab",
			map[string]string{"f1": "a\nb\n", "f2": "1\n2\n"},
			[]string{"f1", "f2"},
			"a\t1\nb\t2\n",
		},
		{
			"first file longer keeps trailing delimiter",
			map[string]string{"f1": "a\nb\n", "f2": "1\n"},
			[]string{"f1", "f2"},
			"a\t1\nb\t\n",
		},
		{
			"first file shorter keeps leading delimiter",
			map[string]string{"f1": "a\n", "f2": "1\n2\n"},
			[]string{"f1", "f2"},
			"a\t1\n\t2\n",
		},
		{
			"delimiter list cycles and resets per line",
			map[string]string{"f1": "a\nA\n", "f2": "b\nB\n", "f3": "c\nC\n"},
			[]string{"-d", ",;", "f1", "f2", "f3"},
			"a,b;c\nA,B;C\n",
		},
		{
			"backslash-zero is no delimiter",
			map[string]string{"f1": "a\n", "f2": "b\n"},
			[]string{"-d", "\\0", "f1", "f2"},
			"ab\n",
		},
		{
			"escaped tab and newline delimiters",
			map[string]string{"f1": "a\n", "f2": "b\n", "f3": "c\n"},
			[]string{"-d", "\\n\\t", "f1", "f2", "f3"},
			"a\nb\tc\n",
		},
		{
			"single file passthrough adds final newline",
			map[string]string{"f1": "a\nb"},
			[]string{"f1"},
			"a\nb\n",
		},
		{
			"empty files produce nothing",
			map[string]string{"f1": "", "f2": ""},
			[]string{"f1", "f2"},
			"",
		},
		{
			"missing final newline still pastes",
			map[string]string{"f1": "a\nb", "f2": "1\n2"},
			[]string{"f1", "f2"},
			"a\t1\nb\t2\n",
		},
	}
	for _, c := range cases {
		dir := writeFiles(t, c.files)
		out, errb, code := runToolDir(t, dir, "", c.args...)
		if out != c.want || code != 0 {
			t.Errorf("%s: paste %v = (%q, %q, %d), want (%q, _, 0)", c.name, c.args, out, errb, code, c.want)
		}
	}
}

func TestPasteSerial(t *testing.T) {
	dir := writeFiles(t, map[string]string{
		"f1":    "a\nb\nc\n",
		"f2":    "1\n2\n",
		"empty": "",
	})
	out, _, code := runToolDir(t, dir, "", "-s", "f1", "f2")
	if out != "a\tb\tc\n1\t2\n" || code != 0 {
		t.Errorf("serial: out=%q code=%d", out, code)
	}
	// delimiter cycle restarts for each file
	out, _, code = runToolDir(t, dir, "", "-s", "-d", ",;", "f1", "f2")
	if out != "a,b;c\n1,2\n" || code != 0 {
		t.Errorf("serial cycle: out=%q code=%d", out, code)
	}
	// an empty file yields a bare line terminator
	out, _, code = runToolDir(t, dir, "", "-s", "empty")
	if out != "\n" || code != 0 {
		t.Errorf("serial empty: out=%q code=%d", out, code)
	}
}

func TestPasteStdin(t *testing.T) {
	dir := writeFiles(t, map[string]string{"f1": "x\ny\n"})
	// no operands: read stdin
	out, _, code := runToolDir(t, dir, "a\nb\n")
	if out != "a\nb\n" || code != 0 {
		t.Errorf("stdin default: out=%q code=%d", out, code)
	}
	// "-" mixed with a file
	out, _, code = runToolDir(t, dir, "a\nb\n", "-", "f1")
	if out != "a\tx\nb\ty\n" || code != 0 {
		t.Errorf("dash + file: out=%q code=%d", out, code)
	}
	// two "-" operands interleave lines from the same stdin
	out, _, code = runToolDir(t, dir, "1\n2\n3\n4\n", "-", "-")
	if out != "1\t2\n3\t4\n" || code != 0 {
		t.Errorf("dash dash: out=%q code=%d", out, code)
	}
	// serial stdin
	out, _, code = runToolDir(t, dir, "1\n2\n3\n", "-s")
	if out != "1\t2\t3\n" || code != 0 {
		t.Errorf("serial stdin: out=%q code=%d", out, code)
	}
}

func TestPasteZeroTerminated(t *testing.T) {
	dir := writeFiles(t, map[string]string{"f1": "a\x00b\x00", "f2": "1\x002\x00"})
	out, _, code := runToolDir(t, dir, "", "-z", "f1", "f2")
	if code != 0 || out != "a\t1\x00b\t2\x00" {
		t.Errorf("parallel -z: out=%q code=%d", out, code)
	}
	out, _, code = runToolDir(t, dir, "", "-z", "-s", "-d", ",", "f1")
	if code != 0 || out != "a,b\x00" {
		t.Errorf("serial -z: out=%q code=%d", out, code)
	}
}

func TestPasteErrors(t *testing.T) {
	dir := writeFiles(t, map[string]string{"f1": "a\n"})
	// trailing unescaped backslash in -d
	_, errb, code := runToolDir(t, dir, "", "-d", "x\\", "f1")
	if code != 1 || !strings.Contains(errb, "delimiter list ends with an unescaped backslash: x\\") {
		t.Errorf("bad -d: code=%d err=%q", code, errb)
	}
	// parallel: missing file aborts before output
	out, errb, code := runToolDir(t, dir, "", "f1", "nosuch")
	if code != 1 || out != "" || !strings.Contains(errb, "paste: nosuch:") {
		t.Errorf("parallel missing: out=%q err=%q code=%d", out, errb, code)
	}
	// serial: missing file diagnosed, remaining files still pasted
	out, errb, code = runToolDir(t, dir, "", "-s", "nosuch", "f1")
	if code != 1 || out != "a\n" || !strings.Contains(errb, "paste: nosuch:") {
		t.Errorf("serial missing: out=%q err=%q code=%d", out, errb, code)
	}
}

func TestPasteUnknownFlag(t *testing.T) {
	_, errb, code := runToolDir(t, t.TempDir(), "", "--frobnicate")
	if code != 2 || !strings.Contains(errb, "frobnicate") || !strings.Contains(errb, "pure-Go") {
		t.Errorf("unknown flag: code=%d err=%q", code, errb)
	}
}

func TestPasteHelpAndVersion(t *testing.T) {
	out, _, code := runToolDir(t, t.TempDir(), "", "--help")
	if code != 0 || !strings.Contains(out, "Usage: paste") {
		t.Errorf("--help: code=%d out=%q", code, out)
	}
	out, _, code = runToolDir(t, t.TempDir(), "", "--version")
	if code != 0 || !strings.Contains(out, "paste") {
		t.Errorf("--version: code=%d out=%q", code, out)
	}
}
