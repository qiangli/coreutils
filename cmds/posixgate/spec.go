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
//   - count drift — availability no longer splits 86/14/16, or effective
//     selection no longer splits 78/22/16
//   - duplicate or ambiguous ownership — a name claimed by two dispositions,
//     a shell name shadowed by a registered tool, an applet that is also a
//     pinned provider
//   - missing provider pin or provenance — a provider without a full sha256
//     pin, or a cached binary that does not match its provenance record
//   - a broken root of trust — the externally supplied build/run manifest
//     missing, unreadable, missing a digest, carrying a digest that is not
//     exactly 64 hexadecimal characters, or approved for the OTHER profile
//   - host PATH fallback or an unapproved executable — a staged runtime in
//     which a required name resolves outside the staged tool directory, or
//     resolves to an executable whose digest is not the manifest's approved
//     multicall digest (a staged symlink to an arbitrary host /bin tool
//     fails here)
//   - unbound provider dispatch — the approved multicall's own disclosed
//     dispatch plan disagreeing with the verified provider cache: a valid
//     cache the staged wrapper does not actually dispatch from
//   - a shell resolved from the host PATH, one whose bytes do not hash to
//     the manifest's approved shell build (a --version line is forgeable and
//     is never a build identity), or one whose reported identity is not the
//     profile's — GNU bash exactly 5.3 in both, with the stock -release
//     flavor for Profile C and the Bashy-specific -bashy-<revision> release
//     marker for Profile D
//   - a shell that is not in POSIX mode, or an environment in which
//     POSIXLY_CORRECT=1 does not reach child processes
//
// See docs/posix-owner-gate.md.
package posixgatecmd

import "fmt"

// The spec itself — specRows in spec_gen.go — is a GENERATED projection of the
// canonical expanded POSIX manifest (docs/posix-required-commands.tsv), written
// by scripts/applet-matrix.py. This package maintains no semantic copy of the
// inventory: the generator's --check mode (run by scripts/crossvet.sh and the
// pre-push hook) fails when the projection is stale, and the tests re-derive
// the projection from the canonical manifest and compare.

// Owner is the AVAILABILITY owner of one required name in the assembled
// Profile C/D runtime: who supplies the name's implementation. The vocabulary
// is closed: any other value in the projection is a validation error, never a
// fourth category.
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

// Selector is the EFFECTIVE selection for a name: what a POSIX-mode
// bash-family shell actually selects when the name is invoked at shell level.
// Availability and effective selection differ for exactly eight names — the
// seven builtin overlaps and the `time` keyword — where the shell selects its
// builtin/keyword while the Go applet still backs the name on PATH for
// exec-style callers (env, xargs, find -exec).
type Selector string

const (
	// SelGoApplet — the staged PATH entry (the Go applet) is selected.
	SelGoApplet Selector = "go_applet"
	// SelShellEntry — the name is the shell's own staged executable (`sh`).
	SelShellEntry Selector = "shell_entry"
	// SelShellBuiltin — the shell selects its builtin.
	SelShellBuiltin Selector = "shell_builtin"
	// SelShellKeyword — the shell selects a reserved word (`time`).
	SelShellKeyword Selector = "shell_keyword"
	// SelProvider — the staged PATH entry (the provider wrapper) is selected.
	SelProvider Selector = "external_provider"
)

// The pinned shape of the required inventory, BOTH axes. These are deliberately
// hard constants, not derived values: changing the inventory must fail this
// gate until the pins are updated on purpose, because silent count drift is
// exactly how a certification arm comes to measure something other than what it
// reports. scripts/applet-matrix.py pins the same splits.
const (
	pinTotal = 116
	// availability: who supplies each name (86/14/16)
	pinAvailGoApplets = 86
	pinAvailShell     = 14
	pinProviders      = 16
	// effective selection: what the shell selects (78/22/16); the 22 is the 14
	// shell-owned names plus the seven builtin overlaps and the time keyword
	pinEffectiveGoApplets = 78
	pinEffectiveShell     = 22
)

// specRow is one required name: its availability owner from the canonical
// manifest and the effective selector the staged shell must exhibit.
type specRow struct {
	Command   string
	GoPackage string
	Owner     Owner
	Effective Selector
}

// loadSpec returns the generated projection after validating it. Duplicates,
// unknown dispositions, and owner/selector incoherence are errors: the spec is
// the yardstick, and a bent yardstick must not measure anything.
func loadSpec() ([]specRow, error) {
	if err := validateSpec(specRows); err != nil {
		return nil, err
	}
	out := make([]specRow, len(specRows))
	copy(out, specRows)
	return out, nil
}

// validateSpec rejects a projection whose rows are duplicated, outside the
// closed vocabularies, or incoherent (an availability owner paired with an
// effective selector it can never have). Counts are checked separately by
// verifyInventory so that count drift is reported as findings, not a parse
// abort.
func validateSpec(rows []specRow) error {
	if len(rows) == 0 {
		return fmt.Errorf("spec projection is empty")
	}
	seen := map[string]bool{}
	for _, r := range rows {
		if r.Command == "" {
			return fmt.Errorf("spec projection has a row with an empty command")
		}
		if seen[r.Command] {
			return fmt.Errorf("spec projection lists %q twice", r.Command)
		}
		seen[r.Command] = true
		switch r.Owner {
		case OwnerGoApplet, OwnerShell, OwnerProvider:
		default:
			return fmt.Errorf("spec projection: %s has unknown availability owner %q", r.Command, r.Owner)
		}
		switch r.Effective {
		case SelGoApplet, SelShellEntry, SelShellBuiltin, SelShellKeyword, SelProvider:
		default:
			return fmt.Errorf("spec projection: %s has unknown effective selector %q", r.Command, r.Effective)
		}
		if err := coherent(r); err != nil {
			return err
		}
	}
	return nil
}

// coherent pins the owner/selector pairings that can exist at all. A pairing
// outside this table is a corrupted projection, not a new kind of ownership.
func coherent(r specRow) error {
	ok := false
	switch r.Owner {
	case OwnerShell:
		// `sh` is the entry point; every other shell-owned name is a builtin.
		ok = (r.Command == "sh" && r.Effective == SelShellEntry) ||
			(r.Command != "sh" && r.Effective == SelShellBuiltin)
	case OwnerGoApplet:
		// Plain applet, a builtin overlap, or the time keyword.
		ok = r.Effective == SelGoApplet || r.Effective == SelShellBuiltin || r.Effective == SelShellKeyword
	case OwnerProvider:
		ok = r.Effective == SelProvider
	}
	if !ok {
		return fmt.Errorf("spec projection: %s pairs availability owner %q with effective selector %q, which cannot exist",
			r.Command, r.Owner, r.Effective)
	}
	return nil
}

// expectedShellClass is the `type -t` classification the staged shell must
// report for a name, given its effective selector. Anything else is a
// rejection.
func expectedShellClass(r specRow) string {
	switch r.Effective {
	case SelShellBuiltin:
		return "builtin"
	case SelShellKeyword:
		return "keyword"
	default: // SelGoApplet, SelProvider, SelShellEntry — a staged executable
		return "file"
	}
}

// pathOwned reports whether the staged tool directory must supply this name as
// an executable on PATH: every Go applet and provider (the multicall owns
// those names), plus `sh` itself. Pure shell builtins have no PATH obligation.
func pathOwned(r specRow) bool {
	return r.Owner != OwnerShell || r.Command == "sh"
}

// multicallOwned reports whether the name's staged PATH entry must BE the
// approved multicall (proved by digest identity, not directory membership);
// `sh` is the shell binary and is deliberately excluded — its identity is
// proved by the shell identity/version/build gate instead.
func multicallOwned(r specRow) bool {
	return r.Owner != OwnerShell
}
