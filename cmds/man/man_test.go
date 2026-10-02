package mancmd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/qiangli/coreutils/tool"
)

func TestManDocumentsRegisteredAndShellCommands(t *testing.T) {
	for _, name := range []string{"man", "cd"} {
		var out, errs bytes.Buffer
		rc := &tool.RunContext{Stdio: tool.Stdio{Out: &out, Err: &errs}}
		if code := run(rc, []string{name}); code != 0 {
			t.Fatalf("man %s: code=%d stderr=%s", name, code, errs.String())
		}
		if !strings.Contains(out.String(), name) {
			t.Fatalf("man %s has no command name: %q", name, out.String())
		}
		var doc tool.MCPCommandDocumentation
		if err := json.Unmarshal(out.Bytes(), &doc); err != nil {
			t.Fatalf("man %s is not JSON: %v", name, err)
		}
		if doc.Tool.Name != name || doc.Call.Method != "tools/call" || doc.Call.Params.Name != name {
			t.Fatalf("man %s has inconsistent MCP identity: %#v", name, doc)
		}
		if doc.Tool.InputSchema["type"] != "object" {
			t.Fatalf("man %s lacks MCP input object schema: %#v", name, doc.Tool.InputSchema)
		}
	}
}

func TestKeywordSearch(t *testing.T) {
	var out, errs bytes.Buffer
	rc := &tool.RunContext{Stdio: tool.Stdio{Out: &out, Err: &errs}}
	if code := run(rc, []string{"-k", "documentation"}); code != 0 {
		t.Fatalf("man -k: code=%d stderr=%s", code, errs.String())
	}
	if !strings.Contains(out.String(), "man") {
		t.Fatalf("keyword result omits man: %q", out.String())
	}
}

func TestKeywordSearchAcceptsOptionTerminator(t *testing.T) {
	var plain, terminated, errs bytes.Buffer
	rc := &tool.RunContext{Stdio: tool.Stdio{Out: &plain, Err: &errs}}
	if code := run(rc, []string{"-k", "documentation"}); code != 0 {
		t.Fatalf("man -k: code=%d stderr=%s", code, errs.String())
	}
	rc.Out = &terminated
	if code := run(rc, []string{"-k", "--", "documentation"}); code != 0 {
		t.Fatalf("man -k --: code=%d stderr=%s", code, errs.String())
	}
	if terminated.String() != plain.String() {
		t.Fatalf("option terminator changed keyword results: %q versus %q", terminated.String(), plain.String())
	}
}

func TestTerminalOutputUsesConfiguredPager(t *testing.T) {
	old := manIsTerminal
	manIsTerminal = func(io.Writer) bool { return true }
	t.Cleanup(func() { manIsTerminal = old })

	marker := filepath.Join(t.TempDir(), "pager-ran")
	var out, errs bytes.Buffer
	rc := &tool.RunContext{
		Env:   []string{"PATH=/bin:/usr/bin", "PAGER=" + fmt.Sprintf("printf 'Test pager\\n' > %q", marker)},
		Stdio: tool.Stdio{Out: &out, Err: &errs},
	}
	if code := run(rc, []string{"man"}); code != 0 {
		t.Fatalf("man man: code=%d stderr=%s", code, errs.String())
	}
	got, err := os.ReadFile(marker)
	if err != nil {
		t.Fatalf("configured pager was not invoked: %v", err)
	}
	if string(got) != "Test pager\n" {
		t.Fatalf("pager marker = %q", got)
	}
	if out.Len() != 0 {
		t.Fatalf("terminal output bypassed configured pager: %q", out.String())
	}
}
