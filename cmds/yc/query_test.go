package yccmd

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/qiangli/coreutils/tool"
)

func runYC(t *testing.T, dir string, args ...string) (out, errOut string, code int) {
	t.Helper()
	var o, e bytes.Buffer
	rc := &tool.RunContext{
		Ctx:   context.Background(),
		Dir:   dir,
		Stdio: tool.Stdio{In: strings.NewReader(""), Out: &o, Err: &e},
	}
	code = cmd.Run(rc, args)
	return o.String(), e.String(), code
}

func writeGo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	src := "package p\n\nfunc Hello() {}\n\nfunc World(x int) int { return x }\n"
	if err := os.WriteFile(filepath.Join(dir, "a.go"), []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestYCQueryGoFunctions(t *testing.T) {
	dir := writeGo(t)
	out, errOut, code := runYC(t, dir, "query", "--lang", "go",
		"(function_declaration name: (identifier) @fn)", ".")
	if code != 0 {
		t.Fatalf("exit %d: %s", code, errOut)
	}
	if !strings.Contains(out, "@fn Hello") || !strings.Contains(out, "@fn World") {
		t.Errorf("expected captures for Hello and World:\n%s", out)
	}
	// file:line:col prefix present
	if !strings.Contains(out, "a.go:3:") {
		t.Errorf("expected a.go:3 for Hello:\n%s", out)
	}
}

func TestYCQueryJSON(t *testing.T) {
	dir := writeGo(t)
	out, _, code := runYC(t, dir, "query", "--lang", "go", "--json",
		"(function_declaration name: (identifier) @fn)", ".")
	if code != 0 {
		t.Fatalf("exit %d", code)
	}
	if !strings.Contains(out, `"name": "fn"`) || !strings.Contains(out, `"text": "Hello"`) {
		t.Errorf("json output missing fields:\n%s", out)
	}
}

func TestYCQueryInferLangFromFile(t *testing.T) {
	dir := writeGo(t)
	// No --lang; single .go file target → language inferred.
	out, errOut, code := runYC(t, dir, "query",
		"(function_declaration name: (identifier) @fn)", "a.go")
	if code != 0 {
		t.Fatalf("exit %d: %s", code, errOut)
	}
	if !strings.Contains(out, "@fn Hello") {
		t.Errorf("inferred-lang query failed:\n%s", out)
	}
}

func TestYCQueryInvalidFailsLoudly(t *testing.T) {
	dir := writeGo(t)
	_, errOut, code := runYC(t, dir, "query", "--lang", "go", "(this is not valid", ".")
	if code == 0 {
		t.Error("an invalid query should fail loudly")
	}
	if !strings.Contains(strings.ToLower(errOut), "invalid query") {
		t.Errorf("error should name the cause: %q", errOut)
	}
}

func TestYCQueryNeedsLangForDir(t *testing.T) {
	dir := writeGo(t)
	_, errOut, code := runYC(t, dir, "query", "(x)") // dir target, no --lang
	if code == 0 || !strings.Contains(errOut, "--lang") {
		t.Errorf("querying a dir without --lang should error asking for it: %q", errOut)
	}
}
