// Copyright (c) 2025 qiangli
// See LICENSE for licensing information

// Hermetic: every test drives a temp cache directory. Nothing here downloads,
// compiles, or reaches the real provider cache.
package posixproviderscmd

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"

	"github.com/qiangli/coreutils/pkg/posixprovider"
	"github.com/qiangli/coreutils/tool"
)

// fakeProviderEnv turns this very test binary into a fake provider. It is the
// only way to observe what argv[0] the provider actually receives: a #! script
// cannot report it (the kernel re-execs the interpreter, so the shell's $0 is
// the script path), and compiling a helper would break hermeticity by requiring
// a toolchain.
const fakeProviderEnv = "COREUTILS_FAKE_POSIX_PROVIDER"

func TestMain(m *testing.M) {
	if os.Getenv(fakeProviderEnv) != "" {
		fmt.Printf("argv0:%s\n", os.Args[0])
		for _, a := range os.Args[1:] {
			fmt.Printf("arg:%s\n", a)
		}
		os.Exit(0)
	}
	os.Exit(m.Run())
}

// provisionSelf installs a copy of the test binary as the provider, so an
// invocation reports its own argv back.
func provisionSelf(t *testing.T, root, name string) {
	t.Helper()
	self, err := os.Executable()
	if err != nil {
		t.Skipf("cannot locate the test binary: %v", err)
	}
	body, err := os.ReadFile(self)
	if err != nil {
		t.Skipf("cannot read the test binary: %v", err)
	}
	provision(t, root, name, string(body))
}

// TestArgvPassthrough pins the two things a certification arm depends on: every
// operand reaches the provider unchanged, and argv[0] is the plain COMMAND NAME
// rather than the cache path. The second is not cosmetic — GNU make and binutils
// print argv[0] in their diagnostics (a cert arm diffs those strings), and vim
// picks vi mode vs ex mode from it, so an `ex` invoked under its cache path
// would come up in vi mode and hang a scripted edit.
func TestArgvPassthrough(t *testing.T) {
	root := t.TempDir()
	provisionSelf(t, root, "bc")

	rc, out, errb := newRC(t, root, fakeProviderEnv+"=1")
	code, stdout, stderr := run(t, "bc", rc, out, errb, "-l", "--", "a b", "-x")
	if code != 0 {
		t.Fatalf("exit = %d, stderr = %q", code, stderr)
	}
	want := "argv0:bc\narg:-l\narg:--\narg:a b\narg:-x\n"
	if stdout != want {
		t.Errorf("stdout = %q, want %q", stdout, want)
	}
}

// newRC builds a RunContext over a temp cache root. Buffers are returned to the
// caller so they are read AFTER Run — never in the same expression that calls
// it, which evaluates the buffers first and silently asserts on empty strings.
func newRC(t *testing.T, cacheRoot string, extraEnv ...string) (*tool.RunContext, *bytes.Buffer, *bytes.Buffer) {
	t.Helper()
	var out, errb bytes.Buffer
	env := append([]string{"BASHY_BIN_CACHE=" + cacheRoot}, extraEnv...)
	rc := &tool.RunContext{
		Ctx: context.Background(),
		Dir: t.TempDir(),
		Env: env,
		FS:  tool.NewLocalFS(),
		Stdio: tool.Stdio{
			In:  strings.NewReader(""),
			Out: &out,
			Err: &errb,
		},
	}
	return rc, &out, &errb
}

// run executes a registered tool and returns (code, stdout, stderr). The
// command RUNS FIRST; the buffers are read only after it returns.
func run(t *testing.T, name string, rc *tool.RunContext, out, errb *bytes.Buffer, args ...string) (int, string, string) {
	t.Helper()
	tl := tool.Lookup(name)
	if tl == nil {
		t.Fatalf("tool %q is not registered", name)
	}
	code := tl.Run(rc, args)
	return code, out.String(), errb.String()
}

// provision writes a fake provisioned provider plus the provenance sidecar the
// build recipe would have written.
func provision(t *testing.T, root, name, body string) string {
	t.Helper()
	e, ok := posixprovider.Lookup(name)
	if !ok {
		t.Fatalf("no manifest entry for %q", name)
	}
	dir := filepath.Join(root, e.Command, e.Version)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	bin := filepath.Join(dir, e.Command)
	if err := os.WriteFile(bin, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256([]byte(body))
	prov := fmt.Sprintf("command\t%s\nversion\t%s\nlicense\t%s\nsource_url\t%s\nsource_sha256\t%s\ncompiler\ttest\nbuilt_sha256\t%s\n",
		e.Command, e.Version, e.License, e.URL, e.SHA256, hex.EncodeToString(sum[:]))
	if err := os.WriteFile(filepath.Join(dir, "provenance.tsv"), []byte(prov), 0o644); err != nil {
		t.Fatal(err)
	}
	return bin
}

// ---------------------------------------------------------------------------
// registration — the load-bearing assertion
// ---------------------------------------------------------------------------

// TestProviderNamesAreRegistered is what the whole package exists for. The
// certification harness's sut-wire.sh rebuilds the measured PATH from the
// multicall's OWN inventory, so a provider missing from tool.Names() is a
// command the arm silently takes from the host while reporting itself
// bashy-only.
func TestProviderNamesAreRegistered(t *testing.T) {
	names := tool.Names()
	for _, n := range posixprovider.Names() {
		if n == "ed" {
			continue // retained pin; the registered owner is cmds/ed
		}
		if !slices.Contains(names, n) {
			t.Errorf("tool.Names() is missing provider %q", n)
		}
		if tool.Lookup(n) == nil {
			t.Errorf("tool.Lookup(%q) = nil", n)
		}
	}
	if !slices.Contains(names, "posix-providers") {
		t.Error("tool.Names() is missing the posix-providers applet")
	}
}

// TestProviderNamesAreRegisteredOnEveryPlatform pins the deliberate choice that
// platform gating happens at RUN time, not at registration time: the multicall
// owns `ed` on Windows too, and refuses loudly there rather than letting the
// name fall through to whatever $PATH holds.
func TestProviderNamesAreRegisteredOnEveryPlatform(t *testing.T) {
	for _, n := range []string{"man"} {
		e, _ := posixprovider.Lookup(n)
		if e.SupportsGOOS("windows") {
			t.Skipf("%s now declares windows; this test no longer distinguishes anything", n)
		}
		if tool.Lookup(n) == nil {
			t.Errorf("%s is not registered; a platform-gated registration would let it fall through to $PATH", n)
		}
	}
}

// ---------------------------------------------------------------------------
// no silent fallback
// ---------------------------------------------------------------------------

func TestUnprovisionedProviderFailsLoudly(t *testing.T) {
	rc, out, errb := newRC(t, t.TempDir())
	code, stdout, stderr := run(t, "make", rc, out, errb)

	if code != 127 {
		t.Errorf("exit = %d, want 127 (command not found)", code)
	}
	if stdout != "" {
		t.Errorf("stdout = %q, want empty", stdout)
	}
	if !strings.Contains(stderr, "not provisioned") {
		t.Errorf("stderr does not say why: %q", stderr)
	}
	if !strings.Contains(stderr, "bashy posix-providers build make") {
		t.Errorf("stderr does not name the fix: %q", stderr)
	}
}

func TestProviderWithBadProvenanceFailsLoudly(t *testing.T) {
	root := t.TempDir()
	bin := provision(t, root, "m4", "#!/bin/sh\nexit 0\n")
	if err := os.WriteFile(bin, []byte("#!/bin/sh\nexit 1\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	rc, out, errb := newRC(t, root)
	code, _, stderr := run(t, "m4", rc, out, errb)
	if code != 127 {
		t.Errorf("exit = %d, want 127", code)
	}
	if !strings.Contains(stderr, "provenance mismatch") {
		t.Errorf("stderr does not report the provenance failure: %q", stderr)
	}
}

// ---------------------------------------------------------------------------
// the posix-providers applet
// ---------------------------------------------------------------------------

func TestAdminList(t *testing.T) {
	root := t.TempDir()
	provision(t, root, "bc", "#!/bin/sh\nexit 0\n")

	rc, out, errb := newRC(t, root)
	code, stdout, stderr := run(t, "posix-providers", rc, out, errb, "list")
	if code != 0 {
		t.Fatalf("exit = %d, stderr = %q", code, stderr)
	}
	if !strings.Contains(stdout, "COMMAND") || !strings.Contains(stdout, "LICENSE") {
		t.Errorf("list has no header: %q", stdout)
	}
	for _, n := range posixprovider.Names() {
		if !strings.Contains(stdout, n) {
			t.Errorf("list omits %q", n)
		}
	}
	if !strings.Contains(stdout, "provisioned") {
		t.Errorf("list never says a provider is provisioned: %q", stdout)
	}
	if !strings.Contains(stdout, "not provisioned") {
		t.Errorf("list never says a provider is missing: %q", stdout)
	}
}

func TestAdminCheck(t *testing.T) {
	root := t.TempDir()
	provision(t, root, "nm", "#!/bin/sh\nexit 0\n")

	t.Run("provisioned passes", func(t *testing.T) {
		rc, out, errb := newRC(t, root)
		code, stdout, stderr := run(t, "posix-providers", rc, out, errb, "check", "nm")
		if code != 0 {
			t.Fatalf("exit = %d, stderr = %q", code, stderr)
		}
		if !strings.HasPrefix(stdout, "PASS nm ") {
			t.Errorf("stdout = %q", stdout)
		}
	})

	t.Run("missing fails", func(t *testing.T) {
		rc, out, errb := newRC(t, root)
		code, _, stderr := run(t, "posix-providers", rc, out, errb, "check", "strip")
		if code != 1 {
			t.Errorf("exit = %d, want 1", code)
		}
		if !strings.Contains(stderr, "FAIL strip") {
			t.Errorf("stderr = %q", stderr)
		}
	})

	t.Run("unknown name is a usage error", func(t *testing.T) {
		rc, out, errb := newRC(t, root)
		code, _, stderr := run(t, "posix-providers", rc, out, errb, "check", "gcc")
		if code != 2 {
			t.Errorf("exit = %d, want 2", code)
		}
		if !strings.Contains(stderr, "not a pinned provider") {
			t.Errorf("stderr = %q", stderr)
		}
	})
}

func TestAdminUsageErrors(t *testing.T) {
	cases := []struct {
		name string
		args []string
	}{
		{"no subcommand", nil},
		{"unknown subcommand", []string{"install"}},
		{"list takes no operands", []string{"list", "make"}},
		// build must reject an unknown name BEFORE it goes looking for a
		// compiler or a network.
		{"build unknown name", []string{"build", "gcc"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rc, out, errb := newRC(t, t.TempDir())
			code, _, stderr := run(t, "posix-providers", rc, out, errb, tc.args...)
			if code != 2 {
				t.Errorf("exit = %d, want 2 (stderr %q)", code, stderr)
			}
		})
	}
}

func TestAdminHelp(t *testing.T) {
	rc, out, errb := newRC(t, t.TempDir())
	code, stdout, _ := run(t, "posix-providers", rc, out, errb, "--help")
	if code != 0 {
		t.Fatalf("exit = %d", code)
	}
	for _, want := range []string{"list", "check", "build", "BASHY_POSIX_PROVIDERS=off"} {
		if !strings.Contains(stdout, want) {
			t.Errorf("help does not mention %q", want)
		}
	}
}

func TestFindBuildScriptReportsWhereItLooked(t *testing.T) {
	rc, _, _ := newRC(t, t.TempDir())
	// rc.Dir is an empty temp dir with no coreutils checkout above it, so the
	// walk-up must fail with an actionable message rather than a nil path.
	if _, err := findBuildScript(rc); err == nil {
		t.Skip("a coreutils checkout is reachable from the temp dir; nothing to assert")
	} else if !strings.Contains(err.Error(), BuildScriptEnv) {
		t.Errorf("error does not name the override env var: %v", err)
	}
}

func TestFindBuildScriptHonoursOverride(t *testing.T) {
	dir := t.TempDir()
	script := filepath.Join(dir, "build.sh")
	if err := os.WriteFile(script, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	rc, _, _ := newRC(t, t.TempDir(), BuildScriptEnv+"="+script)
	got, err := findBuildScript(rc)
	if err != nil {
		t.Fatalf("findBuildScript: %v", err)
	}
	if got != script {
		t.Errorf("findBuildScript = %q, want %q", got, script)
	}
}

func TestMaterializedManifestMatchesTheEmbeddedOne(t *testing.T) {
	path := materializeManifest()
	if path == "" {
		t.Skip("temp file unavailable")
	}
	defer os.Remove(path)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != posixprovider.ManifestText() {
		t.Error("the manifest handed to the build recipe is not the embedded one")
	}
}

// ---------------------------------------------------------------------------
// the opt-out
// ---------------------------------------------------------------------------

const optOutChildEnv = "COREUTILS_POSIXPROVIDERS_OPTOUT_CHILD"

// TestOptOutUnregistersProviders re-execs the test binary with the opt-out set,
// because registration happens in init() and cannot be undone in-process. The
// child prints what it found; the parent asserts on that.
func TestOptOutUnregistersProviders(t *testing.T) {
	if os.Getenv(optOutChildEnv) == "1" {
		for _, n := range posixprovider.Names() {
			if tool.Lookup(n) != nil {
				fmt.Println("STILL-REGISTERED:" + n)
			}
		}
		if tool.Lookup("posix-providers") == nil {
			// The applet must survive the opt-out: it is how you get back out
			// of the un-provisioned state.
			fmt.Println("ADMIN-MISSING")
		}
		fmt.Println("CHILD-DONE")
		return
	}

	cmd := exec.Command(os.Args[0], "-test.run=TestOptOutUnregistersProviders", "-test.v")
	cmd.Env = append(os.Environ(),
		optOutChildEnv+"=1",
		posixprovider.OptOutEnv+"=off",
	)
	outBytes, err := cmd.CombinedOutput()
	got := string(outBytes)
	if err != nil {
		t.Fatalf("child failed: %v\n%s", err, got)
	}
	if !strings.Contains(got, "CHILD-DONE") {
		t.Fatalf("child never ran the assertion body:\n%s", got)
	}
	if strings.Contains(got, "STILL-REGISTERED:") {
		t.Errorf("%s=off left providers registered:\n%s", posixprovider.OptOutEnv, got)
	}
	if strings.Contains(got, "ADMIN-MISSING") {
		t.Error("the opt-out also removed posix-providers; it must stay reachable")
	}
}

// TestAdminDispatchPlan: the introspection surface posix-gate trusts. One
// strict TSV row per provider this host can dispatch — command, version,
// resolved path, verified built sha256 — and a loud FAIL (exit 1) for any
// provider whose dispatch target cannot be verified. A plan with holes must
// never read as a plan.
func TestAdminDispatchPlan(t *testing.T) {
	root := t.TempDir()
	bodies := map[string]string{}
	for _, e := range posixprovider.DispatchEntries() {
		body := "#!/bin/sh\n# " + e.Command + "\nexit 0\n"
		bodies[e.Command] = body
		provision(t, root, e.Command, body)
	}

	rc, out, errb := newRC(t, root)
	code, stdout, stderr := run(t, "posix-providers", rc, out, errb, "dispatch-plan")

	rows := map[string][]string{}
	for _, line := range strings.Split(strings.TrimSuffix(stdout, "\n"), "\n") {
		f := strings.Split(line, "\t")
		if len(f) != 4 {
			t.Fatalf("malformed dispatch-plan row: %q", line)
		}
		rows[f[0]] = f
	}
	unsupported := 0
	for _, e := range posixprovider.DispatchEntries() {
		if !e.SupportsGOOS(runtime.GOOS) {
			// The honest answer for a platform the manifest does not declare
			// is FAIL, not silence: this host's wrapper cannot dispatch it.
			unsupported++
			if _, ok := rows[e.Command]; ok {
				t.Errorf("%s is not declared for %s but got a dispatch row", e.Command, runtime.GOOS)
			}
			if !strings.Contains(stderr, "FAIL "+e.Command) {
				t.Errorf("undeclared %s not FAILed on stderr: %q", e.Command, stderr)
			}
			continue
		}
		f, ok := rows[e.Command]
		if !ok {
			t.Errorf("no dispatch row for %s: %q", e.Command, stdout)
			continue
		}
		sum := sha256.Sum256([]byte(bodies[e.Command]))
		if f[1] != e.Version || f[2] != filepath.Join(root, e.Command, e.Version, e.Command) ||
			f[3] != hex.EncodeToString(sum[:]) {
			t.Errorf("dispatch row for %s = %v, want version %s under the cache with the built digest", e.Command, f, e.Version)
		}
	}
	if unsupported == 0 && code != 0 {
		t.Errorf("fully dispatchable plan: exit = %d, stderr = %q", code, stderr)
	}
	if unsupported > 0 && code != 1 {
		t.Errorf("plan with undeclared providers: exit = %d, want 1", code)
	}

	// A tampered provider has no verifiable dispatch target: FAIL, exit 1.
	e, _ := posixprovider.Lookup("make")
	if err := os.WriteFile(filepath.Join(root, "make", e.Version, "make"), []byte("tampered"), 0o755); err != nil {
		t.Fatal(err)
	}
	rc, out, errb = newRC(t, root)
	code, _, stderr = run(t, "posix-providers", rc, out, errb, "dispatch-plan")
	if code != 1 || !strings.Contains(stderr, "FAIL make") || !strings.Contains(stderr, "no verifiable dispatch target") {
		t.Errorf("tampered make: exit = %d, stderr = %q", code, stderr)
	}

	// Operands are a usage error: the plan is always the whole pinned set.
	rc, out, errb = newRC(t, root)
	code, _, _ = run(t, "posix-providers", rc, out, errb, "dispatch-plan", "make")
	if code != 2 {
		t.Errorf("dispatch-plan with an operand: exit = %d, want 2", code)
	}
}
