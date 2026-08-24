// Copyright (c) 2025 qiangli
// See LICENSE for licensing information

// Package posixgatecmd is the fail-closed effective-owner gate for the
// canonical 116 POSIX-required utility names (the Profile C/D inventory).
//
// # What it proves
//
// The assembled Profile C/D runtime claims that every one of the 116 required
// names is supplied by exactly one INTENDED owner: a registered Bashy Go
// applet, the shell (entry point, builtin, or the `time` keyword), or one of
// the sixteen pinned POSIX external providers. `posix-gate` turns that claim
// into a checkable verdict, and every check is fail-closed: the gate proves
// the intended owner is selected, or it fails naming the name and the cause.
// There is no "probably fine" state — an unverifiable owner is a rejection.
//
// # What it rejects
//
//   - count drift — the inventory no longer has 116 names split 86/14/16
//   - duplicate or ambiguous ownership — a name claimed by two dispositions,
//     a shell name shadowed by a registered tool, an applet that is also a
//     pinned provider
//   - missing provider pin or provenance — a provider without a full sha256
//     pin, or a cached binary that does not match its provenance record
//   - host PATH fallback — a staged runtime in which a required name resolves
//     outside the staged tool directory
//   - a shell that is not in POSIX mode, or an environment in which
//     POSIXLY_CORRECT=1 does not reach child processes
//
// See docs/posix-owner-gate.md.
package posixgatecmd

import (
	"bufio"
	_ "embed"
	"fmt"
	"strings"
)

// The canonical inventory, embedded. scripts/applet-matrix.py writes this file
// and docs/posix-required-commands.tsv from the same rows, and its --check mode
// (run by scripts/crossvet.sh and the pre-push hook) fails when either copy is
// stale — so the gate cannot drift from the documented inventory.
//
//go:embed posix-required-commands.tsv
var specTSV string

// Owner is the intended supplier of one required name in the assembled
// Profile C/D runtime. The vocabulary is closed: any other value in the
// inventory is a parse error, never a fourth category.
type Owner string

const (
	// OwnerGoApplet — a registered Bashy Go coreutils applet supplies the name.
	OwnerGoApplet Owner = "go_applet"
	// OwnerShell — the shell supplies the name: `sh` itself, or a builtin.
	OwnerShell Owner = "shell"
	// OwnerProvider — a pinned POSIX external provider: the multicall owns the
	// name and dispatches to a locally built, provenance-checked upstream copy.
	OwnerProvider Owner = "external_provider"
)

// The pinned shape of the required inventory. These are deliberately hard
// constants, not derived values: changing the inventory must fail this gate
// until the pins are updated on purpose, because silent count drift is exactly
// how a certification arm comes to measure something other than what it
// reports. scripts/applet-matrix.py pins the same split.
const (
	pinTotal     = 116
	pinGoApplets = 86
	pinShell     = 14
	pinProviders = 16
)

// specRow is one required name and its intended owner.
type specRow struct {
	Command   string
	GoPackage string
	Owner     Owner
}

// loadSpec parses the embedded inventory. Duplicates, unknown dispositions,
// and malformed rows are errors: the spec is the yardstick, and a bent
// yardstick must not measure anything.
func loadSpec() ([]specRow, error) {
	return parseSpec(specTSV)
}

func parseSpec(text string) ([]specRow, error) {
	sc := bufio.NewScanner(strings.NewReader(text))
	seen := map[string]bool{}
	var out []specRow
	line := 0
	for sc.Scan() {
		line++
		raw := strings.TrimRight(sc.Text(), "\r")
		if raw == "" {
			continue
		}
		f := strings.Split(raw, "\t")
		if len(f) != 5 {
			return nil, fmt.Errorf("spec line %d: want 5 tab-separated columns, got %d", line, len(f))
		}
		if line == 1 {
			if f[0] != "command" || f[4] != "profile_cd_disposition" {
				return nil, fmt.Errorf("spec line 1: unrecognized header %q", raw)
			}
			continue
		}
		r := specRow{
			Command:   strings.TrimSpace(f[0]),
			GoPackage: strings.TrimSpace(f[2]),
			Owner:     Owner(strings.TrimSpace(f[4])),
		}
		if r.Command == "" {
			return nil, fmt.Errorf("spec line %d: empty command", line)
		}
		if seen[r.Command] {
			return nil, fmt.Errorf("spec line %d: duplicate name %q", line, r.Command)
		}
		seen[r.Command] = true
		switch r.Owner {
		case OwnerGoApplet, OwnerShell, OwnerProvider:
		default:
			return nil, fmt.Errorf("spec line %d: %s has unknown disposition %q", line, r.Command, r.Owner)
		}
		out = append(out, r)
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("spec is empty")
	}
	return out, nil
}

// builtinOverlap lists the go_applet names that a POSIX-mode bash-family shell
// ALSO implements as regular builtins. In the staged runtime the builtin is the
// effective owner of a shell-level invocation, while the Go applet backs the
// same name on PATH for exec-style callers (env, xargs, find -exec). Both facts
// are intended, and the runtime gate verifies both. Any name OUTSIDE this set
// that classifies as a builtin is an ownership violation, not a curiosity.
var builtinOverlap = map[string]bool{
	"echo": true, "false": true, "kill": true, "printf": true,
	"pwd": true, "test": true, "true": true,
}

// keywordOwned: `time` is a reserved word in bash-family shells (pipeline
// timing), so inside the shell the keyword — not the Go applet — is the
// effective owner. The applet still backs the name on PATH.
var keywordOwned = map[string]bool{"time": true}

// expectedShellClass is the `type -t` classification the staged shell must
// report for a name, given its intended owner. Anything else is a rejection.
func expectedShellClass(r specRow) string {
	switch r.Owner {
	case OwnerShell:
		if r.Command == "sh" {
			return "file" // the shell entry point is a staged executable
		}
		return "builtin"
	case OwnerGoApplet:
		if keywordOwned[r.Command] {
			return "keyword"
		}
		if builtinOverlap[r.Command] {
			return "builtin"
		}
		return "file"
	default: // OwnerProvider
		return "file"
	}
}

// pathOwned reports whether the staged tool directory must supply this name as
// an executable on PATH: every Go applet and provider (the multicall owns
// those names), plus `sh` itself. Pure shell builtins have no PATH obligation.
func pathOwned(r specRow) bool {
	return r.Owner != OwnerShell || r.Command == "sh"
}

// multicallOwned reports whether the name's staged PATH entry is expected to be
// the multicall (used by the optional --same-target strictness); `sh` is the
// shell binary and is deliberately excluded.
func multicallOwned(r specRow) bool {
	return r.Owner != OwnerShell
}
