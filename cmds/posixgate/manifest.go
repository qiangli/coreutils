// Copyright (c) 2025 qiangli
// See LICENSE for licensing information

package posixgatecmd

import (
	"bufio"
	"fmt"
	"os"
	"regexp"
	"strings"
)

// The approved build/run manifest is the runtime gate's ROOT OF TRUST for
// executable identity. It is supplied EXTERNALLY — written at build/release
// time by whoever produced the approved shell and multicall builds — and never
// derived from the staged binaries under test: a digest computed from the very
// binary a caller staged would only prove the binary equals itself. The gate
// verifies both staged executables against these pins, so a caller cannot
// select their own root of trust.
//
// Format: provenance.tsv-style key<TAB>value rows, '#' comments and blank
// lines skipped. Required keys:
//
//	profile          C or D — the profile these builds are approved for
//	shell_sha256     sha256 of the approved staged shell executable
//	multicall_sha256 sha256 of the approved multicall executable
//
// Unknown keys are permitted (a build manifest legitimately records more —
// builder, date, source revisions); missing or malformed required keys are
// rejections. A duplicated required key is a rejection too: two values for one
// root-of-trust pin is a manifest nobody can act on.
type buildManifest struct {
	profile      string // "C" or "D"
	shellSHA     string // lower-case 64-hex
	multicallSHA string // lower-case 64-hex
}

// profiles are the two approved certification profiles. BOTH approve a shell
// that identifies as GNU bash exactly 5.3: Bashy is a bash-5.3 drop-in and its
// staged shell reports the stock "GNU bash, version 5.3…" line — there is no
// bashy-branded version-line prefix (a "bashy, GNU Bash … compatible" banner
// belongs to the bashy front-door command, never to the staged shell). What
// separates the profiles is the RELEASE FLAVOR the build stamps after the
// version digits: an approved stock GNU release carries -release (Profile C),
// a Bashy build carries the Bashy-specific -bashy-<revision> marker, e.g.
// 5.3.0(1)-bashy-dev (Profile D). Each profile rejects the other's flavor.
// The version/flavor checks are a cross-check on top of the manifest digest —
// the digest is the build identity, the version line merely has to agree with
// it.
var profiles = map[string]struct {
	bashy bool // false: stock -release flavor; true: -bashy-<revision> flavor
	human string
}{
	"C": {bashy: false, human: "stock GNU Bash 5.3 (a -" + approvedStockFlavor + " build)"},
	"D": {bashy: true, human: "Bashy 5.3 (GNU bash 5.3 with a -bashy-<revision> release marker)"},
}

// profileKnown reports whether p names an approved certification profile.
func profileKnown(p string) bool {
	_, ok := profiles[p]
	return ok
}

// Both approved profiles run exactly a 5.3 shell: 5.2 is the previous release
// and 5.4 a future one, and neither is the certified configuration. Profile
// C's approved stock builds are -release; alpha, beta, rc, and maint flavors
// are not certified stock releases.
const (
	approvedShellMajor  = 5
	approvedShellMinor  = 3
	approvedStockFlavor = "release"
)

// sha256Re is the ONLY accepted digest shape: exactly 64 hexadecimal
// characters. Truncated, padded, or non-hex values are malformed, never
// "close enough" — a digest that cannot be compared byte-for-byte pins
// nothing.
var sha256Re = regexp.MustCompile(`^[0-9a-fA-F]{64}$`)

// loadBuildManifest reads and validates the externally supplied manifest and
// checks it against the profile the caller claims to be certifying. Every
// defect is a Finding (check "manifest", or "profile" for a profile mismatch):
// with any of them present the gate has no root of trust and must not verify
// anything else.
func loadBuildManifest(path, wantProfile string) (buildManifest, []Finding) {
	var m buildManifest
	f, err := os.Open(path)
	if err != nil {
		return m, []Finding{{Check: "manifest",
			Detail: fmt.Sprintf("cannot read the approved build manifest: %v", err)}}
	}
	defer f.Close()

	var out []Finding
	seen := map[string]bool{}
	sc := bufio.NewScanner(f)
	line := 0
	for sc.Scan() {
		line++
		raw := strings.TrimRight(sc.Text(), "\r")
		if strings.TrimSpace(raw) == "" || strings.HasPrefix(raw, "#") {
			continue
		}
		key, val, ok := strings.Cut(raw, "\t")
		key, val = strings.TrimSpace(key), strings.TrimSpace(val)
		if !ok || key == "" || val == "" {
			out = append(out, Finding{Check: "manifest",
				Detail: fmt.Sprintf("%s line %d: not a key<TAB>value row: %q", path, line, raw)})
			continue
		}
		switch key {
		case "profile", "shell_sha256", "multicall_sha256":
			if seen[key] {
				out = append(out, Finding{Check: "manifest",
					Detail: fmt.Sprintf("%s line %d: duplicate %s", path, line, key)})
				continue
			}
			seen[key] = true
		default:
			continue // extra build metadata is fine; the pins are what is checked
		}
		switch key {
		case "profile":
			m.profile = val
		case "shell_sha256":
			m.shellSHA = strings.ToLower(val)
		case "multicall_sha256":
			m.multicallSHA = strings.ToLower(val)
		}
	}
	if err := sc.Err(); err != nil {
		return m, append(out, Finding{Check: "manifest",
			Detail: fmt.Sprintf("cannot read the approved build manifest %s: %v", path, err)})
	}

	digest := func(key, val string) {
		switch {
		case !seen[key]:
			out = append(out, Finding{Check: "manifest",
				Detail: fmt.Sprintf("%s does not record %s — the approved digest is mandatory, not derivable from the staged binary", path, key)})
		case !sha256Re.MatchString(val):
			out = append(out, Finding{Check: "manifest",
				Detail: fmt.Sprintf("%s records a malformed %s %q: a digest is exactly 64 hexadecimal characters", path, key, val)})
		}
	}
	digest("shell_sha256", m.shellSHA)
	digest("multicall_sha256", m.multicallSHA)

	switch {
	case !seen["profile"]:
		out = append(out, Finding{Check: "manifest",
			Detail: fmt.Sprintf("%s does not record a profile", path)})
	case !profileKnown(m.profile):
		out = append(out, Finding{Check: "manifest",
			Detail: fmt.Sprintf("%s records unknown profile %q (approved profiles: C, D)", path, m.profile)})
	case m.profile != wantProfile:
		out = append(out, Finding{Check: "profile",
			Detail: fmt.Sprintf("gate invoked for profile %s, but %s is a profile %s build manifest — approved builds do not transfer between profiles", wantProfile, path, m.profile)})
	}
	return m, out
}
