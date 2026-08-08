package lean

import (
	"bytes"
	"context"
	"os"
	"sort"
	"strings"
	"testing"

	"github.com/qiangli/coreutils/multicall"
	"github.com/qiangli/coreutils/tool"
)

// leanExcluded are the applets present in cmds/all but deliberately NOT in the
// lean inventory: agent extensions and non-utility commands whose dependency
// trees dominate the binary's page footprint. This list IS the contract; keep
// it in sync with the lean.go package comment.
var leanExcluded = map[string]bool{
	"ast":      true,
	"browser":  true,
	"clip":     true,
	"duration": true,
	"fetch":    true,
	"graph":    true,
	"jq":       true,
	"tokens":   true,
}

// notInAll are applet dirs deliberately excluded from BOTH cmds/all and
// cmds/lean (host front-door verbs, not bare userland). Mirrors the note in
// cmds/all/all.go so a future addition there stays consistent.
var notInAll = map[string]bool{
	"foreman":   true,
	"resources": true,
}

// nonAppletDirs are cmds/ entries that are not individual applets.
var nonAppletDirs = map[string]bool{
	"all":       true,
	"internal":  true,
	"lean":      true,
	"perfbench": true,
}

// TestLeanExcludedAbsent asserts every documented exclusion is genuinely absent
// from the lean registry — i.e. the binary fails closed for them, not silently
// links them.
func TestLeanExcludedAbsent(t *testing.T) {
	for name := range leanExcluded {
		if tool.Lookup(name) != nil {
			t.Errorf("lean build must not register excluded applet %q", name)
		}
	}
}

// TestLeanCoreUtilitiesPresent asserts the standard cheap helpers that motivate
// the lean build are all present and runnable.
func TestLeanCoreUtilitiesPresent(t *testing.T) {
	for _, name := range []string{
		"true", "false", "expr", "test", "echo", "printf", "cat", "sed", "grep",
		"cmp", "cut", "sort", "wc", "ls", "cp", "mv", "rm", "mkdir", "head",
		"tail", "tr", "env", "xargs", "find", "awk", "tar", "diff",
	} {
		if tool.Lookup(name) == nil {
			t.Errorf("lean build must register standard utility %q", name)
		}
	}
}

// TestLeanInventoryMatchesFilesystem is the drift guard: every applet directory
// under cmds/ must be either registered by the lean inventory, on the lean
// exclusion list, on the cmds/all exclusion list, or a non-applet dir. A new
// command added to the tree without a deliberate lean-vs-full decision fails
// this test so the inventory stays intentional.
func TestLeanInventoryMatchesFilesystem(t *testing.T) {
	entries, err := os.ReadDir("..") // cmds/
	if err != nil {
		t.Fatalf("read cmds/: %v", err)
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		name := e.Name()
		if nonAppletDirs[name] || notInAll[name] {
			continue
		}
		if leanExcluded[name] {
			continue
		}
		// Most applet dirs register a tool under the dir name. A few register
		// only an alias spelling (handled below); allow those explicitly.
		if tool.Lookup(name) != nil {
			continue
		}
		t.Errorf("applet dir cmds/%s is neither registered by the lean inventory nor listed as excluded — decide lean-vs-full and update cmds/lean/lean.go or leanExcluded", name)
	}
}

// TestLeanDispatchFailClosed asserts the multicall dispatch path rejects an
// excluded applet with exit code 2 (usage error), matching the standalone
// binary's "not a supported command" behavior — never a silent no-op or
// approximation.
func TestLeanDispatchFailClosed(t *testing.T) {
	for _, name := range []string{"jq", "ast", "browser", "fetch", "tokens", "definitely-not-a-command"} {
		var sb strings.Builder
		rc := &tool.RunContext{
			Ctx:   context.Background(),
			Stdio: tool.Stdio{Out: &sb, Err: &sb},
		}
		if code := multicall.Dispatch(rc, name, nil); code != 2 {
			t.Errorf("Dispatch(%q) = exit %d, want 2 (fail closed)", name, code)
		}
	}
}

// TestLeanDispatchCheapCommands asserts the cheap helpers that motivated this
// build dispatch and produce correct results through the real multicall path.
func TestLeanDispatchCheapCommands(t *testing.T) {
	cases := []struct {
		name string
		args []string
		want int
		out  string
	}{
		{"true", nil, 0, ""},
		{"false", nil, 1, ""},
		{"expr", []string{"1", "+", "2"}, 0, "3\n"},
		{"test", []string{"1", "-eq", "1"}, 0, ""},
		{"echo", []string{"hi"}, 0, "hi\n"},
	}
	for _, c := range cases {
		var out strings.Builder
		rc := &tool.RunContext{
			Ctx:   context.Background(),
			Stdio: tool.Stdio{Out: &out, Err: &strings.Builder{}},
		}
		if code := multicall.Dispatch(rc, c.name, c.args); code != c.want || out.String() != c.out {
			t.Errorf("Dispatch(%q, %v) = (code=%d, out=%q), want (code=%d, out=%q)",
				c.name, c.args, code, out.String(), c.want, c.out)
		}
	}
}

// TestLeanInventorySortedExports is a documentation/consistency check: the
// registered names are sorted (tool.Names guarantees this) so --list output is
// deterministic across builds — a property the lean binary's consumers rely on.
func TestLeanInventorySorted(t *testing.T) {
	names := tool.Names()
	if len(names) < 100 {
		t.Fatalf("lean inventory unexpectedly small: %d names", len(names))
	}
	if !sort.StringsAreSorted(names) {
		t.Errorf("tool.Names() is not sorted; --list would be non-deterministic")
	}
}

// Benchmarks measure the deterministic in-process multicall dispatch path for
// the cheap commands that motivate the lean build. They do NOT capture
// process-startup page-fault cost (that is binary-size-driven and measured by
// scripts/lean-bench.sh against the built binaries); they isolate the dispatch
// overhead so a regression in Resolve/Dispatch/Lookup shows up clearly in CI
// independent of the host's page cache.

func benchDispatch(b *testing.B, name string, args []string) {
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		rc := &tool.RunContext{
			Ctx:   context.Background(),
			Stdio: tool.Stdio{Out: &bytes.Buffer{}, Err: &bytes.Buffer{}, In: strings.NewReader("")},
		}
		if code := multicall.Dispatch(rc, name, args); code != 0 && code != 1 {
			b.Fatalf("Dispatch(%q) exit %d", name, code)
		}
	}
}

func BenchmarkLeanDispatchTrue(b *testing.B) { benchDispatch(b, "true", nil) }
func BenchmarkLeanDispatchExpr(b *testing.B) { benchDispatch(b, "expr", []string{"1", "+", "2"}) }
func BenchmarkLeanDispatchTest(b *testing.B) { benchDispatch(b, "test", []string{"1", "-eq", "1"}) }
func BenchmarkLeanDispatchEcho(b *testing.B) { benchDispatch(b, "echo", []string{"x"}) }

func BenchmarkLeanDispatchCmp(b *testing.B) {
	// cmp needs two equal regular files (/dev/null is non-regular and errors).
	f1, err := os.CreateTemp("", "lean-cmp-*")
	if err != nil {
		b.Fatal(err)
	}
	defer os.Remove(f1.Name())
	f1.WriteString("same\n")
	f1.Close()
	f2, err := os.CreateTemp("", "lean-cmp-*")
	if err != nil {
		b.Fatal(err)
	}
	defer os.Remove(f2.Name())
	f2.WriteString("same\n")
	f2.Close()
	benchDispatch(b, "cmp", []string{f1.Name(), f2.Name()})
}

// BenchmarkLeanLookup isolates the registry lookup that Dispatch performs,
// confirming the map+RLock path is constant-time regardless of inventory size.
func BenchmarkLeanLookup(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		if tool.Lookup("true") == nil {
			b.Fatal("true not registered")
		}
	}
}
