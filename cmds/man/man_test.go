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

// posixBaseUPENames109 is the 109-name POSIX base + UPE inventory under test
// (see s385 R2 diagnosis names-109.txt): every name must resolve to
// documentation either via a cmds/* applet registered with tool.Lookup (under
// the full registry) or via a shellHelp entry for regular shell built-ins.
var posixBaseUPENames109 = []string{
	"awk", "basename", "bc", "cat", "cd", "chgrp", "chmod", "chown",
	"cksum", "cmp", "comm", "command", "cp", "cut", "date", "dd",
	"diff", "dirname", "echo", "ed", "env", "expr", "false", "find",
	"fold", "getconf", "getopts", "grep", "head", "id", "join", "kill",
	"ln", "locale", "localedef", "logger", "logname", "lp", "ls", "mailx",
	"mkdir", "mkfifo", "mv", "nohup", "od", "paste", "pathchk", "pax",
	"pr", "printf", "pwd", "read", "rm", "rmdir", "sed", "sh", "sleep",
	"sort", "stty", "tail", "tee", "test", "touch", "tr", "true", "tty",
	"umask", "uname", "uniq", "wait", "wc", "xargs", "alias", "at",
	"batch", "bg", "crontab", "csplit", "ctags", "df", "du", "ex",
	"expand", "fc", "fg", "file", "jobs", "man", "mesg", "more",
	"newgrp", "nice", "nm", "patch", "ps", "renice", "split", "strings",
	"tabs", "talk", "time", "tput", "unalias", "unexpand", "uudecode",
	"uuencode", "vi", "who", "write",
}

// nonAppletPOSIXNames are the members of posixBaseUPENames109 that have no
// cmds/* applet and therefore must be covered by shellHelp. ("hash" is also
// in shellHelp but is outside the 109-name set.)
var nonAppletPOSIXNames = []string{
	"alias", "bg", "cd", "command", "fc", "fg", "getopts", "jobs",
	"read", "sh", "umask", "unalias", "wait",
}

func checkManJSON(t *testing.T, name string, out bytes.Buffer) tool.MCPCommandDocumentation {
	t.Helper()
	var doc tool.MCPCommandDocumentation
	if err := json.Unmarshal(out.Bytes(), &doc); err != nil {
		t.Fatalf("man %s is not valid MCPCommandDocumentation JSON: %v", name, err)
	}
	if doc.Tool.Name != name || doc.Call.Method != "tools/call" || doc.Call.Params.Name != name {
		t.Fatalf("man %s has inconsistent MCP identity: %#v", name, doc)
	}
	if doc.Tool.InputSchema["type"] != "object" {
		t.Fatalf("man %s lacks MCP input object schema: %#v", name, doc.Tool.InputSchema)
	}
	if strings.TrimSpace(doc.Tool.Description) == "" {
		t.Fatalf("man %s has empty synopsis", name)
	}
	return doc
}

func TestManJobControlBuiltins(t *testing.T) {
	for _, name := range []string{"bg", "fc", "fg", "jobs"} {
		var out, errs bytes.Buffer
		rc := &tool.RunContext{Stdio: tool.Stdio{Out: &out, Err: &errs}}
		if code := run(rc, []string{name}); code != 0 {
			t.Fatalf("man %s: code=%d stderr=%s", name, code, errs.String())
		}
		checkManJSON(t, name, out)
	}
}

func TestPOSIX109NamesShellBuiltinCoverage(t *testing.T) {
	if len(posixBaseUPENames109) != 109 {
		t.Fatalf("POSIX base + UPE inventory = %d names, want 109", len(posixBaseUPENames109))
	}
	seen := make(map[string]bool, len(posixBaseUPENames109))
	for _, name := range posixBaseUPENames109 {
		if seen[name] {
			t.Fatalf("duplicate name %q in POSIX inventory", name)
		}
		seen[name] = true
	}
	for _, name := range nonAppletPOSIXNames {
		if !seen[name] {
			t.Fatalf("non-applet builtin %q missing from POSIX inventory", name)
		}
		if _, ok := shellHelp[name]; !ok {
			t.Errorf("POSIX shell built-in %q has no shellHelp entry", name)
		}
	}
	// shellHelp must cover exactly the known builtins: the 13 non-applet
	// POSIX names plus hash. Anything else is drift in either direction.
	wantKeys := append(append([]string{}, nonAppletPOSIXNames...), "hash")
	if len(shellHelp) != len(wantKeys) {
		t.Fatalf("shellHelp has %d entries, want %d", len(shellHelp), len(wantKeys))
	}
	for _, name := range wantKeys {
		if _, ok := shellHelp[name]; !ok {
			t.Errorf("shellHelp missing expected builtin %q", name)
		}
	}
	for _, name := range wantKeys {
		var out, errs bytes.Buffer
		rc := &tool.RunContext{Stdio: tool.Stdio{Out: &out, Err: &errs}}
		if code := run(rc, []string{name}); code != 0 {
			t.Errorf("man %s: code=%d stderr=%s", name, code, errs.String())
			continue
		}
		checkManJSON(t, name, out)
	}
}

func TestKeywordSearchFindsJobControlBuiltins(t *testing.T) {
	cases := []struct {
		term string
		want []string
	}{
		{"foreground", []string{"fg"}},
		{"background", []string{"bg"}},
		{"command history", []string{"fc"}},
		{"jobs", []string{"bg", "fg", "jobs"}},
	}
	for _, tc := range cases {
		var first, second, errs bytes.Buffer
		rc := &tool.RunContext{Stdio: tool.Stdio{Out: &first, Err: &errs}}
		if code := run(rc, []string{"-k", tc.term}); code != 0 {
			t.Fatalf("man -k %q: code=%d stderr=%s", tc.term, code, errs.String())
		}
		rc.Out = &second
		if code := run(rc, []string{"-k", tc.term}); code != 0 {
			t.Fatalf("man -k %q rerun: code=%d stderr=%s", tc.term, code, errs.String())
		}
		if first.String() != second.String() {
			t.Fatalf("man -k %q is not deterministic:\n%q versus\n%q", tc.term, first.String(), second.String())
		}
		for _, name := range tc.want {
			found := false
			for _, line := range strings.Split(strings.TrimSpace(first.String()), "\n") {
				if strings.HasPrefix(line, name+" ") {
					found = true
					break
				}
			}
			if !found {
				t.Fatalf("man -k %q omits entry for %s: %q", tc.term, name, first.String())
			}
		}
		// Matched shellHelp lines must appear in sorted order.
		var matched []string
		for _, line := range strings.Split(strings.TrimSpace(first.String()), "\n") {
			if syn, ok := shellHelp[strings.SplitN(line, " ", 2)[0]]; ok && syn == line {
				matched = append(matched, line)
			}
		}
		for i := 1; i < len(matched); i++ {
			if matched[i-1] > matched[i] {
				t.Fatalf("man -k %q shell entries out of order: %q", tc.term, matched)
			}
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
