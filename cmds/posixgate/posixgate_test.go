// Copyright (c) 2025 qiangli
// See LICENSE for licensing information

// Hermetic: every test runs against temp directories, a seamed registry view,
// and a seamed shell probe. Nothing here spawns a real shell, downloads,
// compiles, or reaches the real provider cache.
package posixgatecmd

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/qiangli/coreutils/pkg/posixprovider"
	"github.com/qiangli/coreutils/tool"
)

// ---------------------------------------------------------------------------
// the spec: the yardstick itself
// ---------------------------------------------------------------------------

// TestSpecMatchesDocsInventory pins the embedded copy to the generated
// canonical inventory byte for byte. scripts/applet-matrix.py writes both;
// this is the in-Go tripwire for a hand edit to either.
func TestSpecMatchesDocsInventory(t *testing.T) {
	canonical, err := os.ReadFile(filepath.Join("..", "..", "docs", "posix-required-commands.tsv"))
	if err != nil {
		t.Fatalf("cannot read the canonical inventory: %v", err)
	}
	if string(canonical) != specTSV {
		t.Error("embedded posix-required-commands.tsv differs from docs/posix-required-commands.tsv; regenerate with scripts/applet-matrix.py")
	}
}

func TestSpecPinnedCounts(t *testing.T) {
	spec, err := loadSpec()
	if err != nil {
		t.Fatal(err)
	}
	counts := map[Owner]int{}
	for _, r := range spec {
		counts[r.Owner]++
	}
	if len(spec) != pinTotal || counts[OwnerGoApplet] != pinGoApplets ||
		counts[OwnerShell] != pinShell || counts[OwnerProvider] != pinProviders {
		t.Errorf("spec shape = %d total %v, pinned %d/%d/%d/%d",
			len(spec), counts, pinTotal, pinGoApplets, pinShell, pinProviders)
	}
}

// TestSpecProvidersMatchManifest: the inventory's external_provider rows and
// the provider manifest must be the same set, both directions.
func TestSpecProvidersMatchManifest(t *testing.T) {
	spec, err := loadSpec()
	if err != nil {
		t.Fatal(err)
	}
	if fs := verifyInventory(spec, posixprovider.Names()); len(fs) != 0 {
		for _, f := range fs {
			t.Errorf("%s", f)
		}
	}
}

func TestParseSpecRejectsMalformedInventories(t *testing.T) {
	header := "command\tcoreutils_go_applet\tgo_package\tshell_provided\tprofile_cd_disposition\n"
	cases := []struct {
		name, text, want string
	}{
		{"duplicate", header + "cat\tyes\tcmds/cat\tno\tgo_applet\ncat\tyes\tcmds/cat\tno\tgo_applet\n", "duplicate"},
		{"unknown disposition", header + "cat\tyes\tcmds/cat\tno\thost_binary\n", "unknown disposition"},
		{"wrong columns", header + "cat\tyes\tgo_applet\n", "5 tab-separated columns"},
		{"bad header", "name\tb\tc\td\te\ncat\tyes\tcmds/cat\tno\tgo_applet\n", "unrecognized header"},
		{"empty", header, "empty"},
	}
	for _, c := range cases {
		if _, err := parseSpec(c.text); err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: err = %v, want mention of %q", c.name, err, c.want)
		}
	}
}

// TestExpectedShellClasses pins the effective-owner classification the runtime
// gate demands, including every builtin overlap and the time keyword.
func TestExpectedShellClasses(t *testing.T) {
	want := map[string]string{
		// shell entry point and builtins
		"sh": "file", "cd": "builtin", "read": "builtin", "umask": "builtin",
		"wait": "builtin", "command": "builtin", "alias": "builtin",
		// go applets that a POSIX-mode bash-family shell shadows as builtins
		"echo": "builtin", "false": "builtin", "kill": "builtin",
		"printf": "builtin", "pwd": "builtin", "test": "builtin", "true": "builtin",
		// the one keyword overlap
		"time": "keyword",
		// plain applets and providers stay files
		"sed": "file", "ls": "file", "make": "file", "vi": "file",
	}
	spec, err := loadSpec()
	if err != nil {
		t.Fatal(err)
	}
	rows := map[string]specRow{}
	overlapSeen := 0
	for _, r := range spec {
		rows[r.Command] = r
		if got := expectedShellClass(r); r.Owner == OwnerGoApplet && got != "file" {
			overlapSeen++
		}
	}
	for name, class := range want {
		r, ok := rows[name]
		if !ok {
			t.Fatalf("%s is not in the inventory", name)
		}
		if got := expectedShellClass(r); got != class {
			t.Errorf("expectedShellClass(%s) = %q, want %q", name, got, class)
		}
	}
	// 7 builtin overlaps + the time keyword, and not one more: an eighth would
	// mean a shell builtin silently took over an applet-owned name.
	if overlapSeen != 8 {
		t.Errorf("go_applet names with a non-file effective class = %d, want 8", overlapSeen)
	}
}

// ---------------------------------------------------------------------------
// inventory and ownership gates
// ---------------------------------------------------------------------------

func findingsHave(fs []Finding, check, name, detail string) bool {
	for _, f := range fs {
		if f.Check == check && f.Name == name && strings.Contains(f.Detail, detail) {
			return true
		}
	}
	return false
}

func TestVerifyInventoryRejectsDrift(t *testing.T) {
	spec, err := loadSpec()
	if err != nil {
		t.Fatal(err)
	}
	// One applet dropped: total and go-applet pins both trip.
	fs := verifyInventory(spec[:len(spec)-1], posixprovider.Names())
	if !findingsHave(fs, "count-drift", "", "pinned") {
		t.Errorf("dropped name produced no count-drift finding: %v", fs)
	}
	// Manifest pins a name the inventory does not list, and vice versa.
	fs = verifyInventory(spec, append(posixprovider.Names(), "cpio"))
	if !findingsHave(fs, "provider-set", "cpio", "not an external_provider") {
		t.Errorf("extra manifest pin not rejected: %v", fs)
	}
	var withoutMake []string
	for _, n := range posixprovider.Names() {
		if n != "make" {
			withoutMake = append(withoutMake, n)
		}
	}
	fs = verifyInventory(spec, withoutMake)
	if !findingsHave(fs, "provider-set", "make", "does not pin it") {
		t.Errorf("missing manifest pin not rejected: %v", fs)
	}
}

func TestVerifyPinsRejectsIncompleteRows(t *testing.T) {
	full := posixprovider.Entries()[0]
	if fs := verifyPins(posixprovider.Entries()); len(fs) != 0 {
		t.Fatalf("real manifest rejected: %v", fs)
	}
	for _, c := range []struct {
		strip func(posixprovider.Entry) posixprovider.Entry
		want  string
	}{
		{func(e posixprovider.Entry) posixprovider.Entry { e.Version = ""; return e }, "version"},
		{func(e posixprovider.Entry) posixprovider.Entry { e.SHA256 = "abc"; return e }, "sha256"},
		{func(e posixprovider.Entry) posixprovider.Entry { e.Platforms = nil; return e }, "platforms"},
		{func(e posixprovider.Entry) posixprovider.Entry { e.URL = ""; return e }, "URL"},
	} {
		if fs := verifyPins([]posixprovider.Entry{c.strip(full)}); !findingsHave(fs, "provider-pin", full.Command, c.want) {
			t.Errorf("stripped %s not rejected: %v", c.want, fs)
		}
	}
}

// fullRegistry models the assembled multicall: every go_applet and provider
// name registered, nothing else.
func fullRegistry(t *testing.T) map[string]bool {
	t.Helper()
	spec, err := loadSpec()
	if err != nil {
		t.Fatal(err)
	}
	reg := map[string]bool{}
	for _, r := range spec {
		if r.Owner != OwnerShell {
			reg[r.Command] = true
		}
	}
	return reg
}

func TestVerifyOwnership(t *testing.T) {
	spec, err := loadSpec()
	if err != nil {
		t.Fatal(err)
	}
	reg := fullRegistry(t)
	lookup := func(n string) bool { return reg[n] }

	if fs := verifyOwnership(spec, lookup, posixprovider.Has, false); len(fs) != 0 {
		t.Fatalf("complete registry rejected: %v", fs)
	}

	// A missing applet is a name the host would silently supply.
	reg["ls"] = false
	if fs := verifyOwnership(spec, lookup, posixprovider.Has, false); !findingsHave(fs, "ownership", "ls", "not in the tool registry") {
		t.Errorf("unregistered applet not rejected")
	}
	reg["ls"] = true

	// A missing provider name, same failure class.
	reg["make"] = false
	if fs := verifyOwnership(spec, lookup, posixprovider.Has, false); !findingsHave(fs, "ownership", "make", "not in the tool registry") {
		t.Errorf("unregistered provider not rejected")
	}
	reg["make"] = true

	// A registered tool under a shell-owned name is ambiguous ownership.
	reg["cd"] = true
	if fs := verifyOwnership(spec, lookup, posixprovider.Has, false); !findingsHave(fs, "ownership", "cd", "ambiguous") {
		t.Errorf("shell-name shadowing not rejected")
	}
	reg["cd"] = false

	// An applet that is also pinned as a provider is ambiguous ownership.
	hasPlus := func(n string) bool { return n == "sed" || posixprovider.Has(n) }
	if fs := verifyOwnership(spec, lookup, hasPlus, false); !findingsHave(fs, "ownership", "sed", "ambiguous") {
		t.Errorf("applet/provider double claim not rejected")
	}

	// The provider opt-out is a hard failure for a certification runtime.
	if fs := verifyOwnership(spec, lookup, posixprovider.Has, true); !findingsHave(fs, "opt-out", "", "BASHY_POSIX_PROVIDERS") {
		t.Errorf("opt-out not rejected")
	}
}

// TestVerifyRegistryFailsClosedOnPartialUserland: in this package's test
// process only posix-gate itself is registered, so the live-registry gate must
// reject — a partial userland can never pass by accident.
func TestVerifyRegistryFailsClosedOnPartialUserland(t *testing.T) {
	fs := VerifyRegistry("")
	if len(fs) == 0 {
		t.Fatal("VerifyRegistry passed against a registry with no userland")
	}
	if !findingsHave(fs, "ownership", "cat", "not in the tool registry") {
		t.Errorf("missing applet cat not named: %v", fs)
	}
}

// ---------------------------------------------------------------------------
// provider gate
// ---------------------------------------------------------------------------

// provision writes a fake provisioned provider plus the provenance sidecar the
// build recipe would have written (same shape as cmds/posixproviders' tests).
func provision(t *testing.T, root, name string) {
	t.Helper()
	e, ok := posixprovider.Lookup(name)
	if !ok {
		t.Fatalf("no manifest entry for %q", name)
	}
	dir := filepath.Join(root, e.Command, e.Version)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	body := "#!/bin/sh\nexit 0\n"
	if err := os.WriteFile(filepath.Join(dir, e.Command), []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256([]byte(body))
	prov := fmt.Sprintf("command\t%s\nversion\t%s\nlicense\t%s\nsource_url\t%s\nsource_sha256\t%s\ncompiler\ttest\nbuilt_sha256\t%s\n",
		e.Command, e.Version, e.License, e.URL, e.SHA256, hex.EncodeToString(sum[:]))
	if err := os.WriteFile(filepath.Join(dir, "provenance.tsv"), []byte(prov), 0o644); err != nil {
		t.Fatal(err)
	}
}

// provisionAll stages every pinned provider. Every manifest row declares
// linux, so the linux resolver view exercises the full-pass path on any host.
func provisionAll(t *testing.T, root string) {
	t.Helper()
	for _, e := range posixprovider.Entries() {
		provision(t, root, e.Command)
	}
}

// withLinuxGate pins the gate's platform view to linux — the certification
// platform, and the one every manifest row declares — so the full-pass path is
// tested on every host.
func withLinuxGate(t *testing.T) {
	t.Helper()
	prev := gateGOOS
	gateGOOS = "linux"
	t.Cleanup(func() { gateGOOS = prev })
}

func TestVerifyProviders(t *testing.T) {
	root := t.TempDir()
	r := posixprovider.Resolver{CacheRoot: root, GOOS: "linux"}

	// Unprovisioned cache: every provider is a rejection.
	if fs := VerifyProviders(r); !findingsHave(fs, "provider", "make", "not provisioned") {
		t.Errorf("empty cache not rejected: %v", fs)
	}

	provisionAll(t, root)
	if fs := VerifyProviders(r); len(fs) != 0 {
		t.Errorf("provisioned cache rejected: %v", fs)
	}

	// A platform a manifest row does not declare is a FAILURE, not a skip: a
	// runtime that cannot supply all sixteen names is not the claimed runtime.
	fs := VerifyProviders(posixprovider.Resolver{CacheRoot: root, GOOS: "windows"})
	if !findingsHave(fs, "provider", "ed", "not declared for windows") {
		t.Errorf("undeclared platform not rejected: %v", fs)
	}

	// A binary that no longer matches its provenance is unattributable.
	e, _ := posixprovider.Lookup("make")
	bin := filepath.Join(root, e.Command, e.Version, e.Command)
	if err := os.WriteFile(bin, []byte("tampered"), 0o755); err != nil {
		t.Fatal(err)
	}
	if fs := VerifyProviders(r); !findingsHave(fs, "provider", "make", "provenance") {
		t.Errorf("tampered binary not rejected: %v", fs)
	}
}

// ---------------------------------------------------------------------------
// runtime gate
// ---------------------------------------------------------------------------

// stageBindir builds a staged tool directory holding an executable entry for
// every PATH-owned name.
func stageBindir(t *testing.T, spec []specRow) string {
	t.Helper()
	dir := t.TempDir()
	for _, r := range spec {
		if !pathOwned(r) {
			continue
		}
		if err := os.WriteFile(filepath.Join(dir, r.Command), []byte("#!/bin/sh\n"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func runtimeRC(t *testing.T, env ...string) *tool.RunContext {
	t.Helper()
	return &tool.RunContext{
		Ctx: context.Background(),
		Dir: t.TempDir(),
		Env: env,
		FS:  tool.NewLocalFS(),
		Stdio: tool.Stdio{
			In:  strings.NewReader(""),
			Out: &bytes.Buffer{},
			Err: &bytes.Buffer{},
		},
	}
}

// fakeShell answers the gate's four probes the way a healthy staged POSIX-mode
// shell would, with per-test overrides for the classification table.
func fakeShell(t *testing.T, spec []specRow, classOverride map[string]string, posixOn bool, childHasVar bool) func(*tool.RunContext, string, ...string) (string, error) {
	t.Helper()
	return func(rc *tool.RunContext, shell string, args ...string) (string, error) {
		if len(args) < 2 || args[0] != "-c" {
			t.Fatalf("unexpected shell invocation: %v", args)
		}
		switch script := args[1]; {
		case script == classifyScript:
			var b strings.Builder
			classes := map[string]string{}
			for _, r := range spec {
				classes[r.Command] = expectedShellClass(r)
			}
			maps.Copy(classes, classOverride)
			for _, n := range args[3:] { // args[2] is $0
				fmt.Fprintf(&b, "%s %s\n", n, classes[n])
			}
			return b.String(), nil
		case script == "set -o":
			state := "on"
			if !posixOn {
				state = "off"
			}
			return "noexec         off\nposix          " + state + "\nverbose        off\n", nil
		case strings.Contains(script, "$POSIXLY_CORRECT"):
			return rc.Getenv("POSIXLY_CORRECT") + "\n", nil
		case script == "exec env":
			if childHasVar {
				// Real bash rewrites the exported value to "y" on entering
				// posix mode; the gate must accept any non-empty value.
				return "PATH=" + rc.Getenv("PATH") + "\nPOSIXLY_CORRECT=y\n", nil
			}
			return "PATH=" + rc.Getenv("PATH") + "\nPOSIXLY_CORRECT=\n", nil
		default:
			t.Fatalf("unexpected probe script: %q", script)
			return "", nil
		}
	}
}

func withFakeShell(t *testing.T, fn func(*tool.RunContext, string, ...string) (string, error)) {
	t.Helper()
	prev := runShellFn
	runShellFn = fn
	t.Cleanup(func() { runShellFn = prev })
}

func TestRuntimeGatePasses(t *testing.T) {
	spec, err := loadSpec()
	if err != nil {
		t.Fatal(err)
	}
	bindir := stageBindir(t, spec)
	rc := runtimeRC(t, "PATH="+bindir, "POSIXLY_CORRECT=1")
	withFakeShell(t, fakeShell(t, spec, nil, true, true))

	if fs := verifyRuntime(rc, spec, runtimeConfig{shell: filepath.Join(bindir, "sh"), binDir: bindir}); len(fs) != 0 {
		t.Errorf("healthy staged runtime rejected: %v", fs)
	}
}

func TestRuntimeGateRejectsHostPathFallback(t *testing.T) {
	spec, err := loadSpec()
	if err != nil {
		t.Fatal(err)
	}
	bindir := stageBindir(t, spec)
	hostdir := t.TempDir()
	if err := os.WriteFile(filepath.Join(hostdir, "sed"), []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	rc := runtimeRC(t, "PATH="+hostdir+string(os.PathListSeparator)+bindir, "POSIXLY_CORRECT=1")
	withFakeShell(t, fakeShell(t, spec, nil, true, true))

	fs := verifyRuntime(rc, spec, runtimeConfig{shell: filepath.Join(bindir, "sh"), binDir: bindir})
	if !findingsHave(fs, "path-owner", "sed", "host PATH fallback") {
		t.Errorf("host-shadowed sed not rejected: %v", fs)
	}
}

func TestRuntimeGateRejectsUnresolvableName(t *testing.T) {
	spec, err := loadSpec()
	if err != nil {
		t.Fatal(err)
	}
	bindir := stageBindir(t, spec)
	if err := os.Remove(filepath.Join(bindir, "make")); err != nil {
		t.Fatal(err)
	}
	rc := runtimeRC(t, "PATH="+bindir, "POSIXLY_CORRECT=1")
	withFakeShell(t, fakeShell(t, spec, nil, true, true))

	fs := verifyRuntime(rc, spec, runtimeConfig{shell: filepath.Join(bindir, "sh"), binDir: bindir})
	if !findingsHave(fs, "path-owner", "make", "not resolvable") {
		t.Errorf("missing staged make not rejected: %v", fs)
	}
}

func TestRuntimeGateRejectsWrongEffectiveOwnerInShell(t *testing.T) {
	spec, err := loadSpec()
	if err != nil {
		t.Fatal(err)
	}
	bindir := stageBindir(t, spec)
	rc := runtimeRC(t, "PATH="+bindir, "POSIXLY_CORRECT=1")
	// time coming back as a file means the keyword is not the effective owner;
	// cd coming back as a file means the shell lost a builtin to the PATH.
	withFakeShell(t, fakeShell(t, spec, map[string]string{"time": "file", "cd": "file"}, true, true))

	fs := verifyRuntime(rc, spec, runtimeConfig{shell: filepath.Join(bindir, "sh"), binDir: bindir})
	if !findingsHave(fs, "shell-owner", "time", `requires "keyword"`) {
		t.Errorf("time misclassification not rejected: %v", fs)
	}
	if !findingsHave(fs, "shell-owner", "cd", `requires "builtin"`) {
		t.Errorf("cd misclassification not rejected: %v", fs)
	}
}

func TestRuntimeGateRejectsNonPosixModeShell(t *testing.T) {
	spec, err := loadSpec()
	if err != nil {
		t.Fatal(err)
	}
	bindir := stageBindir(t, spec)
	rc := runtimeRC(t, "PATH="+bindir, "POSIXLY_CORRECT=1")
	withFakeShell(t, fakeShell(t, spec, nil, false, true))

	fs := verifyRuntime(rc, spec, runtimeConfig{shell: filepath.Join(bindir, "sh"), binDir: bindir})
	if !findingsHave(fs, "posix-mode", "", "posix on") {
		t.Errorf("non-POSIX-mode shell not rejected: %v", fs)
	}
}

func TestRuntimeGateRejectsMissingPosixlyCorrect(t *testing.T) {
	spec, err := loadSpec()
	if err != nil {
		t.Fatal(err)
	}
	bindir := stageBindir(t, spec)
	rc := runtimeRC(t, "PATH="+bindir) // no POSIXLY_CORRECT anywhere
	withFakeShell(t, fakeShell(t, spec, nil, true, false))

	fs := verifyRuntime(rc, spec, runtimeConfig{shell: filepath.Join(bindir, "sh"), binDir: bindir})
	if !findingsHave(fs, "posixly-correct", "", "gate's own environment") {
		t.Errorf("unset POSIXLY_CORRECT in own env not rejected: %v", fs)
	}
	if !findingsHave(fs, "posixly-correct", "", "inside the shell") {
		t.Errorf("unset POSIXLY_CORRECT in shell child not rejected: %v", fs)
	}
	if !findingsHave(fs, "posixly-correct-child", "", "exec'd by the shell") {
		t.Errorf("missing POSIXLY_CORRECT in exec'd grandchild not rejected: %v", fs)
	}
}

func TestRuntimeGateSameTarget(t *testing.T) {
	spec, err := loadSpec()
	if err != nil {
		t.Fatal(err)
	}
	bindir := t.TempDir()
	multicall := filepath.Join(t.TempDir(), "coreutils")
	if err := os.WriteFile(multicall, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	other := filepath.Join(t.TempDir(), "impostor")
	if err := os.WriteFile(other, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, r := range spec {
		if !pathOwned(r) {
			continue
		}
		target := multicall
		if r.Command == "sh" {
			target = other // sh is the shell binary, exempt from same-target
		}
		if err := os.Symlink(target, filepath.Join(bindir, r.Command)); err != nil {
			t.Skipf("symlinks unavailable: %v", err)
		}
	}
	rc := runtimeRC(t, "PATH="+bindir, "POSIXLY_CORRECT=1")
	withFakeShell(t, fakeShell(t, spec, nil, true, true))
	cfg := runtimeConfig{shell: filepath.Join(bindir, "sh"), binDir: bindir, sameTarget: true}

	if fs := verifyRuntime(rc, spec, cfg); len(fs) != 0 {
		t.Errorf("single-target staging rejected: %v", fs)
	}

	// Re-point one applet at a second executable: two targets, one rejection.
	link := filepath.Join(bindir, "sed")
	if err := os.Remove(link); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(other, link); err != nil {
		t.Fatal(err)
	}
	if fs := verifyRuntime(rc, spec, cfg); !findingsHave(fs, "path-target", "", "more than one executable") {
		t.Errorf("split targets not rejected: %v", fs)
	}
}

// ---------------------------------------------------------------------------
// the applet surface
// ---------------------------------------------------------------------------

func runGateCmd(t *testing.T, rc *tool.RunContext, args ...string) (int, string, string) {
	t.Helper()
	tl := tool.Lookup("posix-gate")
	if tl == nil {
		t.Fatal("posix-gate is not registered")
	}
	out := rc.Out.(*bytes.Buffer)
	errb := rc.Err.(*bytes.Buffer)
	code := tl.Run(rc, args)
	return code, out.String(), errb.String()
}

func TestGateUsageErrors(t *testing.T) {
	for _, args := range [][]string{
		{},
		{"bogus"},
		{"spec", "extra"},
		{"registry", "extra"},
		{"providers", "extra"},
		{"runtime"},
		{"runtime", "--shell", "/bin/sh"},
		{"runtime", "--bindir", "/x"},
		{"runtime", "--shell", "/bin/sh", "--bindir", "/x", "--wat"},
		{"runtime", "--shell", "/bin/sh", "--bindir", "/x", "--same-target=yes"},
	} {
		rc := runtimeRC(t)
		code, _, stderr := runGateCmd(t, rc, args...)
		if code != 2 || stderr == "" {
			t.Errorf("posix-gate %v = exit %d (stderr %q), want a loud usage error (2)", args, code, stderr)
		}
	}
}

func TestGateSpecSubcommand(t *testing.T) {
	rc := runtimeRC(t)
	code, stdout, stderr := runGateCmd(t, rc, "spec")
	if code != 0 {
		t.Fatalf("exit = %d, stderr = %q", code, stderr)
	}
	lines := strings.Split(strings.TrimRight(stdout, "\n"), "\n")
	if len(lines) != pinTotal+1 {
		t.Errorf("spec printed %d lines, want %d names + 1 summary", len(lines), pinTotal)
	}
	if !strings.Contains(lines[len(lines)-1], "total 116: 86 go_applet, 14 shell, 16 external_provider") {
		t.Errorf("summary line = %q", lines[len(lines)-1])
	}
}

// TestGateRegistrySubcommandFailsClosedHere: this test process has no userland
// registered, and the applet must say so loudly with exit 1.
func TestGateRegistrySubcommandFailsClosedHere(t *testing.T) {
	rc := runtimeRC(t)
	code, stdout, stderr := runGateCmd(t, rc, "registry")
	if code != 1 {
		t.Fatalf("exit = %d, want 1; stdout=%q stderr=%q", code, stdout, stderr)
	}
	if !strings.Contains(stderr, "posix-gate registry: FAIL") || !strings.Contains(stderr, "[ownership]") {
		t.Errorf("stderr does not carry the verdict and causes: %q", stderr)
	}
}

func TestGateProvidersSubcommand(t *testing.T) {
	withLinuxGate(t)
	root := t.TempDir()
	provisionAll(t, root)
	rc := runtimeRC(t, "BASHY_BIN_CACHE="+root)
	code, stdout, stderr := runGateCmd(t, rc, "providers")
	if code != 0 || !strings.Contains(stdout, "posix-gate providers: PASS") {
		t.Errorf("exit = %d, stdout = %q, stderr = %q", code, stdout, stderr)
	}

	// Empty cache: rejection, exit 1.
	rc = runtimeRC(t, "BASHY_BIN_CACHE="+t.TempDir())
	code, _, stderr = runGateCmd(t, rc, "providers")
	if code != 1 || !strings.Contains(stderr, "not provisioned") {
		t.Errorf("empty cache: exit = %d, stderr = %q", code, stderr)
	}
}

// TestGateRuntimeSubcommandEndToEnd drives the full staged verdict through the
// applet: seamed full registry, provisioned temp cache, staged bindir, healthy
// fake shell — then breaks one leg and watches the verdict flip.
func TestGateRuntimeSubcommandEndToEnd(t *testing.T) {
	withLinuxGate(t)
	spec, err := loadSpec()
	if err != nil {
		t.Fatal(err)
	}
	reg := fullRegistry(t)
	prevReg := registeredFn
	registeredFn = func(n string) bool { return reg[n] }
	t.Cleanup(func() { registeredFn = prevReg })

	root := t.TempDir()
	provisionAll(t, root)
	bindir := stageBindir(t, spec)
	withFakeShell(t, fakeShell(t, spec, nil, true, true))
	shell := filepath.Join(bindir, "sh")

	rc := runtimeRC(t, "PATH="+bindir, "POSIXLY_CORRECT=1", "BASHY_BIN_CACHE="+root)
	code, stdout, stderr := runGateCmd(t, rc, "runtime", "--shell", shell, "--bindir", bindir)
	if code != 0 || !strings.Contains(stdout, "posix-gate runtime: PASS") {
		t.Fatalf("healthy runtime: exit = %d, stdout = %q, stderr = %q", code, stdout, stderr)
	}

	// Same staging, but the gate's own environment lost POSIXLY_CORRECT.
	rc = runtimeRC(t, "PATH="+bindir, "BASHY_BIN_CACHE="+root)
	code, _, stderr = runGateCmd(t, rc, "runtime", "--shell", shell, "--bindir", bindir)
	if code != 1 || !strings.Contains(stderr, "posix-gate runtime: FAIL") {
		t.Errorf("degraded runtime: exit = %d, stderr = %q", code, stderr)
	}
}
