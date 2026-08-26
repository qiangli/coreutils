// Copyright (c) 2025 qiangli
// See LICENSE for licensing information

// Hermetic by construction: every test drives a temp cache directory. Nothing
// here touches the network, a compiler, or the real provider cache — which is
// also the property the code under test guarantees, so a test that needed any
// of those would be evidence of a bug.
package posixprovider

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
)

// provision writes a fake provisioned provider: the binary, plus the
// provenance.tsv sidecar build.sh would have written next to it.
func provision(t *testing.T, root string, e Entry, body []byte) string {
	t.Helper()
	dir := filepath.Join(root, e.Command, e.Version)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	bin := filepath.Join(dir, e.Command)
	if err := os.WriteFile(bin, body, 0o755); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(body)
	writeProvenance(t, dir, map[string]string{
		"command":       e.Command,
		"version":       e.Version,
		"license":       e.License,
		"source_url":    e.URL,
		"source_sha256": e.SHA256,
		"compiler":      "/usr/bin/cc",
		"built_sha256":  hex.EncodeToString(sum[:]),
		"distributed":   "no (built locally; copyleft binaries are never republished)",
	})
	return bin
}

func writeProvenance(t *testing.T, dir string, rec map[string]string) {
	t.Helper()
	var b strings.Builder
	for _, k := range []string{"command", "version", "license", "source_url", "source_sha256", "compiler", "built_sha256", "distributed"} {
		if v, ok := rec[k]; ok {
			b.WriteString(k + "\t" + v + "\n")
		}
	}
	if err := os.WriteFile(filepath.Join(dir, "provenance.tsv"), []byte(b.String()), 0o644); err != nil {
		t.Fatal(err)
	}
}

func mustLookup(t *testing.T, name string) Entry {
	t.Helper()
	e, ok := Lookup(name)
	if !ok {
		t.Fatalf("manifest has no entry for %q", name)
	}
	return e
}

func TestManifestShape(t *testing.T) {
	names := Names()
	want := []string{"ar", "bc", "ctags", "ed", "ex", "localedef", "lp", "m4", "mailx",
		"make", "man", "nm", "patch", "strip", "talk", "vi"}
	if !slices.Equal(names, want) {
		t.Fatalf("Names() = %v, want %v", names, want)
	}
	for _, n := range names {
		e := mustLookup(t, n)
		if len(e.SHA256) != 64 {
			t.Errorf("%s: sha256 pin is %d chars, want 64", n, len(e.SHA256))
		}
		if e.Version == "" || e.License == "" || e.URL == "" {
			t.Errorf("%s: incomplete entry %+v", n, e)
		}
	}
	// Unix-only, no windows. CUPS and s-nail are POSIX-only upstream, and
	// neither ships a native windows build path.
	for _, n := range []string{"ed", "man", "lp", "mailx"} {
		e := mustLookup(t, n)
		if e.SupportsGOOS("windows") {
			t.Errorf("%s: manifest now declares windows; update the gating tests", n)
		}
		if !e.SupportsGOOS("linux") || !e.SupportsGOOS("darwin") {
			t.Errorf("%s: manifest no longer declares linux+darwin: %v", n, e.Platforms)
		}
	}
	// linux ONLY, and both are measured rather than assumed: localedef compiles
	// glibc's own sources against glibc, and netkit's configure emits a
	// malformed MCONFIG on darwin. A darwin claim here would be a lie the build
	// only discovers three minutes in.
	for _, n := range []string{"localedef", "talk"} {
		e := mustLookup(t, n)
		if !e.SupportsGOOS("linux") {
			t.Errorf("%s: manifest no longer declares linux: %v", n, e.Platforms)
		}
		if e.SupportsGOOS("darwin") || e.SupportsGOOS("windows") {
			t.Errorf("%s: declared beyond linux (%v) without a build to back it", n, e.Platforms)
		}
	}
	if !mustLookup(t, "make").SupportsGOOS("windows") {
		t.Error("make: expected windows to be declared")
	}
}

func TestManifestTextIsTheEmbeddedFile(t *testing.T) {
	text := ManifestText()
	if !strings.Contains(text, "LICENSE POSTURE") {
		t.Error("ManifestText lost the licence-posture header; the recipe reads this file")
	}
	if !strings.Contains(text, "\nmake\t4.3\t") {
		t.Error("ManifestText does not carry the make row")
	}
}

func TestGoAppletPinsAreRetainedButNotDispatchOwners(t *testing.T) {
	for _, name := range []string{"ed", "patch"} {
		if !Has(name) {
			t.Fatalf("%s differential-control pin was removed", name)
		}
		if IsDispatchProvider(name) {
			t.Fatalf("%s is still an active provider despite Go applet ownership", name)
		}
		if slices.Contains(DispatchNames(), name) {
			t.Fatalf("%s leaked into active provider names", name)
		}
	}
	if len(DispatchEntries()) != len(Entries())-2 {
		t.Fatalf("active entries=%d all pins=%d, want two retained controls", len(DispatchEntries()), len(Entries()))
	}
}

func TestParseManifestRefusesBadRows(t *testing.T) {
	cases := map[string]string{
		"missing column":  "make\t4.3\tGPL-3.0\tlinux\tdeadbeef\n",
		"short digest":    "make\t4.3\tGPL-3.0\tlinux\tdeadbeef\thttps://example/x.tar.gz\n",
		"no platforms":    "make\t4.3\tGPL-3.0\t\t" + strings.Repeat("a", 64) + "\thttps://example/x.tar.gz\n",
		"empty command":   "\t4.3\tGPL-3.0\tlinux\t" + strings.Repeat("a", 64) + "\thttps://example/x.tar.gz\n",
		"nothing but doc": "# only a comment\n",
	}
	for name, text := range cases {
		if _, err := parseManifest(text); err == nil {
			t.Errorf("%s: parseManifest accepted a row it should refuse", name)
		}
	}
	good := "make\t4.3\tGPL-3.0\tlinux, darwin\t" + strings.Repeat("a", 64) + "\thttps://example/x.tar.gz\n"
	got, err := parseManifest("# header\n\n" + good)
	if err != nil {
		t.Fatalf("parseManifest rejected a good row: %v", err)
	}
	if len(got) != 1 || !slices.Equal(got[0].Platforms, []string{"linux", "darwin"}) {
		t.Fatalf("parseManifest = %+v", got)
	}
}

func TestResolveCacheHit(t *testing.T) {
	root := t.TempDir()
	e := mustLookup(t, "make")
	want := provision(t, root, e, []byte("#!/bin/sh\nexit 0\n"))

	r := Resolver{CacheRoot: root, GOOS: runtime.GOOS}
	got, err := r.Resolve("make")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if got != want {
		t.Errorf("Resolve = %q, want %q", got, want)
	}
}

func TestResolveHonoursBashyBinCache(t *testing.T) {
	root := t.TempDir()
	e := mustLookup(t, "bc")
	want := provision(t, root, e, []byte("#!/bin/sh\nexit 0\n"))

	// The package-level Resolve goes through binmgr.CacheDir, which reads
	// $BASHY_BIN_CACHE. Still hermetic: the root is a temp dir.
	t.Setenv("BASHY_BIN_CACHE", root)
	got, err := Resolve("bc")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if got != want {
		t.Errorf("Resolve = %q, want %q", got, want)
	}
}

func TestResolveCacheMissNamesTheProvisioningCommand(t *testing.T) {
	root := t.TempDir()
	r := Resolver{CacheRoot: root, GOOS: runtime.GOOS}

	_, err := r.Resolve("m4")
	if err == nil {
		t.Fatal("Resolve succeeded against an empty cache")
	}
	if !errors.Is(err, ErrNotProvisioned) {
		t.Errorf("error is not ErrNotProvisioned: %v", err)
	}
	msg := err.Error()
	// The whole point of the miss path: it must tell the operator the exact
	// command that fixes it, because Resolve itself will never fix it.
	if !strings.Contains(msg, "bashy posix-providers build m4") {
		t.Errorf("miss error does not name the provisioning command: %s", msg)
	}
	if !strings.Contains(msg, "never downloads or compiles") {
		t.Errorf("miss error does not state the prepare/run split: %s", msg)
	}
}

func TestResolveRefusesUndeclaredPlatform(t *testing.T) {
	root := t.TempDir()
	e := mustLookup(t, "ed")
	// Provision it anyway: the platform gate must refuse BEFORE the cache is
	// consulted, so a stray binary cannot re-enable an undeclared platform.
	provision(t, root, e, []byte("#!/bin/sh\nexit 0\n"))

	r := Resolver{CacheRoot: root, GOOS: "windows"}
	_, err := r.Resolve("ed")
	if err == nil {
		t.Fatal("Resolve succeeded on an undeclared platform")
	}
	if !errors.Is(err, ErrUnsupportedPlatform) {
		t.Errorf("error is not ErrUnsupportedPlatform: %v", err)
	}
	if !strings.Contains(err.Error(), "not supported on windows") {
		t.Errorf("error does not name the platform: %v", err)
	}
	if !strings.Contains(err.Error(), "linux,darwin") {
		t.Errorf("error does not list the declared platforms: %v", err)
	}
}

func TestResolveRejectsUnknownName(t *testing.T) {
	r := Resolver{CacheRoot: t.TempDir(), GOOS: runtime.GOOS}
	_, err := r.Resolve("definitely-not-pinned")
	if !errors.Is(err, ErrUnknown) {
		t.Fatalf("error is not ErrUnknown: %v", err)
	}
}

func TestResolveRejectsProvenanceMismatch(t *testing.T) {
	e := mustLookup(t, "patch")

	t.Run("binary does not match built_sha256", func(t *testing.T) {
		root := t.TempDir()
		bin := provision(t, root, e, []byte("original"))
		// Swap the bytes after provenance was recorded: the classic "somebody
		// dropped a different binary into the cache" case.
		if err := os.WriteFile(bin, []byte("tampered"), 0o755); err != nil {
			t.Fatal(err)
		}
		r := Resolver{CacheRoot: root, GOOS: runtime.GOOS}
		_, err := r.Resolve("patch")
		if !errors.Is(err, ErrProvenance) {
			t.Fatalf("error is not ErrProvenance: %v", err)
		}
	})

	t.Run("provenance records another version", func(t *testing.T) {
		root := t.TempDir()
		bin := provision(t, root, e, []byte("original"))
		writeProvenance(t, filepath.Dir(bin), map[string]string{
			"command": e.Command, "version": "0.0.0-not-pinned",
			"source_sha256": e.SHA256, "built_sha256": strings.Repeat("b", 64),
		})
		r := Resolver{CacheRoot: root, GOOS: runtime.GOOS}
		_, err := r.Resolve("patch")
		if !errors.Is(err, ErrProvenance) {
			t.Fatalf("error is not ErrProvenance: %v", err)
		}
		if !strings.Contains(err.Error(), "0.0.0-not-pinned") {
			t.Errorf("error does not show the offending value: %v", err)
		}
	})

	t.Run("provenance records another source digest", func(t *testing.T) {
		root := t.TempDir()
		bin := provision(t, root, e, []byte("original"))
		sum := sha256.Sum256([]byte("original"))
		writeProvenance(t, filepath.Dir(bin), map[string]string{
			"command": e.Command, "version": e.Version,
			"source_sha256": strings.Repeat("c", 64),
			"built_sha256":  hex.EncodeToString(sum[:]),
		})
		r := Resolver{CacheRoot: root, GOOS: runtime.GOOS}
		if _, err := r.Resolve("patch"); !errors.Is(err, ErrProvenance) {
			t.Fatalf("error is not ErrProvenance: %v", err)
		}
	})

	t.Run("no provenance at all", func(t *testing.T) {
		root := t.TempDir()
		bin := provision(t, root, e, []byte("original"))
		if err := os.Remove(filepath.Join(filepath.Dir(bin), "provenance.tsv")); err != nil {
			t.Fatal(err)
		}
		r := Resolver{CacheRoot: root, GOOS: runtime.GOOS}
		_, err := r.Resolve("patch")
		if !errors.Is(err, ErrProvenance) {
			t.Fatalf("an unattributable binary must be an error, not a warning: %v", err)
		}
	})
}

func TestResolveNeverTouchesTheNetwork(t *testing.T) {
	// A structural assertion, not a behavioural one: the resolve path must not
	// reach a downloader. binmgr's own Ensure/download live in another package
	// and are simply never called from here — this test pins the property that
	// the miss path RETURNS rather than provisioning.
	root := t.TempDir()
	r := Resolver{CacheRoot: root, GOOS: runtime.GOOS}
	if _, err := r.Resolve("ctags"); err == nil {
		t.Fatal("a cache miss must fail, never provision")
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Errorf("Resolve wrote into the cache: %v", entries)
	}
}

func TestStatus(t *testing.T) {
	root := t.TempDir()
	e := mustLookup(t, "nm")
	provision(t, root, e, []byte("#!/bin/sh\nexit 0\n"))
	r := Resolver{CacheRoot: root, GOOS: runtime.GOOS}

	if st := r.Status("nm"); !st.Ready() {
		t.Errorf("nm status not ready: %v", st.Err)
	}
	if st := r.Status("strip"); st.Ready() {
		t.Error("strip reported ready with nothing in the cache")
	}
	unsupported := Resolver{CacheRoot: root, GOOS: "windows"}.Status("man")
	if unsupported.Supported {
		t.Error("man reported supported on windows")
	}
}

func TestEnabledIn(t *testing.T) {
	for _, v := range []string{"off", "OFF", " Off "} {
		if EnabledIn(v) {
			t.Errorf("EnabledIn(%q) = true, want false", v)
		}
	}
	for _, v := range []string{"", "on", "1", "yes", "anything"} {
		if !EnabledIn(v) {
			t.Errorf("EnabledIn(%q) = false, want true (only the exact word 'off' opts out)", v)
		}
	}
}

// TestVerifiedIdentity: the full verified identity — resolved path, pinned
// version, and the built digest re-verified against the file — that a
// dispatch-plan disclosure is compared against. Every Resolve failure mode is
// a VerifiedIdentity failure mode; a tampered binary yields no identity.
func TestVerifiedIdentity(t *testing.T) {
	root := t.TempDir()
	e := mustLookup(t, "m4")
	body := []byte("#!/bin/sh\n# fake m4\nexit 0\n")
	bin := provision(t, root, e, body)
	r := Resolver{CacheRoot: root, GOOS: "linux"}

	id, err := r.VerifiedIdentity("m4")
	if err != nil {
		t.Fatalf("VerifiedIdentity(m4) = %v", err)
	}
	sum := sha256.Sum256(body)
	if id.Command != "m4" || id.Version != e.Version || id.Path != bin ||
		id.BuiltSHA256 != hex.EncodeToString(sum[:]) {
		t.Errorf("identity = %+v, want %s %s at %s with built sha %s",
			id, e.Command, e.Version, bin, hex.EncodeToString(sum[:]))
	}

	// A tampered binary is unattributable: no identity, an ErrProvenance.
	if err := os.WriteFile(bin, []byte("tampered"), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := r.VerifiedIdentity("m4"); !errors.Is(err, ErrProvenance) {
		t.Errorf("tampered binary: err = %v, want ErrProvenance", err)
	}

	// Unknown and unprovisioned names fail exactly like Resolve.
	if _, err := r.VerifiedIdentity("gcc"); !errors.Is(err, ErrUnknown) {
		t.Errorf("unknown name: err = %v, want ErrUnknown", err)
	}
	if _, err := r.VerifiedIdentity("bc"); !errors.Is(err, ErrNotProvisioned) {
		t.Errorf("unprovisioned: err = %v, want ErrNotProvisioned", err)
	}
}
