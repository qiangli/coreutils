// Copyright (c) 2025 qiangli
// See LICENSE for licensing information

// Hermetic: every test runs against temp directories, a seamed registry view,
// and a seamed shell probe. Nothing here spawns a real shell, downloads,
// compiles, or reaches the real provider cache.
package posixgatecmd

import (
	"bufio"
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

// TestSpecMatchesCanonicalManifest re-derives the projection from the
// canonical expanded POSIX manifest and compares it against the generated
// specRows. scripts/applet-matrix.py owns both; this is the in-Go tripwire
// for a hand edit to either. The overlap sets restated here are the test's
// independent expectation of the availability→effective projection.
func TestSpecMatchesCanonicalManifest(t *testing.T) {
	f, err := os.Open(filepath.Join("..", "..", "docs", "posix-required-commands.tsv"))
	if err != nil {
		t.Fatalf("cannot read the canonical manifest: %v", err)
	}
	defer f.Close()

	overlapBuiltin := map[string]bool{
		"echo": true, "false": true, "kill": true, "printf": true,
		"pwd": true, "test": true, "true": true,
	}
	var want []specRow
	sc := bufio.NewScanner(f)
	line := 0
	for sc.Scan() {
		line++
		if line == 1 {
			if got := sc.Text(); got != "command\tcoreutils_go_applet\tgo_package\tshell_provided\tprofile_cd_disposition" {
				t.Fatalf("canonical manifest header changed: %q", got)
			}
			continue
		}
		fields := strings.Split(strings.TrimRight(sc.Text(), "\r"), "\t")
		if len(fields) != 5 {
			t.Fatalf("canonical manifest line %d: %d columns", line, len(fields))
		}
		r := specRow{Command: fields[0], GoPackage: fields[2]}
		switch fields[4] {
		case "go_applet":
			r.Owner = OwnerGoApplet
			switch {
			case overlapBuiltin[r.Command]:
				r.Effective = SelShellBuiltin
			case r.Command == "time":
				r.Effective = SelShellKeyword
			default:
				r.Effective = SelGoApplet
			}
		case "shell":
			r.Owner = OwnerShell
			if r.Command == "sh" {
				r.Effective = SelShellEntry
			} else {
				r.Effective = SelShellBuiltin
			}
		case "external_provider":
			r.Owner, r.Effective = OwnerProvider, SelProvider
		default:
			t.Fatalf("canonical manifest line %d: unknown disposition %q", line, fields[4])
		}
		want = append(want, r)
	}
	if err := sc.Err(); err != nil {
		t.Fatal(err)
	}
	if len(want) != len(specRows) {
		t.Fatalf("generated projection has %d rows, canonical manifest %d", len(specRows), len(want))
	}
	for i, w := range want {
		if specRows[i] != w {
			t.Errorf("row %d: generated %+v, canonical projects %+v; regenerate with scripts/applet-matrix.py", i, specRows[i], w)
		}
	}
}

// TestSpecPinnedCounts pins BOTH axes: availability 86/14/16 and effective
// selection 78/22/16.
func TestSpecPinnedCounts(t *testing.T) {
	spec, err := loadSpec()
	if err != nil {
		t.Fatal(err)
	}
	avail := map[Owner]int{}
	effGo, effShell, effProv := 0, 0, 0
	for _, r := range spec {
		avail[r.Owner]++
		switch r.Effective {
		case SelGoApplet:
			effGo++
		case SelShellEntry, SelShellBuiltin, SelShellKeyword:
			effShell++
		case SelProvider:
			effProv++
		}
	}
	if len(spec) != 116 || avail[OwnerGoApplet] != 86 || avail[OwnerShell] != 14 || avail[OwnerProvider] != 16 {
		t.Errorf("availability = %d total %v, want 116 split 86/14/16", len(spec), avail)
	}
	if effGo != 78 || effShell != 22 || effProv != 16 {
		t.Errorf("effective selection = %d/%d/%d, want 78/22/16", effGo, effShell, effProv)
	}
	if pinTotal != 116 || pinAvailGoApplets != 86 || pinAvailShell != 14 || pinProviders != 16 ||
		pinEffectiveGoApplets != 78 || pinEffectiveShell != 22 {
		t.Error("pin constants drifted from the documented 116 = 86/14/16 availability, 78/22/16 effective")
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

func TestValidateSpecRejectsCorruptProjections(t *testing.T) {
	row := func(cmd string, o Owner, e Selector) specRow {
		return specRow{Command: cmd, GoPackage: "cmds/" + cmd, Owner: o, Effective: e}
	}
	cases := []struct {
		name string
		rows []specRow
		want string
	}{
		{"empty", nil, "empty"},
		{"empty command", []specRow{row("", OwnerGoApplet, SelGoApplet)}, "empty command"},
		{"duplicate", []specRow{row("cat", OwnerGoApplet, SelGoApplet), row("cat", OwnerGoApplet, SelGoApplet)}, "twice"},
		{"unknown owner", []specRow{row("cat", Owner("host_binary"), SelGoApplet)}, "unknown availability owner"},
		{"unknown selector", []specRow{row("cat", OwnerGoApplet, Selector("magic"))}, "unknown effective selector"},
		{"provider selecting applet", []specRow{row("make", OwnerProvider, SelGoApplet)}, "cannot exist"},
		{"shell name as entry", []specRow{row("cd", OwnerShell, SelShellEntry)}, "cannot exist"},
		{"sh as builtin", []specRow{row("sh", OwnerShell, SelShellBuiltin)}, "cannot exist"},
		{"shell owner selecting applet", []specRow{row("cd", OwnerShell, SelGoApplet)}, "cannot exist"},
	}
	for _, c := range cases {
		if err := validateSpec(c.rows); err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: err = %v, want mention of %q", c.name, err, c.want)
		}
	}
}

// TestExpectedShellClasses pins the effective classification the runtime
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
	// mean a shell builtin silently took over an applet-owned name. This is
	// the entire 86→78 availability→effective difference.
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
	// Effective drift with availability intact: an applet-owned name whose
	// selector flips to shell_builtin (an eighth overlap) keeps 86/14/16 but
	// breaks 78/22 — the effective pins must catch it on their own.
	shifted := make([]specRow, len(spec))
	copy(shifted, spec)
	for i, r := range shifted {
		if r.Owner == OwnerGoApplet && r.Effective == SelGoApplet {
			shifted[i].Effective = SelShellBuiltin
			break
		}
	}
	fs = verifyInventory(shifted, posixprovider.Names())
	if !findingsHave(fs, "count-drift", "", "effective go-applet count is 77") ||
		!findingsHave(fs, "count-drift", "", "effective shell count is 23") {
		t.Errorf("effective-selection drift not rejected: %v", fs)
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

// TestVerifyRegistryRejectsOptOut: the opt-out value observed in the
// environment is a rejection in itself — never a skip, never a downgrade.
func TestVerifyRegistryRejectsOptOut(t *testing.T) {
	if fs := VerifyRegistry("off"); !findingsHave(fs, "opt-out", "", "BASHY_POSIX_PROVIDERS") {
		t.Errorf("opt-out value did not produce the opt-out rejection: %v", fs)
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

// Distinct staged contents: every multicall-owned entry carries the approved
// multicall's bytes (identity is digest equality, so copies and symlinks are
// equally valid staging); the shell is its own executable; the host tool is
// what a bypass would smuggle in.
const (
	multicallBody = "#!/bin/sh\n# staged approved multicall\nexit 0\n"
	shellBody     = "#!/bin/sh\n# staged profile shell\nexit 0\n"
	hostToolBody  = "#!/bin/sh\n# arbitrary host /bin tool\nexit 0\n"
)

const approvedBashLine = "GNU bash, version 5.2.32(1)-release (x86_64-pc-linux-gnu)"

// stageRuntime builds a staged tool directory: the approved multicall, a copy
// of it under every multicall-owned name, and the shell as its own file.
// Returns the bindir and the approved multicall's path.
func stageRuntime(t *testing.T, spec []specRow) (string, string) {
	t.Helper()
	dir := t.TempDir()
	multicall := filepath.Join(dir, "coreutils")
	if err := os.WriteFile(multicall, []byte(multicallBody), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, r := range spec {
		if !pathOwned(r) {
			continue
		}
		body := multicallBody
		if r.Command == "sh" {
			body = shellBody
		}
		if err := os.WriteFile(filepath.Join(dir, r.Command), []byte(body), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	return dir, multicall
}

func runtimeCfg(bindir, multicall string) runtimeConfig {
	return runtimeConfig{shellName: "sh", binDir: bindir, multicall: multicall}
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

// shellSim configures the fake staged shell: what it reports for --version,
// how it classifies names, whether posix mode is on, and whether the exec'd
// grandchild sees POSIXLY_CORRECT. transcriptEdit lets adversarial tests
// mutate the raw classification transcript.
type shellSim struct {
	versionLine    string
	classOverride  map[string]string
	posixOn        bool
	childHasVar    bool
	transcriptEdit func([]string) []string
}

func healthySim() shellSim {
	return shellSim{versionLine: approvedBashLine, posixOn: true, childHasVar: true}
}

// fakeShell answers the gate's probes the way the simulated shell would.
func fakeShell(t *testing.T, spec []specRow, sim shellSim) func(*tool.RunContext, string, ...string) (string, error) {
	t.Helper()
	return func(rc *tool.RunContext, shell string, args ...string) (string, error) {
		if len(args) == 1 && args[0] == "--version" {
			return sim.versionLine + "\n", nil
		}
		if len(args) < 2 || args[0] != "-c" {
			t.Fatalf("unexpected shell invocation: %v", args)
		}
		switch script := args[1]; {
		case script == classifyScript:
			classes := map[string]string{}
			for _, r := range spec {
				classes[r.Command] = expectedShellClass(r)
			}
			maps.Copy(classes, sim.classOverride)
			var lines []string
			for _, n := range args[3:] { // args[2] is $0
				lines = append(lines, n+" "+classes[n])
			}
			if sim.transcriptEdit != nil {
				lines = sim.transcriptEdit(lines)
			}
			return strings.Join(lines, "\n") + "\n", nil
		case script == "set -o":
			state := "on"
			if !sim.posixOn {
				state = "off"
			}
			return "noexec         off\nposix          " + state + "\nverbose        off\n", nil
		case strings.Contains(script, "$POSIXLY_CORRECT"):
			return rc.Getenv("POSIXLY_CORRECT") + "\n", nil
		case script == "exec env":
			if sim.childHasVar {
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
	bindir, multicall := stageRuntime(t, spec)
	rc := runtimeRC(t, "PATH="+bindir, "POSIXLY_CORRECT=1")
	withFakeShell(t, fakeShell(t, spec, healthySim()))

	if fs := verifyRuntime(rc, spec, runtimeCfg(bindir, multicall)); len(fs) != 0 {
		t.Errorf("healthy staged runtime rejected: %v", fs)
	}

	// The externally pinned digest, when given, must accept the real one.
	sum := sha256.Sum256([]byte(multicallBody))
	cfg := runtimeCfg(bindir, multicall)
	cfg.multicallSHA = strings.ToUpper(hex.EncodeToString(sum[:])) // case-insensitive
	if fs := verifyRuntime(rc, spec, cfg); len(fs) != 0 {
		t.Errorf("healthy runtime with matching pinned digest rejected: %v", fs)
	}
}

func TestRuntimeGateRejectsHostPathFallback(t *testing.T) {
	spec, err := loadSpec()
	if err != nil {
		t.Fatal(err)
	}
	bindir, multicall := stageRuntime(t, spec)
	hostdir := t.TempDir()
	if err := os.WriteFile(filepath.Join(hostdir, "sed"), []byte(hostToolBody), 0o755); err != nil {
		t.Fatal(err)
	}
	rc := runtimeRC(t, "PATH="+hostdir+string(os.PathListSeparator)+bindir, "POSIXLY_CORRECT=1")
	withFakeShell(t, fakeShell(t, spec, healthySim()))

	fs := verifyRuntime(rc, spec, runtimeCfg(bindir, multicall))
	if !findingsHave(fs, "path-owner", "sed", "host PATH fallback") {
		t.Errorf("host-shadowed sed not rejected: %v", fs)
	}
}

func TestRuntimeGateRejectsUnresolvableName(t *testing.T) {
	spec, err := loadSpec()
	if err != nil {
		t.Fatal(err)
	}
	bindir, multicall := stageRuntime(t, spec)
	if err := os.Remove(filepath.Join(bindir, "make")); err != nil {
		t.Fatal(err)
	}
	rc := runtimeRC(t, "PATH="+bindir, "POSIXLY_CORRECT=1")
	withFakeShell(t, fakeShell(t, spec, healthySim()))

	fs := verifyRuntime(rc, spec, runtimeCfg(bindir, multicall))
	if !findingsHave(fs, "path-owner", "make", "not resolvable") {
		t.Errorf("missing staged make not rejected: %v", fs)
	}
}

// TestRuntimeGateRejectsStagedSymlinkToHostTool: the central identity bypass —
// an entry INSIDE the staged directory that is a symlink to an arbitrary host
// tool. Directory membership passes; digest identity must not.
func TestRuntimeGateRejectsStagedSymlinkToHostTool(t *testing.T) {
	spec, err := loadSpec()
	if err != nil {
		t.Fatal(err)
	}
	bindir, multicall := stageRuntime(t, spec)
	hostTool := filepath.Join(t.TempDir(), "ls")
	if err := os.WriteFile(hostTool, []byte(hostToolBody), 0o755); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(bindir, "ls")
	if err := os.Remove(link); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(hostTool, link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	rc := runtimeRC(t, "PATH="+bindir, "POSIXLY_CORRECT=1")
	withFakeShell(t, fakeShell(t, spec, healthySim()))

	fs := verifyRuntime(rc, spec, runtimeCfg(bindir, multicall))
	if !findingsHave(fs, "exec-identity", "ls", "not the approved multicall") {
		t.Errorf("staged symlink to a host tool not rejected: %v", fs)
	}
}

// TestRuntimeGateRejectsForeignExecutableInBindir: same bypass without the
// symlink — a foreign binary copied over a staged name (runs on every
// platform, symlinks or not).
func TestRuntimeGateRejectsForeignExecutableInBindir(t *testing.T) {
	spec, err := loadSpec()
	if err != nil {
		t.Fatal(err)
	}
	bindir, multicall := stageRuntime(t, spec)
	if err := os.WriteFile(filepath.Join(bindir, "awk"), []byte(hostToolBody), 0o755); err != nil {
		t.Fatal(err)
	}
	rc := runtimeRC(t, "PATH="+bindir, "POSIXLY_CORRECT=1")
	withFakeShell(t, fakeShell(t, spec, healthySim()))

	fs := verifyRuntime(rc, spec, runtimeCfg(bindir, multicall))
	if !findingsHave(fs, "exec-identity", "awk", "not the approved multicall") {
		t.Errorf("foreign executable under a staged name not rejected: %v", fs)
	}
}

func TestRuntimeGateRejectsUnapprovedMulticall(t *testing.T) {
	spec, err := loadSpec()
	if err != nil {
		t.Fatal(err)
	}
	bindir, multicall := stageRuntime(t, spec)
	rc := runtimeRC(t, "PATH="+bindir, "POSIXLY_CORRECT=1")
	withFakeShell(t, fakeShell(t, spec, healthySim()))

	// The named multicall does not exist: identity cannot be established.
	cfg := runtimeCfg(bindir, filepath.Join(bindir, "no-such-multicall"))
	if fs := verifyRuntime(rc, spec, cfg); !findingsHave(fs, "approved-multicall", "", "not a file") {
		t.Errorf("missing approved multicall not rejected: %v", fs)
	}

	// The externally pinned digest disagrees with the staged binary.
	cfg = runtimeCfg(bindir, multicall)
	cfg.multicallSHA = strings.Repeat("0", 64)
	if fs := verifyRuntime(rc, spec, cfg); !findingsHave(fs, "approved-multicall", "", "not the approved digest") {
		t.Errorf("digest-mismatched multicall not rejected: %v", fs)
	}
}

func TestRuntimeGateRejectsHostPathShell(t *testing.T) {
	spec, err := loadSpec()
	if err != nil {
		t.Fatal(err)
	}
	bindir, multicall := stageRuntime(t, spec)
	hostdir := t.TempDir()
	if err := os.WriteFile(filepath.Join(hostdir, "sh"), []byte(hostToolBody), 0o755); err != nil {
		t.Fatal(err)
	}
	rc := runtimeRC(t, "PATH="+hostdir+string(os.PathListSeparator)+bindir, "POSIXLY_CORRECT=1")
	withFakeShell(t, fakeShell(t, spec, healthySim()))

	fs := verifyRuntime(rc, spec, runtimeCfg(bindir, multicall))
	if !findingsHave(fs, "shell-path", "sh", "host PATH shell") {
		t.Errorf("shell resolved from the host PATH not rejected: %v", fs)
	}

	// No shell anywhere on the staged PATH: also a rejection, never a probe of
	// some implicit fallback.
	if err := os.Remove(filepath.Join(bindir, "sh")); err != nil {
		t.Fatal(err)
	}
	rc = runtimeRC(t, "PATH="+bindir, "POSIXLY_CORRECT=1")
	fs = verifyRuntime(rc, spec, runtimeCfg(bindir, multicall))
	if !findingsHave(fs, "shell-path", "sh", "not resolvable") {
		t.Errorf("unresolvable shell not rejected: %v", fs)
	}
}

func TestRuntimeGateRejectsUnapprovedShellIdentity(t *testing.T) {
	spec, err := loadSpec()
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name, line, want string
	}{
		{"foreign shell", "zsh 5.9 (x86_64-apple-darwin24.0)", "does not identify an approved"},
		{"garbage", "hello world", "does not identify an approved"},
		{"empty", "", "does not identify an approved"},
		{"bash 4", "GNU bash, version 4.4.20(1)-release (x86_64-pc-linux-gnu)", "require a bash-5-family shell"},
		{"blank build", "GNU bash, version 5.2.32(1)-release ( )", "no build identifier"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			bindir, multicall := stageRuntime(t, spec)
			rc := runtimeRC(t, "PATH="+bindir, "POSIXLY_CORRECT=1")
			sim := healthySim()
			sim.versionLine = c.line
			withFakeShell(t, fakeShell(t, spec, sim))
			fs := verifyRuntime(rc, spec, runtimeCfg(bindir, multicall))
			if !findingsHave(fs, "shell-identity", "", c.want) {
				t.Errorf("version line %q not rejected with %q: %v", c.line, c.want, fs)
			}
		})
	}

	// The approved bashy line must pass (Profile D).
	bindir, multicall := stageRuntime(t, spec)
	rc := runtimeRC(t, "PATH="+bindir, "POSIXLY_CORRECT=1")
	sim := healthySim()
	sim.versionLine = "bashy, GNU Bash 5.3 compatible, version 5.3.0(1)-bashy-dev (a0a0315)"
	withFakeShell(t, fakeShell(t, spec, sim))
	if fs := verifyRuntime(rc, spec, runtimeCfg(bindir, multicall)); len(fs) != 0 {
		t.Errorf("approved bashy identity rejected: %v", fs)
	}
}

func TestRuntimeGateRejectsWrongEffectiveOwnerInShell(t *testing.T) {
	spec, err := loadSpec()
	if err != nil {
		t.Fatal(err)
	}
	bindir, multicall := stageRuntime(t, spec)
	rc := runtimeRC(t, "PATH="+bindir, "POSIXLY_CORRECT=1")
	// time coming back as a file means the keyword is not the effective owner;
	// cd coming back as a file means the shell lost a builtin to the PATH.
	sim := healthySim()
	sim.classOverride = map[string]string{"time": "file", "cd": "file"}
	withFakeShell(t, fakeShell(t, spec, sim))

	fs := verifyRuntime(rc, spec, runtimeCfg(bindir, multicall))
	if !findingsHave(fs, "shell-owner", "time", `requires "keyword"`) {
		t.Errorf("time misclassification not rejected: %v", fs)
	}
	if !findingsHave(fs, "shell-owner", "cd", `requires "builtin"`) {
		t.Errorf("cd misclassification not rejected: %v", fs)
	}
}

// TestRuntimeGateStrictTranscript: the classification transcript must account
// for exactly the 116 expected names — duplicate rows, extra names, missing
// rows, and malformed rows are each their own rejection.
func TestRuntimeGateStrictTranscript(t *testing.T) {
	spec, err := loadSpec()
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name  string
		edit  func([]string) []string
		fname string
		want  string
	}{
		{"duplicate row", func(l []string) []string { return append(l, l[0]) }, "alias", "duplicate classification row"},
		{"extra name", func(l []string) []string { return append(l, "cpio file") }, "cpio", "outside the 116-name inventory"},
		{"missing row", func(l []string) []string { return l[1:] }, "alias", "no classification row"},
		{"malformed row", func(l []string) []string { l[0] = "alias"; return l }, "", "malformed classification row"},
		{"unknown class", func(l []string) []string { l[0] = "alias wizard"; return l }, "", "malformed classification row"},
		{"trailing junk", func(l []string) []string { l[0] = "alias builtin extra"; return l }, "", "malformed classification row"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			bindir, multicall := stageRuntime(t, spec)
			rc := runtimeRC(t, "PATH="+bindir, "POSIXLY_CORRECT=1")
			sim := healthySim()
			sim.transcriptEdit = c.edit
			withFakeShell(t, fakeShell(t, spec, sim))
			fs := verifyRuntime(rc, spec, runtimeCfg(bindir, multicall))
			if !findingsHave(fs, "transcript", c.fname, c.want) {
				t.Errorf("%s not rejected with %q: %v", c.name, c.want, fs)
			}
		})
	}

	// A transcript short of 116 unique expected names also carries the
	// explicit count rejection.
	bindir, multicall := stageRuntime(t, spec)
	rc := runtimeRC(t, "PATH="+bindir, "POSIXLY_CORRECT=1")
	sim := healthySim()
	sim.transcriptEdit = func(l []string) []string { return l[2:] }
	withFakeShell(t, fakeShell(t, spec, sim))
	fs := verifyRuntime(rc, spec, runtimeCfg(bindir, multicall))
	if !findingsHave(fs, "transcript", "", "114 unique expected names, want exactly 116") {
		t.Errorf("short transcript did not carry the count rejection: %v", fs)
	}
}

func TestRuntimeGateRejectsNonPosixModeShell(t *testing.T) {
	spec, err := loadSpec()
	if err != nil {
		t.Fatal(err)
	}
	bindir, multicall := stageRuntime(t, spec)
	rc := runtimeRC(t, "PATH="+bindir, "POSIXLY_CORRECT=1")
	sim := healthySim()
	sim.posixOn = false
	withFakeShell(t, fakeShell(t, spec, sim))

	fs := verifyRuntime(rc, spec, runtimeCfg(bindir, multicall))
	if !findingsHave(fs, "posix-mode", "", "posix on") {
		t.Errorf("non-POSIX-mode shell not rejected: %v", fs)
	}
}

func TestRuntimeGateRejectsMissingPosixlyCorrect(t *testing.T) {
	spec, err := loadSpec()
	if err != nil {
		t.Fatal(err)
	}
	bindir, multicall := stageRuntime(t, spec)
	rc := runtimeRC(t, "PATH="+bindir) // no POSIXLY_CORRECT anywhere
	sim := healthySim()
	sim.childHasVar = false
	withFakeShell(t, fakeShell(t, spec, sim))

	fs := verifyRuntime(rc, spec, runtimeCfg(bindir, multicall))
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
		{"runtime", "--bindir", "/x"},                             // no --multicall
		{"runtime", "--multicall", "/x/coreutils"},                // no --bindir
		{"runtime", "--bindir", "/x", "--multicall", "/x/c", "--wat"},
		{"runtime", "--bindir", "/x", "--multicall", "/x/c", "--shell", "/bin/sh"},        // a PATH, not a name
		{"runtime", "--bindir", "/x", "--multicall", "/x/c", "--multicall-sha256", "abc"}, // not 64 hex
		{"runtime", "--bindir", "/x", "--multicall", "/x/c", "--same-target"},             // removed option
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
	if len(lines) != pinTotal+2 {
		t.Errorf("spec printed %d lines, want %d names + 2 summary lines", len(lines), pinTotal)
	}
	if !strings.Contains(stdout, "availability 86 go_applet, 14 shell, 16 external_provider") {
		t.Errorf("availability summary missing from %q", stdout)
	}
	if !strings.Contains(stdout, "effective selection: 78 go_applet, 22 shell, 16 external_provider") {
		t.Errorf("effective-selection summary missing from %q", stdout)
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
// fake shell — then breaks one leg at a time and watches the verdict flip.
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
	bindir, multicall := stageRuntime(t, spec)
	withFakeShell(t, fakeShell(t, spec, healthySim()))

	rc := runtimeRC(t, "PATH="+bindir, "POSIXLY_CORRECT=1", "BASHY_BIN_CACHE="+root)
	code, stdout, stderr := runGateCmd(t, rc, "runtime", "--bindir", bindir, "--multicall", multicall)
	if code != 0 || !strings.Contains(stdout, "posix-gate runtime: PASS") {
		t.Fatalf("healthy runtime: exit = %d, stdout = %q, stderr = %q", code, stdout, stderr)
	}

	// Provider provenance must be bound to the STAGED wrapper's dispatch
	// cache: an environment that does not name it is a rejection, never a
	// fall-back to the gate process's own default cache.
	rc = runtimeRC(t, "PATH="+bindir, "POSIXLY_CORRECT=1")
	code, _, stderr = runGateCmd(t, rc, "runtime", "--bindir", bindir, "--multicall", multicall)
	if code != 1 || !strings.Contains(stderr, "[provider-cache]") {
		t.Errorf("unbound provider cache: exit = %d, stderr = %q", code, stderr)
	}

	// Same staging, but the gate's own environment lost POSIXLY_CORRECT.
	rc = runtimeRC(t, "PATH="+bindir, "BASHY_BIN_CACHE="+root)
	code, _, stderr = runGateCmd(t, rc, "runtime", "--bindir", bindir, "--multicall", multicall)
	if code != 1 || !strings.Contains(stderr, "posix-gate runtime: FAIL") {
		t.Errorf("degraded runtime: exit = %d, stderr = %q", code, stderr)
	}
}
