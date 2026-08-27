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

// TestSpecPinnedCounts pins BOTH axes: availability 92/14/10 and effective
// selection 84/22/10.
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
	if len(spec) != 116 || avail[OwnerGoApplet] != 92 || avail[OwnerShell] != 14 || avail[OwnerProvider] != 10 {
		t.Errorf("availability = %d total %v, want 116 split 92/14/10", len(spec), avail)
	}
	if effGo != 84 || effShell != 22 || effProv != 10 {
		t.Errorf("effective selection = %d/%d/%d, want 84/22/10", effGo, effShell, effProv)
	}
	if pinTotal != 116 || pinAvailGoApplets != 92 || pinAvailShell != 14 || pinProviders != 10 ||
		pinEffectiveGoApplets != 84 || pinEffectiveShell != 22 || pinManifestProviders != 10 {
		t.Error("pin constants drifted from the documented 116 = 92/14/10 availability, 84/22/10 effective, 10 manifest-pinned")
	}
}

// TestSpecProvidersMatchManifest: the inventory's external_provider rows and
// the active dispatch-provider set must be the same set, both directions.
func TestSpecProvidersMatchManifest(t *testing.T) {
	spec, err := loadSpec()
	if err != nil {
		t.Fatal(err)
	}
	if fs := verifyInventory(spec, posixprovider.DispatchNames()); len(fs) != 0 {
		for _, f := range fs {
			t.Errorf("%s", f)
		}
	}
}

func TestGoAppletReplacementsAreNotProviders(t *testing.T) {
	spec, err := loadSpec()
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{
		"ed": "cmds/ed", "mailx": "cmds/mailx", "patch": "cmds/patch", "talk": "cmds/talk",
	}
	for _, row := range spec {
		pkg, ok := want[row.Command]
		if !ok {
			continue
		}
		if row.Owner != OwnerGoApplet || row.Effective != SelGoApplet || row.GoPackage != pkg {
			t.Errorf("%s = owner %s selector %s package %s; want Go applet %s", row.Command, row.Owner, row.Effective, row.GoPackage, pkg)
		}
		delete(want, row.Command)
	}
	if len(want) != 0 {
		t.Errorf("replacement applets missing from Profile C/D spec: %v", want)
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
		{"provider selecting applet", []specRow{row("m4", OwnerProvider, SelGoApplet)}, "cannot exist"},
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
		"sed": "file", "ls": "file", "m4": "file", "vi": "file",
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
	// the entire 87→79 availability→effective difference.
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
	fs := verifyInventory(spec[:len(spec)-1], posixprovider.DispatchNames())
	if !findingsHave(fs, "count-drift", "", "pinned") {
		t.Errorf("dropped name produced no count-drift finding: %v", fs)
	}
	// Effective drift with availability intact: an applet-owned name whose
	// selector flips to shell_builtin keeps 92/14/10 but breaks 82/22 — the
	// effective pins must catch it on their own.
	shifted := make([]specRow, len(spec))
	copy(shifted, spec)
	for i, r := range shifted {
		if r.Owner == OwnerGoApplet && r.Effective == SelGoApplet {
			shifted[i].Effective = SelShellBuiltin
			break
		}
	}
	fs = verifyInventory(shifted, posixprovider.DispatchNames())
	if !findingsHave(fs, "count-drift", "", "effective go-applet count is 83") ||
		!findingsHave(fs, "count-drift", "", "effective shell count is 23") {
		t.Errorf("effective-selection drift not rejected: %v", fs)
	}
	// Manifest pins a name the inventory does not list, and vice versa.
	fs = verifyInventory(spec, append(posixprovider.DispatchNames(), "cpio"))
	if !findingsHave(fs, "provider-set", "cpio", "not external_provider") {
		t.Errorf("extra manifest pin not rejected: %v", fs)
	}
	var withoutMake []string
	for _, n := range posixprovider.DispatchNames() {
		if n != "m4" {
			withoutMake = append(withoutMake, n)
		}
	}
	fs = verifyInventory(spec, withoutMake)
	if !findingsHave(fs, "provider-set", "m4", "does not pin it") {
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

	if fs := verifyOwnership(spec, lookup, posixprovider.IsDispatchProvider, false); len(fs) != 0 {
		t.Fatalf("complete registry rejected: %v", fs)
	}

	// A missing applet is a name the host would silently supply.
	reg["ls"] = false
	if fs := verifyOwnership(spec, lookup, posixprovider.IsDispatchProvider, false); !findingsHave(fs, "ownership", "ls", "not in the tool registry") {
		t.Errorf("unregistered applet not rejected")
	}
	reg["ls"] = true

	// A missing provider name, same failure class.
	reg["m4"] = false
	if fs := verifyOwnership(spec, lookup, posixprovider.IsDispatchProvider, false); !findingsHave(fs, "ownership", "m4", "not in the tool registry") {
		t.Errorf("unregistered provider not rejected")
	}
	reg["m4"] = true

	// A registered tool under a shell-owned name is ambiguous ownership.
	reg["cd"] = true
	if fs := verifyOwnership(spec, lookup, posixprovider.IsDispatchProvider, false); !findingsHave(fs, "ownership", "cd", "ambiguous") {
		t.Errorf("shell-name shadowing not rejected")
	}
	reg["cd"] = false

	// An applet that is also pinned as a provider is ambiguous ownership.
	hasPlus := func(n string) bool { return n == "sed" || posixprovider.Has(n) }
	if fs := verifyOwnership(spec, lookup, hasPlus, false); !findingsHave(fs, "ownership", "sed", "ambiguous") {
		t.Errorf("applet/provider double claim not rejected")
	}

	// The provider opt-out is a hard failure for a certification runtime.
	if fs := verifyOwnership(spec, lookup, posixprovider.IsDispatchProvider, true); !findingsHave(fs, "opt-out", "", "BASHY_POSIX_PROVIDERS") {
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
	if fs := VerifyProviders(r); !findingsHave(fs, "provider", "m4", "not provisioned") {
		t.Errorf("empty cache not rejected: %v", fs)
	}

	provisionAll(t, root)
	if fs := VerifyProviders(r); len(fs) != 0 {
		t.Errorf("provisioned cache rejected: %v", fs)
	}

	// A platform a manifest row does not declare is a FAILURE, not a skip: a
	// runtime that cannot supply all ten active names is not the claimed runtime.
	fs := VerifyProviders(posixprovider.Resolver{CacheRoot: root, GOOS: "windows"})
	if !findingsHave(fs, "provider", "man", "not declared for windows") {
		t.Errorf("undeclared platform not rejected: %v", fs)
	}

	// A binary that no longer matches its provenance is unattributable.
	e, _ := posixprovider.Lookup("m4")
	bin := filepath.Join(root, e.Command, e.Version, e.Command)
	if err := os.WriteFile(bin, []byte("tampered"), 0o755); err != nil {
		t.Fatal(err)
	}
	if fs := VerifyProviders(r); !findingsHave(fs, "provider", "m4", "provenance") {
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

// The approved Profile C/D version lines. BOTH are the stock GNU Bash line —
// Bashy is a bash-5.3 drop-in, so its staged shell reports "GNU bash,
// version 5.3.0(1)-bashy-dev (a0a0315)" (this is what a real `bash --version`
// under Bashy prints), distinguished from stock only by the -bashy- release
// flavor. inventedBashyBanner is the bashy FRONT-DOOR command's banner: it is
// real output of `bashy --version`, but it is not a shell version line and a
// gate that accepts it is classifying the wrong executable.
const (
	approvedBashLine    = "GNU bash, version 5.3.8(1)-release (x86_64-pc-linux-gnu)"
	approvedBashyLine   = "GNU bash, version 5.3.0(1)-bashy-dev (a0a0315)"
	inventedBashyBanner = "bashy, GNU Bash 5.3 compatible, version 5.3.0(1)-bashy-dev (a0a0315)"
)

func sha256Hex(body string) string {
	sum := sha256.Sum256([]byte(body))
	return hex.EncodeToString(sum[:])
}

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

// runtimeCfg models the config runRuntime would assemble from a correct
// external build manifest: profile C, with the manifest recording exactly the
// staged bodies' digests. Adversarial tests then bend one field at a time.
func runtimeCfg(bindir, multicall string) runtimeConfig {
	return runtimeConfig{
		profile:      "C",
		shellName:    "sh",
		binDir:       bindir,
		multicall:    multicall,
		shellSHA:     sha256Hex(shellBody),
		multicallSHA: sha256Hex(multicallBody),
	}
}

// dispatchPlanFor renders the healthy dispatch plan the approved multicall
// would disclose for a provisioned cache root: one strict TSV row per active
// provider, matching the gate's own verified identities exactly.
func dispatchPlanFor(t *testing.T, root string) string {
	t.Helper()
	r := posixprovider.Resolver{CacheRoot: root, GOOS: "linux"}
	var b strings.Builder
	for _, e := range posixprovider.DispatchEntries() {
		id, err := r.VerifiedIdentity(e.Command)
		if err != nil {
			t.Fatalf("healthy plan for %s: %v", e.Command, err)
		}
		fmt.Fprintf(&b, "%s\t%s\t%s\t%s\n", id.Command, id.Version, id.Path, id.BuiltSHA256)
	}
	return b.String()
}

// withFakePlan seams the trusted multicall introspection probe with a canned
// dispatch plan (or a probe failure).
func withFakePlan(t *testing.T, plan string, err error) {
	t.Helper()
	prev := runMulticallFn
	runMulticallFn = func(rc *tool.RunContext, exe string, args ...string) (string, error) {
		if len(args) != 2 || args[0] != "posix-providers" || args[1] != "dispatch-plan" {
			t.Fatalf("unexpected multicall probe: %v", args)
		}
		return plan, err
	}
	t.Cleanup(func() { runMulticallFn = prev })
}

// stageCertified assembles the complete healthy staged runtime — bindir,
// provisioned provider cache, linux gate view, healthy shell and dispatch-plan
// probes — and returns the RunContext, config, and cache root. The base for
// the full-pass tests and for adversarial tests that break exactly one leg.
func stageCertified(t *testing.T, spec []specRow, sim shellSim) (*tool.RunContext, runtimeConfig, string) {
	t.Helper()
	withLinuxGate(t)
	bindir, multicall := stageRuntime(t, spec)
	root := t.TempDir()
	provisionAll(t, root)
	withFakeShell(t, fakeShell(t, spec, sim))
	withFakePlan(t, dispatchPlanFor(t, root), nil)
	rc := runtimeRC(t, "PATH="+bindir, "POSIXLY_CORRECT=1", "BASHY_BIN_CACHE="+root)
	return rc, runtimeCfg(bindir, multicall), root
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
	// Profile C: approved stock GNU Bash 5.3.
	rc, cfg, _ := stageCertified(t, spec, healthySim())
	if fs := verifyRuntime(rc, spec, cfg); len(fs) != 0 {
		t.Errorf("healthy staged Profile C runtime rejected: %v", fs)
	}

	// Manifest digests are compared case-insensitively (hex is hex).
	upper := cfg
	upper.shellSHA = strings.ToUpper(upper.shellSHA)
	upper.multicallSHA = strings.ToUpper(upper.multicallSHA)
	if fs := verifyRuntime(rc, spec, upper); len(fs) != 0 {
		t.Errorf("healthy runtime with upper-case manifest digests rejected: %v", fs)
	}
}

// TestRuntimeGatePassesProfileD: the same staging certified as Profile D must
// pass with the approved Bashy 5.3 identity — and ONLY with it.
func TestRuntimeGatePassesProfileD(t *testing.T) {
	spec, err := loadSpec()
	if err != nil {
		t.Fatal(err)
	}
	sim := healthySim()
	sim.versionLine = approvedBashyLine
	rc, cfg, _ := stageCertified(t, spec, sim)
	cfg.profile = "D"
	if fs := verifyRuntime(rc, spec, cfg); len(fs) != 0 {
		t.Errorf("healthy staged Profile D runtime rejected: %v", fs)
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
	if err := os.Remove(filepath.Join(bindir, "m4")); err != nil {
		t.Fatal(err)
	}
	rc := runtimeRC(t, "PATH="+bindir, "POSIXLY_CORRECT=1")
	withFakeShell(t, fakeShell(t, spec, healthySim()))

	fs := verifyRuntime(rc, spec, runtimeCfg(bindir, multicall))
	if !findingsHave(fs, "path-owner", "m4", "not resolvable") {
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

	// The manifest's approved digest disagrees with the staged binary: the
	// staged binary is not the approved build, no matter where it sits.
	cfg = runtimeCfg(bindir, multicall)
	cfg.multicallSHA = strings.Repeat("0", 64)
	if fs := verifyRuntime(rc, spec, cfg); !findingsHave(fs, "approved-multicall", "", "not the approved build manifest's") {
		t.Errorf("digest-mismatched multicall not rejected: %v", fs)
	}

	// No manifest digest at all: identity is never derivable from the staged
	// binary itself, so a direct caller with no pin gets a rejection, not a
	// self-certifying digest.
	cfg = runtimeCfg(bindir, multicall)
	cfg.multicallSHA = ""
	if fs := verifyRuntime(rc, spec, cfg); !findingsHave(fs, "approved-multicall", "", "identity cannot be established") {
		t.Errorf("missing manifest digest not rejected: %v", fs)
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

// TestRuntimeGateRejectsUnapprovedShellIdentity: even a shell whose BUILD
// digest matches the manifest must also SAY the right thing for the profile —
// GNU bash exactly version 5.3, the profile's approved release flavor, and a
// build identifier. 5.2 and 5.4 are neighboring releases, not the certified
// one; a flavor approved for one profile is not approved for the other; and a
// "bashy," banner never identifies a shell at all. Every case here models the
// digests PASSING (the manifest pins exactly the staged bytes) — including
// "Bashy build under profile C", the accidentally-pinned-Bashy-bytes case:
// the flavor cross-check must still reject even though the digest agrees.
func TestRuntimeGateRejectsUnapprovedShellIdentity(t *testing.T) {
	spec, err := loadSpec()
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name, profile, line, want string
	}{
		{"foreign shell", "C", "zsh 5.9 (x86_64-apple-darwin24.0)", "not a GNU Bash version line"},
		{"garbage", "C", "hello world", "not a GNU Bash version line"},
		{"empty", "C", "", "not a GNU Bash version line"},
		// The invented "bashy, GNU Bash …" prefix is the bashy front-door
		// command's banner, not any shell's --version line: rejected under
		// BOTH profiles, even though every digit in it is version-correct.
		{"invented bashy banner under D", "D", inventedBashyBanner, "not a GNU Bash version line"},
		{"invented bashy banner under C", "C", inventedBashyBanner, "not a GNU Bash version line"},
		{"bash 4", "C", "GNU bash, version 4.4.20(1)-release (x86_64-pc-linux-gnu)", "exactly version 5.3"},
		{"bash 5.2", "C", "GNU bash, version 5.2.32(1)-release (x86_64-pc-linux-gnu)", "exactly version 5.3"},
		{"bash 5.4", "C", "GNU bash, version 5.4.0(1)-release (x86_64-pc-linux-gnu)", "exactly version 5.3"},
		{"bashy 5.2", "D", "GNU bash, version 5.2.1(1)-bashy-dev (a0a0315)", "exactly version 5.3"},
		{"bashy 5.4", "D", "GNU bash, version 5.4.0(1)-bashy-dev (a0a0315)", "exactly version 5.3"},
		// Cross-profile flavor rejections: Profile C accidentally pinning a
		// Bashy build (digest passes, flavor says -bashy-) and Profile D
		// pinning a stock -release build.
		{"Bashy build under profile C", "C", approvedBashyLine, "profile C requires stock GNU Bash 5.3"},
		{"stock GNU bash under profile D", "D", approvedBashLine, "profile D requires Bashy 5.3"},
		// Flavor mismatches inside each profile: a non-release stock flavor
		// is not the approved stock build, a bare -bashy flavor with no
		// build revision does not identify a Bashy build, and a Bashy
		// marker buried elsewhere in the flavor is not a leading marker.
		{"beta flavor under C", "C", "GNU bash, version 5.3.8(1)-beta (x86_64-pc-linux-gnu)", "approved stock flavor is -release"},
		{"maint flavor under C", "C", "GNU bash, version 5.3.8(1)-maint (x86_64-pc-linux-gnu)", "approved stock flavor is -release"},
		{"bare bashy flavor under D", "D", "GNU bash, version 5.3.0(1)-bashy (a0a0315)", "profile D requires Bashy 5.3"},
		{"trailing bashy marker under D", "D", "GNU bash, version 5.3.0(1)-release-bashy (a0a0315)", "profile D requires Bashy 5.3"},
		{"blank build", "C", "GNU bash, version 5.3.8(1)-release ( )", "no build identifier"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			bindir, multicall := stageRuntime(t, spec)
			rc := runtimeRC(t, "PATH="+bindir, "POSIXLY_CORRECT=1")
			sim := healthySim()
			sim.versionLine = c.line
			withFakeShell(t, fakeShell(t, spec, sim))
			cfg := runtimeCfg(bindir, multicall)
			cfg.profile = c.profile
			fs := verifyRuntime(rc, spec, cfg)
			if !findingsHave(fs, "shell-identity", "", c.want) {
				t.Errorf("version line %q not rejected with %q: %v", c.line, c.want, fs)
			}
		})
	}
}

// TestRuntimeGateRejectsWrongShellBuild: a --version line is forgeable, so a
// staged shell whose reported identity is perfect but whose bytes do not hash
// to the manifest's approved shell build is rejected BEFORE any of its answers
// are trusted — no classification, POSIX-mode, or propagation probes run.
func TestRuntimeGateRejectsWrongShellBuild(t *testing.T) {
	spec, err := loadSpec()
	if err != nil {
		t.Fatal(err)
	}
	bindir, multicall := stageRuntime(t, spec)
	// A perfectly claiming shell that is not the approved build.
	if err := os.WriteFile(filepath.Join(bindir, "sh"), []byte("#!/bin/sh\n# forged shell\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	rc := runtimeRC(t, "PATH="+bindir, "POSIXLY_CORRECT=1")
	withFakeShell(t, fakeShell(t, spec, healthySim()))

	fs := verifyRuntime(rc, spec, runtimeCfg(bindir, multicall))
	if !findingsHave(fs, "shell-build", "", "not the approved profile C shell build") {
		t.Errorf("digest-mismatched shell not rejected: %v", fs)
	}
	for _, f := range fs {
		if f.Check == "shell-owner" || f.Check == "posix-mode" {
			t.Errorf("probe ran against an unproven shell: %v", f)
		}
	}

	// And a config with no shell digest at all cannot certify: the staged
	// shell must never become its own root of trust.
	cfg := runtimeCfg(bindir, multicall)
	cfg.shellSHA = ""
	fs = verifyRuntime(rc, spec, cfg)
	if !findingsHave(fs, "shell-build", "", "shell identity cannot be established") {
		t.Errorf("missing shell digest not rejected: %v", fs)
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
// provider dispatch binding
// ---------------------------------------------------------------------------

// editPlan applies a per-line edit to the healthy dispatch plan.
func editPlan(plan string, edit func([]string) []string) string {
	lines := strings.Split(strings.TrimSuffix(plan, "\n"), "\n")
	return strings.Join(edit(lines), "\n") + "\n"
}

// TestRuntimeGateBindsProviderDispatch: a verified cache is necessary but not
// sufficient — the staged wrapper must PROVE it dispatches to exactly the
// verified binaries. The central bypass: a valid, fully provisioned cache
// sitting unused while the wrapper would dispatch an arbitrary staged
// executable. The plan disclosing that executable (or failing to account for
// every provider) is a rejection.
func TestRuntimeGateBindsProviderDispatch(t *testing.T) {
	spec, err := loadSpec()
	if err != nil {
		t.Fatal(err)
	}

	t.Run("unused valid cache, arbitrary staged executable", func(t *testing.T) {
		rc, cfg, root := stageCertified(t, spec, healthySim())
		// The wrapper would dispatch `m4` to an arbitrary executable while
		// the valid cache sits unused.
		rogue := filepath.Join(t.TempDir(), "m4")
		if err := os.WriteFile(rogue, []byte(hostToolBody), 0o755); err != nil {
			t.Fatal(err)
		}
		withFakePlan(t, editPlan(dispatchPlanFor(t, root), func(l []string) []string {
			for i, line := range l {
				if strings.HasPrefix(line, "m4\t") {
					f := strings.Split(line, "\t")
					l[i] = strings.Join([]string{f[0], f[1], rogue, sha256Hex(hostToolBody)}, "\t")
				}
			}
			return l
		}), nil)
		fs := verifyRuntime(rc, spec, cfg)
		if !findingsHave(fs, "provider-dispatch", "m4", "verified cache identity") {
			t.Errorf("unused-cache bypass not rejected: %v", fs)
		}
	})

	t.Run("built digest mismatch", func(t *testing.T) {
		rc, cfg, root := stageCertified(t, spec, healthySim())
		withFakePlan(t, editPlan(dispatchPlanFor(t, root), func(l []string) []string {
			f := strings.Split(l[0], "\t")
			f[3] = strings.Repeat("0", 64)
			l[0] = strings.Join(f, "\t")
			return l
		}), nil)
		fs := verifyRuntime(rc, spec, cfg)
		if !findingsHave(fs, "provider-dispatch", "m4", "verified cache identity") {
			t.Errorf("digest-mismatched dispatch row not rejected: %v", fs)
		}
	})

	t.Run("version mismatch", func(t *testing.T) {
		rc, cfg, root := stageCertified(t, spec, healthySim())
		withFakePlan(t, editPlan(dispatchPlanFor(t, root), func(l []string) []string {
			f := strings.Split(l[0], "\t")
			f[1] = "0.0.1"
			l[0] = strings.Join(f, "\t")
			return l
		}), nil)
		fs := verifyRuntime(rc, spec, cfg)
		if !findingsHave(fs, "provider-dispatch", "m4", "verified cache identity") {
			t.Errorf("version-mismatched dispatch row not rejected: %v", fs)
		}
	})

	t.Run("missing row", func(t *testing.T) {
		rc, cfg, root := stageCertified(t, spec, healthySim())
		var dropped string
		withFakePlan(t, editPlan(dispatchPlanFor(t, root), func(l []string) []string {
			dropped = strings.SplitN(l[0], "\t", 2)[0]
			return l[1:]
		}), nil)
		fs := verifyRuntime(rc, spec, cfg)
		if !findingsHave(fs, "provider-dispatch", dropped, "no dispatch-plan row") ||
			!findingsHave(fs, "provider-dispatch", "", "accounts for 9 active providers, want exactly 10") {
			t.Errorf("missing dispatch row not rejected: %v", fs)
		}
	})

	t.Run("duplicate row", func(t *testing.T) {
		rc, cfg, root := stageCertified(t, spec, healthySim())
		withFakePlan(t, editPlan(dispatchPlanFor(t, root), func(l []string) []string {
			return append(l, l[0])
		}), nil)
		fs := verifyRuntime(rc, spec, cfg)
		if !findingsHave(fs, "provider-dispatch", "m4", "duplicate dispatch-plan row") {
			t.Errorf("duplicate dispatch row not rejected: %v", fs)
		}
	})

	t.Run("extra name", func(t *testing.T) {
		rc, cfg, root := stageCertified(t, spec, healthySim())
		withFakePlan(t, editPlan(dispatchPlanFor(t, root), func(l []string) []string {
			return append(l, "cpio\t2.15\t/x/cpio\t"+strings.Repeat("a", 64))
		}), nil)
		fs := verifyRuntime(rc, spec, cfg)
		if !findingsHave(fs, "provider-dispatch", "cpio", "outside the ten active providers") {
			t.Errorf("extra dispatch row not rejected: %v", fs)
		}
	})

	t.Run("malformed rows", func(t *testing.T) {
		for _, bad := range []string{
			"m4\t1.4.19\t/x/m4",                             // three columns
			"m4\t1.4.19\t/x/m4\tnot-a-digest",               // non-hex digest
			"m4\t1.4.19\t/x/m4\t" + strings.Repeat("a", 63), // truncated digest
			"m4\t1.4.19\t/x/m4\t" + strings.Repeat("a", 65), // padded digest
		} {
			rc, cfg, root := stageCertified(t, spec, healthySim())
			withFakePlan(t, editPlan(dispatchPlanFor(t, root), func(l []string) []string {
				return append(l, bad)
			}), nil)
			fs := verifyRuntime(rc, spec, cfg)
			if !findingsHave(fs, "provider-dispatch", "", "malformed dispatch-plan row") {
				t.Errorf("row %q not rejected as malformed: %v", bad, fs)
			}
		}
	})

	t.Run("probe failure", func(t *testing.T) {
		rc, cfg, _ := stageCertified(t, spec, healthySim())
		withFakePlan(t, "", fmt.Errorf("exec format error"))
		fs := verifyRuntime(rc, spec, cfg)
		if !findingsHave(fs, "provider-dispatch", "", "could not disclose its dispatch plan") {
			t.Errorf("failed dispatch probe not rejected: %v", fs)
		}
	})

	t.Run("no probe against an unproven multicall", func(t *testing.T) {
		// When the multicall's own identity is not established, its answers
		// about itself are worthless: the probe must not run (the seam would
		// t.Fatal on any call), and the identity finding is the failure.
		rc, cfg, _ := stageCertified(t, spec, healthySim())
		withFakePlan(t, "", nil) // will fail the test if consulted
		runMulticallFn = func(rc *tool.RunContext, exe string, args ...string) (string, error) {
			t.Fatal("dispatch probe ran against an unproven multicall")
			return "", nil
		}
		cfg.multicallSHA = strings.Repeat("1", 64)
		fs := verifyRuntime(rc, spec, cfg)
		if !findingsHave(fs, "approved-multicall", "", "not the approved build manifest's") {
			t.Errorf("unproven multicall not rejected: %v", fs)
		}
	})
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
	// A complete runtime invocation minus the flag under test.
	base := []string{"runtime", "--profile", "C", "--manifest", "/x/m.tsv", "--bindir", "/x", "--multicall", "/x/c"}
	for _, args := range [][]string{
		{},
		{"bogus"},
		{"spec", "extra"},
		{"registry", "extra"},
		{"providers", "extra"},
		{"runtime"},
		{"runtime", "--profile", "C", "--manifest", "/x/m.tsv", "--bindir", "/x"},      // no --multicall
		{"runtime", "--profile", "C", "--manifest", "/x/m.tsv", "--multicall", "/x/c"}, // no --bindir
		{"runtime", "--profile", "C", "--bindir", "/x", "--multicall", "/x/c"},         // no --manifest
		{"runtime", "--manifest", "/x/m.tsv", "--bindir", "/x", "--multicall", "/x/c"}, // no --profile
		append(append([]string{}, base...), "--wat"),
		append(append([]string{}, base...), "--shell", "/bin/sh"),                                        // a PATH, not a name
		append(append([]string{}, base...), "--multicall-sha256", strings.Repeat("a", 64)),               // removed option: the digest comes from the manifest
		append(append([]string{}, base...), "--same-target"),                                             // removed option
		{"runtime", "--profile", "E", "--manifest", "/x/m.tsv", "--bindir", "/x", "--multicall", "/x/c"}, // no such profile
		{"runtime", "--profile", "c", "--manifest", "/x/m.tsv", "--bindir", "/x", "--multicall", "/x/c"}, // profiles are exact
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
	if !strings.Contains(stdout, "availability 92 go_applet, 14 shell, 10 external_provider") {
		t.Errorf("availability summary missing from %q", stdout)
	}
	if !strings.Contains(stdout, "effective selection: 84 go_applet, 22 shell, 10 external_provider") {
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
	if code != 0 || !strings.Contains(stdout, "posix-gate providers: PASS (10 active providers provisioned") ||
		strings.Contains(stdout, "16 providers provisioned") {
		t.Errorf("exit = %d, stdout = %q, stderr = %q", code, stdout, stderr)
	}

	// Empty cache: rejection, exit 1.
	rc = runtimeRC(t, "BASHY_BIN_CACHE="+t.TempDir())
	code, _, stderr = runGateCmd(t, rc, "providers")
	if code != 1 || !strings.Contains(stderr, "not provisioned") {
		t.Errorf("empty cache: exit = %d, stderr = %q", code, stderr)
	}

	rc = runtimeRC(t, "BASHY_BIN_CACHE=relative/cache")
	code, _, stderr = runGateCmd(t, rc, "providers")
	if code != 1 || !strings.Contains(stderr, "must be an absolute path") {
		t.Errorf("relative cache: exit = %d, stderr = %q", code, stderr)
	}
}

// writeManifest writes an approved build/run manifest with the given rows.
func writeManifest(t *testing.T, rows ...string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "approved-builds.tsv")
	body := "# approved build/run manifest (test)\n" + strings.Join(rows, "\n") + "\n"
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

// healthyManifestRows are the rows an honest release process would have
// written for the staged test bodies, plus extra build metadata the gate must
// tolerate (a build manifest legitimately records more than the pins).
func healthyManifestRows() []string {
	return []string{
		"profile\tC",
		"shell_sha256\t" + sha256Hex(shellBody),
		"multicall_sha256\t" + sha256Hex(multicallBody),
		"builder\ttest-harness",
	}
}

// TestGateRuntimeSubcommandEndToEnd drives the full staged verdict through the
// applet: seamed full registry, provisioned temp cache, staged bindir, healthy
// fake shell and dispatch plan, a real manifest file — then breaks one leg at
// a time and watches the verdict flip.
func TestGateRuntimeSubcommandEndToEnd(t *testing.T) {
	spec, err := loadSpec()
	if err != nil {
		t.Fatal(err)
	}
	reg := fullRegistry(t)
	prevReg := registeredFn
	registeredFn = func(n string) bool { return reg[n] }
	t.Cleanup(func() { registeredFn = prevReg })

	rc, cfg, root := stageCertified(t, spec, healthySim())
	bindir, multicall := cfg.binDir, cfg.multicall
	manifest := writeManifest(t, healthyManifestRows()...)
	args := func(extra ...string) []string {
		return append([]string{"runtime", "--profile", "C", "--manifest", manifest,
			"--bindir", bindir, "--multicall", multicall}, extra...)
	}

	code, stdout, stderr := runGateCmd(t, rc, args()...)
	if code != 0 || !strings.Contains(stdout, "posix-gate runtime: PASS") {
		t.Fatalf("healthy runtime: exit = %d, stdout = %q, stderr = %q", code, stdout, stderr)
	}

	// Provider provenance must be bound to the STAGED wrapper's dispatch
	// cache: an environment that does not name it is a rejection, never a
	// fall-back to the gate process's own default cache.
	rc = runtimeRC(t, "PATH="+bindir, "POSIXLY_CORRECT=1")
	code, _, stderr = runGateCmd(t, rc, args()...)
	if code != 1 || !strings.Contains(stderr, "[provider-cache]") {
		t.Errorf("unbound provider cache: exit = %d, stderr = %q", code, stderr)
	}

	// Same staging, but the gate's own environment lost POSIXLY_CORRECT.
	rc = runtimeRC(t, "PATH="+bindir, "BASHY_BIN_CACHE="+root)
	code, _, stderr = runGateCmd(t, rc, args()...)
	if code != 1 || !strings.Contains(stderr, "posix-gate runtime: FAIL") {
		t.Errorf("degraded runtime: exit = %d, stderr = %q", code, stderr)
	}
}

// TestGateRuntimeManifestRejections: the externally supplied manifest is the
// root of trust, so every defect in it — unreadable, missing digest, malformed
// digest, missing/unknown profile, duplicate pins, or a manifest for the OTHER
// profile — rejects before anything else is verified.
func TestGateRuntimeManifestRejections(t *testing.T) {
	spec, err := loadSpec()
	if err != nil {
		t.Fatal(err)
	}
	shellSHA := "shell_sha256\t" + sha256Hex(shellBody)
	multicallSHA := "multicall_sha256\t" + sha256Hex(multicallBody)
	cases := []struct {
		name    string
		rows    []string
		profile string
		check   string
		want    string
	}{
		{"profile mismatch", []string{"profile\tD", shellSHA, multicallSHA}, "C",
			"profile", "do not transfer between profiles"},
		{"missing shell digest", []string{"profile\tC", multicallSHA}, "C",
			"manifest", "does not record shell_sha256"},
		{"missing multicall digest", []string{"profile\tC", shellSHA}, "C",
			"manifest", "does not record multicall_sha256"},
		{"truncated digest", []string{"profile\tC", "shell_sha256\t" + strings.Repeat("a", 63), multicallSHA}, "C",
			"manifest", "malformed shell_sha256"},
		{"non-hex digest", []string{"profile\tC", shellSHA, "multicall_sha256\t" + strings.Repeat("g", 64)}, "C",
			"manifest", "malformed multicall_sha256"},
		{"missing profile", []string{shellSHA, multicallSHA}, "C",
			"manifest", "does not record a profile"},
		{"unknown profile", []string{"profile\tX", shellSHA, multicallSHA}, "C",
			"manifest", "unknown profile"},
		{"duplicate pin", []string{"profile\tC", shellSHA, shellSHA, multicallSHA}, "C",
			"manifest", "duplicate shell_sha256"},
		{"row without a tab", []string{"profile\tC", "shell_sha256 " + sha256Hex(shellBody), multicallSHA}, "C",
			"manifest", "not a key<TAB>value row"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			bindir, multicall := stageRuntime(t, spec)
			manifest := writeManifest(t, c.rows...)
			rc := runtimeRC(t, "PATH="+bindir, "POSIXLY_CORRECT=1")
			code, _, stderr := runGateCmd(t, rc, "runtime", "--profile", c.profile,
				"--manifest", manifest, "--bindir", bindir, "--multicall", multicall)
			if code != 1 || !strings.Contains(stderr, "["+c.check+"]") || !strings.Contains(stderr, c.want) {
				t.Errorf("exit = %d, stderr = %q; want exit 1 with [%s] %q", code, stderr, c.check, c.want)
			}
		})
	}

	// An unreadable manifest is the same class of failure.
	bindir, multicall := stageRuntime(t, spec)
	rc := runtimeRC(t, "PATH="+bindir, "POSIXLY_CORRECT=1")
	code, _, stderr := runGateCmd(t, rc, "runtime", "--profile", "C",
		"--manifest", filepath.Join(t.TempDir(), "absent.tsv"), "--bindir", bindir, "--multicall", multicall)
	if code != 1 || !strings.Contains(stderr, "cannot read the approved build manifest") {
		t.Errorf("unreadable manifest: exit = %d, stderr = %q", code, stderr)
	}
}
