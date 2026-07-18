package pathchkcmd

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/qiangli/coreutils/tool"
)

func runPathchk(t *testing.T, dir string, args ...string) (int, string) {
	t.Helper()
	var out, err bytes.Buffer
	code := run(&tool.RunContext{
		Ctx: context.Background(),
		Dir: dir,
		Stdio: tool.Stdio{
			Out: &out,
			Err: &err,
			In:  strings.NewReader(""),
		},
	}, args)
	if out.Len() != 0 {
		t.Fatalf("unexpected stdout %q", out.String())
	}
	return code, err.String()
}

func TestPathchkPortable(t *testing.T) {
	var out, err bytes.Buffer
	code := run(&tool.RunContext{Ctx: context.Background(), Dir: t.TempDir(), Stdio: tool.Stdio{Out: &out, Err: &err, In: strings.NewReader("")}}, []string{"-p", "abc/def"})
	if code != 0 {
		t.Fatalf("code=%d err=%s", code, err.String())
	}
}

func TestPathchkRejectsLeadingHyphen(t *testing.T) {
	var out, err bytes.Buffer
	code := run(&tool.RunContext{Ctx: context.Background(), Dir: t.TempDir(), Stdio: tool.Stdio{Out: &out, Err: &err, In: strings.NewReader("")}}, []string{"-P", "./-bad"})
	if code != 1 {
		t.Fatalf("code=%d out=%s err=%s", code, out.String(), err.String())
	}
}

func TestPathchkEmptyPathnameOptions(t *testing.T) {
	dir := t.TempDir()
	for _, tc := range []struct {
		name string
		args []string
		code int
	}{
		{name: "default", args: []string{""}, code: 0},
		{name: "posix portability", args: []string{"-p", ""}, code: 0},
		{name: "special portability", args: []string{"-P", ""}, code: 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			code, errText := runPathchk(t, dir, tc.args...)
			if code != tc.code {
				t.Fatalf("code=%d, want %d; stderr=%q", code, tc.code, errText)
			}
			if (code == 0) != (errText == "") {
				t.Fatalf("code=%d stderr=%q", code, errText)
			}
		})
	}
}

func TestPathchkRejectsNonDirectoryPrefix(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "file"), nil, 0o600); err != nil {
		t.Fatal(err)
	}

	code, errText := runPathchk(t, dir, "file/child/grandchild")
	if code != 1 || !strings.Contains(errText, "not a directory") {
		t.Fatalf("code=%d stderr=%q", code, errText)
	}
}

func TestPathchkAllowsMissingDirectoryPrefix(t *testing.T) {
	code, errText := runPathchk(t, t.TempDir(), "missing/child")
	if code != 0 || errText != "" {
		t.Fatalf("code=%d stderr=%q", code, errText)
	}
}
