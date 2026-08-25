package pathchkcmd

import (
	"bytes"
	"context"
	"io"
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
		{name: "default", args: []string{""}, code: 1},
		{name: "posix portability", args: []string{"-p", ""}, code: 1},
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
	const limit = 32
	limits := func(string) (int, int, error) { return limit, 255, nil }
	pathAtLimit := strings.Repeat("a"+string(filepath.Separator), limit/2)
	dir := t.TempDir()
	var errOut bytes.Buffer
	rc := &tool.RunContext{Ctx: context.Background(), Dir: dir,
		Stdio: tool.Stdio{In: strings.NewReader(""), Out: io.Discard, Err: &errOut}}
	ok := checkDefaultWith(rc, pathAtLimit, limits)
	code, errText := 0, errOut.String()
	if !ok {
		code = 1
	}
	if code != 1 || !strings.Contains(errText, "exceeds limit") {
		t.Fatalf("code=%d stderr=%q", code, errText)
	}

	pathBelowLimit := pathAtLimit[:len(pathAtLimit)-1]
	errOut.Reset()
	ok = checkDefaultWith(rc, pathBelowLimit, limits)
	code, errText = 0, errOut.String()
	if !ok {
		code = 1
	}
	if code != 0 || errText != "" {
		t.Fatalf("code=%d stderr=%q", code, errText)
	}
}

func TestPathchkDefaultUsesContainingDirectoryNameLimit(t *testing.T) {
	dir := t.TempDir()
	child := filepath.Join(dir, "child")
	if err := os.Mkdir(child, 0o700); err != nil {
		t.Fatal(err)
	}
	var queried []string
	limits := func(path string) (int, int, error) {
		queried = append(queried, path)
		if path == child {
			return 4096, 3, nil
		}
		return 4096, 255, nil
	}
	var errOut bytes.Buffer
	rc := &tool.RunContext{Ctx: context.Background(), Dir: dir,
		Stdio: tool.Stdio{In: strings.NewReader(""), Out: io.Discard, Err: &errOut}}
	if checkDefaultWith(rc, "child/long", limits) || !strings.Contains(errOut.String(), "exceeds limit 3") {
		t.Fatalf("queries=%q err=%q", queried, errOut.String())
	}
	if len(queried) < 2 || queried[0] != dir || queried[1] != child {
		t.Fatalf("limit queries=%q, want containing directories %q then %q", queried, dir, child)
	}
}

func TestPathchkDefaultPreservesResolutionSyntax(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "file"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	for _, operand := range []string{"file/", "file/..", "file/../child"} {
		code, errText := runPathchk(t, dir, operand)
		if code != 1 || !strings.Contains(errText, "not a directory") {
			t.Fatalf("operand=%q code=%d err=%q", operand, code, errText)
		}
	}
}

func TestPathchkPosixNameLimitAndPortableCharacters(t *testing.T) {
	code, errText := runPathchk(t, t.TempDir(), "-p", strings.Repeat("a", posixNameMax))
	if code != 0 || errText != "" {
		t.Fatalf("at POSIX name limit: code=%d stderr=%q", code, errText)
	}

	code, errText = runPathchk(t, t.TempDir(), "-p", strings.Repeat("a", posixNameMax+1))
	if code != 1 || !strings.Contains(errText, "exceeds POSIX limit") {
		t.Fatalf("over POSIX name limit: code=%d stderr=%q", code, errText)
	}

	for _, name := range []string{"aü", "x:y", "bad*name"} {
		code, errText = runPathchk(t, t.TempDir(), "-p", name)
		if code != 1 || !strings.Contains(errText, "nonportable") {
			t.Fatalf("nonportable %q: code=%d stderr=%q", name, code, errText)
		}
	}
}

func TestPathchkPosixDoesNotRejectLeadingHyphen(t *testing.T) {
	code, errText := runPathchk(t, t.TempDir(), "-p", "--", "-foo")
	if code != 0 || errText != "" {
		t.Fatalf("code=%d stderr=%q", code, errText)
	}
}

func TestPathchkMultipleOperandsAggregateStatus(t *testing.T) {
	code, errText := runPathchk(t, t.TempDir(), "-p", "ok", "bad*name", "also_ok")
	if code != 1 || !strings.Contains(errText, "bad*name") {
		t.Fatalf("code=%d stderr=%q", code, errText)
	}
	if strings.Contains(errText, "ok") || strings.Contains(errText, "also_ok") {
		t.Fatalf("unexpected diagnostics for passing operands: %q", errText)
	}

	code, errText = runPathchk(t, t.TempDir(), "-p", "ok", "also_ok")
	if code != 0 || errText != "" {
		t.Fatalf("all-pass aggregate: code=%d stderr=%q", code, errText)
	}
}

func TestPathchkMissingOperandUsage(t *testing.T) {
	code, errText := runPathchk(t, t.TempDir())
	if code != 2 || !strings.Contains(errText, "missing operand") {
		t.Fatalf("code=%d stderr=%q", code, errText)
	}
}
