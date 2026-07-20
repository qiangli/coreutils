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

func TestPathchkSpecialAlsoChecksFilesystemPrefixes(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "file"), nil, 0o600); err != nil {
		t.Fatal(err)
	}

	code, errText := runPathchk(t, dir, "-P", "file/child")
	if code != 1 || !strings.Contains(errText, "not a directory") {
		t.Fatalf("code=%d stderr=%q", code, errText)
	}
}

func TestPathchkPosixPathLimitIncludesTerminator(t *testing.T) {
	// _POSIX_PATH_MAX (256) counts the terminating NUL, so a pathname of
	// exactly posixPathMax characters is already too long under -p. Use
	// single-character components so the component-length limit (14) does
	// not fire first.
	atLimit := strings.Repeat("a/", posixPathMax/2)
	if len(atLimit) != posixPathMax {
		t.Fatalf("test setup: len=%d, want %d", len(atLimit), posixPathMax)
	}
	code, errText := runPathchk(t, t.TempDir(), "-p", atLimit)
	if code != 1 || !strings.Contains(errText, "exceeds POSIX limit") {
		t.Fatalf("at limit: code=%d stderr=%q", code, errText)
	}

	belowLimit := atLimit[:len(atLimit)-1] // 255 bytes
	code, errText = runPathchk(t, t.TempDir(), "-p", belowLimit)
	if code != 0 || errText != "" {
		t.Fatalf("below limit: code=%d stderr=%q", code, errText)
	}
}

func TestPathchkDefaultPathLimitIncludesTerminator(t *testing.T) {
	limit := defaultPathMax()
	pathAtLimit := strings.Repeat("a/", limit/2)
	code, errText := runPathchk(t, t.TempDir(), pathAtLimit)
	if code != 1 || !strings.Contains(errText, "exceeds limit") {
		t.Fatalf("code=%d stderr=%q", code, errText)
	}

	pathBelowLimit := pathAtLimit[:len(pathAtLimit)-1]
	code, errText = runPathchk(t, t.TempDir(), pathBelowLimit)
	if code != 0 || errText != "" {
		t.Fatalf("code=%d stderr=%q", code, errText)
	}
}
