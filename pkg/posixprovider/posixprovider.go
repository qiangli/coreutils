// Copyright (c) 2025 qiangli
// See LICENSE for licensing information

// Package posixprovider is the READ half of the POSIX external provider
// mechanism: the sixteen POSIX-required commands this repository deliberately
// does not implement in Go (make, bc, patch, m4, ed, man, ctags, ar, nm, strip,
// ex, vi, lp, mailx, localedef, talk), pinned by manifest, built locally from
// upstream source, and resolved out of the binmgr cache so the multicall OWNS
// the name.
//
// # Why the name must be owned
//
// Profile C of the POSIX certification campaign is "GNU Bash + the Bashy Go
// coreutils". Until this package existed, those sixteen names were not in
// tool.Names(), so the shell adapter fell through to $PATH and the arm silently
// measured Ubuntu's binaries while claiming to measure ours. There is therefore
// NO fallback here of any kind: an unavailable provider is a loud failure, never
// a quiet substitution.
//
// # Resolve NEVER builds
//
// Resolve is a CACHE LOOKUP. It does not download, does not compile, does not
// touch the network, and has no code path that could. Provisioning is a
// PREPARE-time activity (tools/posix-providers/build.sh, driven by the
// `posix-providers` applet); running is a TEST-time activity. Fusing them would
// let a resolve inside a six-hour certification arm decide to fetch and compile
// GNU make, injecting network and toolchain variance into measured evidence and
// risking a hang that costs the whole arm. The separation is the design.
//
// # Licence posture
//
// Every provider is copyleft (GPL-2.0, GPL-3.0, or the Vim licence). We ship the
// manifest and the recipe, never the binaries — see the header of manifest.tsv
// and ../../docs/posix-provider-distribution-policy.md in the umbrella.
package posixprovider

import (
	"bufio"
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"

	"github.com/qiangli/coreutils/pkg/binmgr"
)

// The manifest is embedded, and this directory is the ONE canonical home for
// it: a go:embed path cannot escape its own package directory, and two copies
// of a pin table is how a resolver and a build recipe come to disagree about
// what is pinned. tools/posix-providers/build.sh reads this same file.
//
//go:embed manifest.tsv
var manifestFS embed.FS

// OptOutEnv unregisters the providers from the tool registry when set to "off".
// It exists so plain bashy stays standalone-graceful on a machine with no
// provider cache: with it set, the sixteen names are simply not ours and normal
// PATH resolution applies again. It is an EXPLICIT opt-out — the default is to
// own the names and fail loudly.
const OptOutEnv = "BASHY_POSIX_PROVIDERS"

// Sentinel errors. Callers match these; the messages carry the detail.
var (
	// ErrUnknown: the name is not in the manifest at all.
	ErrUnknown = errors.New("not a POSIX external provider")
	// ErrUnsupportedPlatform: the manifest does not declare this platform. The
	// sentinel text is a fragment on purpose — it is spliced into "<cmd> <ver>
	// is not supported on <goos>", which is the phrasing a caller reads.
	ErrUnsupportedPlatform = errors.New("not supported on")
	// ErrNotProvisioned: no cached binary. The error text names the command to run.
	ErrNotProvisioned = errors.New("not provisioned")
	// ErrProvenance: a cached binary that does not match its recorded provenance.
	// This is an ERROR, never a warning: an unattributable binary in a
	// certification arm is worse than a missing one, because it still produces
	// numbers.
	ErrProvenance = errors.New("provenance mismatch")
)

// Entry is one manifest row.
type Entry struct {
	Command   string
	Version   string
	License   string
	Platforms []string // GOOS values, as declared
	SHA256    string   // digest of the UPSTREAM SOURCE archive
	URL       string   // upstream source URL
}

// SupportsGOOS reports whether the manifest declares this provider for goos.
// A platform absent from the row is REFUSED, not attempted.
func (e Entry) SupportsGOOS(goos string) bool {
	for _, p := range e.Platforms {
		if p == goos {
			return true
		}
	}
	return false
}

var (
	entries []Entry
	byName  map[string]Entry
)

func init() {
	data, err := manifestFS.ReadFile("manifest.tsv")
	if err != nil {
		// Unreachable: the file is embedded at build time.
		panic("posixprovider: embedded manifest unreadable: " + err.Error())
	}
	entries, err = parseManifest(string(data))
	if err != nil {
		panic("posixprovider: embedded manifest is malformed: " + err.Error())
	}
	byName = make(map[string]Entry, len(entries))
	for _, e := range entries {
		byName[e.Command] = e
	}
}

// parseManifest reads the TSV form: comments start with '#', blank lines are
// skipped, and every data row must carry all six columns with a full-length
// sha256. An entry without a digest is REFUSED rather than silently trusted.
func parseManifest(text string) ([]Entry, error) {
	var out []Entry
	sc := bufio.NewScanner(strings.NewReader(text))
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	line := 0
	for sc.Scan() {
		line++
		raw := strings.TrimRight(sc.Text(), "\r")
		if raw == "" || strings.HasPrefix(raw, "#") {
			continue
		}
		f := strings.Split(raw, "\t")
		if len(f) != 6 {
			return nil, fmt.Errorf("line %d: want 6 tab-separated columns, got %d", line, len(f))
		}
		e := Entry{
			Command: strings.TrimSpace(f[0]),
			Version: strings.TrimSpace(f[1]),
			License: strings.TrimSpace(f[2]),
			SHA256:  strings.ToLower(strings.TrimSpace(f[4])),
			URL:     strings.TrimSpace(f[5]),
		}
		for _, p := range strings.Split(f[3], ",") {
			if p = strings.TrimSpace(p); p != "" {
				e.Platforms = append(e.Platforms, p)
			}
		}
		if e.Command == "" || e.Version == "" {
			return nil, fmt.Errorf("line %d: empty command or version", line)
		}
		if len(e.SHA256) != 64 {
			return nil, fmt.Errorf("line %d: %s has no full sha256 pin", line, e.Command)
		}
		if len(e.Platforms) == 0 {
			return nil, fmt.Errorf("line %d: %s declares no platforms", line, e.Command)
		}
		out = append(out, e)
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	if len(out) == 0 {
		return nil, errors.New("no entries")
	}
	return out, nil
}

// ManifestText returns the embedded manifest verbatim, including its comment
// header. The provisioning applet writes it out and points the build recipe at
// it, so a stale copy in a checkout cannot make the recipe and the resolver
// disagree about what is pinned.
func ManifestText() string {
	data, err := manifestFS.ReadFile("manifest.tsv")
	if err != nil {
		return ""
	}
	return string(data)
}

// Names returns every provider command name, sorted.
func Names() []string {
	out := make([]string, 0, len(entries))
	for _, e := range entries {
		out = append(out, e.Command)
	}
	sort.Strings(out)
	return out
}

// Entries returns the manifest rows in file order.
func Entries() []Entry {
	out := make([]Entry, len(entries))
	copy(out, entries)
	return out
}

// Lookup returns the manifest entry for name.
func Lookup(name string) (Entry, bool) {
	e, ok := byName[name]
	return e, ok
}

// Has reports whether name is a POSIX external provider.
func Has(name string) bool {
	_, ok := byName[name]
	return ok
}

// EnabledIn reports whether a given OptOutEnv value leaves the providers
// registered. Only the exact word "off" (case-insensitive) disables them; an
// unset or unrecognised value keeps the default, which is ON.
func EnabledIn(value string) bool {
	return !strings.EqualFold(strings.TrimSpace(value), "off")
}

// Enabled reads OptOutEnv from the process environment. Registration happens in
// init(), before any RunContext exists, so this is the one place the process
// environment is legitimately consulted.
func Enabled() bool { return EnabledIn(os.Getenv(OptOutEnv)) }

// Resolver locates provisioned providers. The zero value is not useful; build
// one with Default, or set the fields directly in a test.
type Resolver struct {
	// CacheRoot is the binmgr cache root (<root>/<cmd>/<version>/<cmd>).
	CacheRoot string
	// GOOS is the platform the manifest is gated against.
	GOOS string
}

// Default builds the Resolver for this process: the binmgr cache root
// ($BASHY_BIN_CACHE, else <UserCacheDir>/bashy/bin) and the running GOOS.
func Default() (Resolver, error) {
	root, err := binmgr.CacheDir()
	if err != nil {
		return Resolver{}, fmt.Errorf("posix provider cache root: %w", err)
	}
	return Resolver{CacheRoot: root, GOOS: runtime.GOOS}, nil
}

// Resolve returns the path to the provisioned provider binary for name.
//
// It NEVER downloads and NEVER compiles — see the package doc. The failure
// modes, in order, are: unknown name, platform not declared, not provisioned
// (the message names the exact provisioning command), and provenance mismatch.
func Resolve(name string) (string, error) {
	r, err := Default()
	if err != nil {
		return "", err
	}
	return r.Resolve(name)
}

// Dir is the cache directory a provider is provisioned into.
func (r Resolver) Dir(e Entry) string {
	return filepath.Join(r.CacheRoot, e.Command, e.Version)
}

// binaryCandidates lists the on-disk names a provider may have been installed
// under. build.sh installs the bare command name; binmgr's convention adds .exe
// on Windows. Both are accepted so a hand-run recipe and the applet agree.
func (r Resolver) binaryCandidates(e Entry) []string {
	dir := r.Dir(e)
	if r.GOOS == "windows" {
		return []string{filepath.Join(dir, e.Command+".exe"), filepath.Join(dir, e.Command)}
	}
	return []string{filepath.Join(dir, e.Command)}
}

// Resolve is the Resolver-scoped form of the package-level Resolve.
func (r Resolver) Resolve(name string) (string, error) {
	id, err := r.VerifiedIdentity(name)
	return id.Path, err
}

// Identity is the verified identity of one provisioned provider: the resolved
// executable, the pinned version it was built from, and the built binary's
// sha256 as recorded — and re-verified against the file — by its provenance
// record. It is what a dispatch-plan probe compares against: two parties that
// agree on an Identity are provably talking about the same binary.
type Identity struct {
	Command     string
	Version     string
	Path        string
	BuiltSHA256 string // lower-case hex, verified equal to the file's digest
}

// VerifiedIdentity resolves name and returns its full verified identity. It
// NEVER downloads and NEVER compiles; the failure modes are exactly Resolve's.
func (r Resolver) VerifiedIdentity(name string) (Identity, error) {
	e, ok := Lookup(name)
	if !ok {
		return Identity{}, fmt.Errorf("%s: %w", name, ErrUnknown)
	}
	if !e.SupportsGOOS(r.GOOS) {
		return Identity{}, fmt.Errorf("%s %s is %w %s (manifest declares: %s)",
			e.Command, e.Version, ErrUnsupportedPlatform, r.GOOS, strings.Join(e.Platforms, ","))
	}
	if r.CacheRoot == "" {
		return Identity{}, fmt.Errorf("%s: no provider cache root configured", e.Command)
	}

	var path string
	for _, c := range r.binaryCandidates(e) {
		if isExecutableFile(c, r.GOOS) {
			path = c
			break
		}
	}
	if path == "" {
		return Identity{}, fmt.Errorf("%s %s is %w: no cached binary under %s\n"+
			"  provision it BEFORE the run:  bashy posix-providers build %s\n"+
			"  (providers are built from pinned upstream source at prepare time; "+
			"resolving one never downloads or compiles)",
			e.Command, e.Version, ErrNotProvisioned, r.Dir(e), e.Command)
	}
	built, err := r.verifyProvenance(e, path)
	if err != nil {
		return Identity{}, err
	}
	return Identity{Command: e.Command, Version: e.Version, Path: path, BuiltSHA256: built}, nil
}

// ProvenancePath is the sidecar build.sh writes next to the binary.
func (r Resolver) ProvenancePath(e Entry) string {
	return filepath.Join(r.Dir(e), "provenance.tsv")
}

// verifyProvenance checks the cached binary against the record build.sh wrote
// and returns the verified built sha256. A cache entry that does not match its
// provenance is an ERROR: it is a binary nobody can attribute to a known
// input, and certification evidence turns on exactly that attribution.
func (r Resolver) verifyProvenance(e Entry, path string) (string, error) {
	provPath := r.ProvenancePath(e)
	rec, err := readProvenance(provPath)
	if err != nil {
		return "", fmt.Errorf("%s %s has a %w: %v\n"+
			"  rebuild it:  bashy posix-providers build %s", e.Command, e.Version, ErrProvenance, err, e.Command)
	}
	mismatch := func(field, got, want string) error {
		return fmt.Errorf("%s %s has a %w: provenance %s is %q, manifest says %q (%s)\n"+
			"  rebuild it:  bashy posix-providers build %s",
			e.Command, e.Version, ErrProvenance, field, got, want, provPath, e.Command)
	}
	if rec["command"] != e.Command {
		return "", mismatch("command", rec["command"], e.Command)
	}
	if rec["version"] != e.Version {
		return "", mismatch("version", rec["version"], e.Version)
	}
	if !strings.EqualFold(rec["source_sha256"], e.SHA256) {
		return "", mismatch("source_sha256", rec["source_sha256"], e.SHA256)
	}
	want := strings.ToLower(strings.TrimSpace(rec["built_sha256"]))
	if len(want) != 64 {
		return "", fmt.Errorf("%s %s has a %w: provenance records no built_sha256 (%s)\n"+
			"  rebuild it:  bashy posix-providers build %s", e.Command, e.Version, ErrProvenance, provPath, e.Command)
	}
	got, err := fileSHA256(path)
	if err != nil {
		return "", fmt.Errorf("%s %s has a %w: %v", e.Command, e.Version, ErrProvenance, err)
	}
	if got != want {
		return "", fmt.Errorf("%s %s has a %w: %s hashes to %s, provenance records %s\n"+
			"  the cached binary is not the one that was built; rebuild it:  bashy posix-providers build %s",
			e.Command, e.Version, ErrProvenance, path, got, want, e.Command)
	}
	return want, nil
}

// readProvenance parses build.sh's two-column key<TAB>value sidecar.
func readProvenance(path string) (map[string]string, error) {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("no provenance record at %s", path)
		}
		return nil, err
	}
	defer f.Close()
	out := map[string]string{}
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		k, v, ok := strings.Cut(strings.TrimRight(sc.Text(), "\r"), "\t")
		if !ok {
			continue
		}
		out[strings.TrimSpace(k)] = strings.TrimSpace(v)
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("empty provenance record at %s", path)
	}
	return out, nil
}

func fileSHA256(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func isExecutableFile(path, goos string) bool {
	fi, err := os.Stat(path)
	if err != nil || fi.IsDir() {
		return false
	}
	if goos == "windows" {
		return true
	}
	return fi.Mode()&0o111 != 0
}

// Status is one provider's provisioning state, for `posix-providers list/check`.
type Status struct {
	Entry     Entry
	Supported bool   // the manifest declares this platform
	Path      string // resolved binary, when Err is nil
	Err       error  // why it is unusable, when it is
}

// Ready reports whether the provider resolved cleanly.
func (s Status) Ready() bool { return s.Err == nil && s.Path != "" }

// Status resolves one provider and reports what happened, without failing.
func (r Resolver) Status(name string) Status {
	e, ok := Lookup(name)
	if !ok {
		return Status{Entry: Entry{Command: name}, Err: fmt.Errorf("%s: %w", name, ErrUnknown)}
	}
	st := Status{Entry: e, Supported: e.SupportsGOOS(r.GOOS)}
	st.Path, st.Err = r.Resolve(name)
	return st
}
