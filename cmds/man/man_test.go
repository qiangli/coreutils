package mancmd

import (
	"bytes"
	"encoding/json"
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
