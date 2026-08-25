package pathchkcmd

// Issue 741: POSIX Issue 7 default-mode filesystem-limit query error and
// indeterminate handling through the limitLookup seam. Byte-sequence validity
// remains an underlying-filesystem property and is deliberately not inferred
// from LC_CTYPE.

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/qiangli/coreutils/tool"
)

var errUnsupportedQuery = errors.New("pathconf query failed for test")

func mkdirChild(t *testing.T, dir, name string) string {
	t.Helper()
	child := filepath.Join(dir, name)
	if err := os.Mkdir(child, 0o700); err != nil {
		t.Fatal(err)
	}
	return child
}

func runPathchkEnv(t *testing.T, dir string, env []string, args ...string) (int, string) {
	t.Helper()
	var out, errOut bytes.Buffer
	code := run(&tool.RunContext{
		Ctx: context.Background(), Dir: dir, Env: env,
		Stdio: tool.Stdio{In: strings.NewReader(""), Out: &out, Err: &errOut},
	}, args)
	if out.Len() != 0 {
		t.Fatalf("unexpected stdout %q", out.String())
	}
	return code, errOut.String()
}

func defaultCheck(t *testing.T, p string, limits limitLookup) (bool, string) {
	t.Helper()
	var errOut bytes.Buffer
	rc := &tool.RunContext{Ctx: context.Background(), Dir: t.TempDir(),
		Stdio: tool.Stdio{In: strings.NewReader(""), Out: io.Discard, Err: &errOut}}
	return checkDefaultWith(rc, p, limits), errOut.String()
}

func TestIssue741LimitsQueryErrorIsDiagnosed(t *testing.T) {
	limits := func(string) (int, int, error) { return 0, 0, errUnsupportedQuery }
	ok, errText := defaultCheck(t, "any/name", limits)
	if ok || !strings.Contains(errText, "cannot determine filesystem limits") ||
		!strings.Contains(errText, errUnsupportedQuery.Error()) {
		t.Fatalf("ok=%v stderr=%q", ok, errText)
	}
}

func TestIssue741IndeterminateLimitsSkipLengthChecks(t *testing.T) {
	// pathconf's -1-without-errno result reaches limitLookup as a
	// non-positive value with a nil error: no limit exists, so neither
	// length check may fire.
	limits := func(string) (int, int, error) { return -1, 0, nil }
	long := strings.Repeat("x", 5000)
	ok, errText := defaultCheck(t, "missing-first/"+long+"/"+long, limits)
	if !ok || errText != "" {
		t.Fatalf("ok=%v stderr=%q", ok, errText)
	}
}

func TestIssue741DeepContainingDirectoryQueryError(t *testing.T) {
	dir := t.TempDir()
	child := mkdirChild(t, dir, "child")
	limits := func(path string) (int, int, error) {
		if path == child {
			return 0, 0, errUnsupportedQuery
		}
		return 4096, 255, nil
	}
	var errOut bytes.Buffer
	rc := &tool.RunContext{Ctx: context.Background(), Dir: dir,
		Stdio: tool.Stdio{In: strings.NewReader(""), Out: io.Discard, Err: &errOut}}
	if checkDefaultWith(rc, "child/grand", limits) ||
		!strings.Contains(errOut.String(), "cannot determine filesystem limits at") {
		t.Fatalf("stderr=%q", errOut.String())
	}
}

func TestIssue741DeepContainingDirectoryIndeterminateNameLimit(t *testing.T) {
	dir := t.TempDir()
	child := mkdirChild(t, dir, "child")
	limits := func(path string) (int, int, error) {
		if path == child {
			return 4096, 0, nil
		}
		return 4096, 8, nil
	}
	var errOut bytes.Buffer
	rc := &tool.RunContext{Ctx: context.Background(), Dir: dir,
		Stdio: tool.Stdio{In: strings.NewReader(""), Out: io.Discard, Err: &errOut}}
	if !checkDefaultWith(rc, "child/"+strings.Repeat("n", 64), limits) || errOut.String() != "" {
		t.Fatalf("stderr=%q", errOut.String())
	}
	errOut.Reset()
	if checkDefaultWith(rc, strings.Repeat("n", 64), limits) ||
		!strings.Contains(errOut.String(), "exceeds limit 8") {
		t.Fatalf("stderr=%q", errOut.String())
	}
}

func TestIssue741LimitsFailuresAggregateAcrossOperands(t *testing.T) {
	calls := 0
	limits := func(string) (int, int, error) {
		calls++
		if calls == 1 {
			return 0, 0, errUnsupportedQuery
		}
		return 4096, 255, nil
	}
	var errOut bytes.Buffer
	rc := &tool.RunContext{Ctx: context.Background(), Dir: t.TempDir(),
		Stdio: tool.Stdio{In: strings.NewReader(""), Out: io.Discard, Err: &errOut}}
	status := 0
	for _, p := range []string{"first", "second"} {
		if !checkDefaultWith(rc, p, limits) {
			status = 1
		}
	}
	if status != 1 || calls != 2 || !strings.Contains(errOut.String(), "first") || strings.Contains(errOut.String(), "second") {
		t.Fatalf("status=%d calls=%d stderr=%q", status, calls, errOut.String())
	}
}
