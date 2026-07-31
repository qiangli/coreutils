package craft

// COMPOSE — a skill is rendered on demand, not stored.
//
// There is no authoritative SKILL.md for a composed skill. What a caller gets
// is assembled at query time from the elected implementation plus everything
// learned since, and any file on disk is a CACHE.
//
// # Band is a cut point, not a variant
//
// The same artifact renders at bands 0–4. It is not different content, it is a
// different CUT on nodes that already exist:
//
//	0  pure script — LatExact steps with bindings, runnable with NO model
//	1  script + preconditions and known failures
//	2  imperative steps with the bound commands inline
//	3  contract + effect cap: the WHAT, not the HOW
//	4  intent + contract: maximum latitude
//
// This is the gradual-formalization ladder read backwards — band N renders rung
// (4−N) — so it costs no new vocabulary, no new data, and no authoring burden.
//
// Two consequences follow, and the first is a rule rather than a preference:
//
// A skill can only render at bands its formalization supports, and asking for a
// lower one is a REPORTED MISS, never a fabrication. A model must never
// synthesize a script at render time to satisfy a band request — that would put
// a model back on the read path and destroy reproducibility, which is the whole
// property being bought. Every skill renders at band 4, because intent always
// exists.
//
// And the renderable range is therefore a property worth reporting: lowering the
// floor is exactly what the write path buys.
//
// # On tokens, which is where the intuition misleads
//
// Higher band means FEWER instruction tokens — terse intent beats a long script.
// It also means MORE total tokens, because the model must re-derive the
// procedure. Band 0 costs zero model tokens because there is no model. The
// curves cross almost immediately, which inverts the obvious policy: render at
// the ARTIFACT'S FLOOR, not at the band of the model you happen to have. A
// premium model handed a deterministic script is cheaper, faster, and
// reproducible; high band is the fallback when the artifact genuinely cannot be
// pinned down.

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"

	dhntskills "github.com/dhnt/dhnt/skills"

	"github.com/qiangli/coreutils/pkg/skills"
)

// Band bounds.
const (
	BandScript = 0 // no model at all
	BandIntent = 4 // maximum latitude
)

// ComposeOptions configures a rendering.
type ComposeOptions struct {
	// Band is the cut point. Negative means "the artifact's floor", which is
	// the default and the right policy.
	Band int
	// Coordinate is the space-time context key the composition is for.
	Coordinate string
	// GraphVersion identifies the evidence state this was composed against, so
	// the result is reproducible.
	GraphVersion string
}

// Composition is a rendered skill plus the stamp that makes it reproducible.
type Composition struct {
	Band       int    `json:"band"`
	Capability string `json:"capability"`
	// Identity addresses the IMPLEMENTATION, not this rendering: a composition
	// is derived, and its inputs are stamped below rather than re-hashed into a
	// new identity.
	Identity   string `json:"identity"`
	Name       string `json:"name"`
	Coordinate string `json:"coordinate,omitempty"`
	// GraphVersion + Stamp make a composition exactly reproducible: given the
	// same implementation, coordinate, band, and graph state, the bytes are the
	// same. This is the property a mutable playbook cannot have, and it is what
	// keeps "dynamic" from meaning "non-deterministic".
	GraphVersion string `json:"graph_version,omitempty"`
	Stamp        string `json:"stamp"`
	Body         string `json:"body"`
	// DeterminismRatio is the fraction of contract predicates and steps that are
	// bound to concrete commands — i.e. runnable with no model. The write path's
	// success metric: it must rise as absorption, compression, and binding
	// proceed, and a skill at 1.0 is a script.
	DeterminismRatio float64 `json:"determinism_ratio"`
	// Floor is the lowest band this artifact can render at.
	Floor int `json:"floor"`
	// Bands is the renderable range.
	Bands []int `json:"bands"`
}

// ErrBandUnavailable reports a band below the artifact's floor.
type ErrBandUnavailable struct {
	Want, Floor int
	Why         string
}

func (e *ErrBandUnavailable) Error() string {
	return fmt.Sprintf("craft: cannot render at band %d — the floor is %d (%s). "+
		"A lower band is not synthesized: a model writing a script at render time "+
		"would put a model back on the read path and the result would stop being reproducible",
		e.Want, e.Floor, e.Why)
}

// Compose renders one implementation at a band.
func Compose(im Implementation, opts ComposeOptions) (Composition, error) {
	key, err := skills.CapabilityKey(im.Skill)
	if err != nil {
		return Composition{}, fmt.Errorf("craft: %q states no contract, so there is nothing to compose against: %w", im.Name, err)
	}
	id, err := dhntskills.Identity(im.Skill)
	if err != nil {
		return Composition{}, err
	}

	ratio, bound, total := determinism(im)
	floor, why := floorOf(im, bound, total)

	band := opts.Band
	if band < 0 {
		band = floor // the policy: the artifact's floor, not the model's ceiling
	}
	if band > BandIntent {
		band = BandIntent
	}
	if band < floor {
		return Composition{}, &ErrBandUnavailable{Want: band, Floor: floor, Why: why}
	}

	c := Composition{
		Band:             band,
		Capability:       key,
		Identity:         id,
		Name:             im.Name,
		Coordinate:       opts.Coordinate,
		GraphVersion:     opts.GraphVersion,
		DeterminismRatio: ratio,
		Floor:            floor,
	}
	for b := floor; b <= BandIntent; b++ {
		c.Bands = append(c.Bands, b)
	}
	c.Body = render(im, band)
	c.Stamp = stamp(c)
	return c, nil
}

// determinism reports the fraction of contract predicates and steps bound to
// concrete commands.
func determinism(im Implementation) (ratio float64, bound, total int) {
	for _, chk := range im.Skill.Contract {
		total++
		if boundFor(im, "check-"+chk.Predicate) != "" {
			bound++
		}
	}
	for _, st := range flattenSteps(im.Skill.Steps) {
		total++
		if boundFor(im, "step-"+st.Primitive) != "" {
			bound++
		}
	}
	if total == 0 {
		return 0, 0, 0
	}
	return float64(bound) / float64(total), bound, total
}

func boundFor(im Implementation, key string) string { return im.Bindings[key] }

// flattenSteps walks branches so a conditional's arms count too.
func flattenSteps(steps []dhntskills.Step) []dhntskills.Step {
	var out []dhntskills.Step
	for i := range steps {
		if b := steps[i].Branch; b != nil {
			out = append(out, flattenSteps(b.Then)...)
			out = append(out, flattenSteps(b.Else)...)
			continue
		}
		out = append(out, steps[i])
	}
	return out
}

// floorOf reports the lowest renderable band, and why it is not lower.
func floorOf(im Implementation, bound, total int) (int, string) {
	switch {
	case total > 0 && bound == total:
		return BandScript, ""
	case bound > 0:
		return 2, fmt.Sprintf("%d of %d predicates/steps are bound to commands; a script needs all of them", bound, total)
	case len(im.Skill.Steps) > 0:
		return 2, "steps are declared but none is bound to a concrete command"
	default:
		return 3, "no steps are declared, only a contract"
	}
}

// render cuts the artifact at a band.
func render(im Implementation, band int) string {
	var b strings.Builder
	switch band {
	case 0:
		writeScript(&b, im)
	case 1:
		// Script PLUS what to do when it fails. The commands are NOT repeated
		// here: they are already above, and a small model re-reading the same
		// line twice learns nothing while paying for it.
		writeScript(&b, im)
		b.WriteString("\n# must hold when done — if one fails, that is the step to fix:\n")
		writeContract(&b, im, "# ", false)
	case 2:
		writeSteps(&b, im)
		b.WriteString("\n")
		writeContract(&b, im, "", true)
	case 3:
		// The WHAT, not the HOW. Concrete commands are deliberately withheld:
		// including them would answer the question the band exists to leave
		// open, and a strong model handed the command is no longer choosing a
		// method.
		writeContract(&b, im, "", false)
		writeEffects(&b, im)
	default:
		fmt.Fprintf(&b, "# %s\n\n", im.Name)
		if im.Description != "" {
			fmt.Fprintf(&b, "%s\n\n", im.Description)
		}
		writeContract(&b, im, "", false)
		writeEffects(&b, im)
	}
	return b.String()
}

func writeScript(b *strings.Builder, im Implementation) {
	fmt.Fprintf(b, "#!/usr/bin/env bash\n# %s — composed at band 0; no model required.\nset -euo pipefail\n\n", im.Name)
	for _, st := range flattenSteps(im.Skill.Steps) {
		if cmd := boundFor(im, "step-"+st.Primitive); cmd != "" {
			fmt.Fprintf(b, "%s\n", cmd)
		}
	}
	for _, chk := range im.Skill.Contract {
		if cmd := boundFor(im, "check-"+chk.Predicate); cmd != "" {
			fmt.Fprintf(b, "%s\n", cmd)
		}
	}
}

func writeSteps(b *strings.Builder, im Implementation) {
	fmt.Fprintf(b, "# %s\n\n## Steps\n\n", im.Name)
	steps := flattenSteps(im.Skill.Steps)
	if len(steps) == 0 {
		b.WriteString("_No steps are declared; this skill states only what must hold._\n")
		return
	}
	for i, st := range steps {
		cmd := boundFor(im, "step-"+st.Primitive)
		if cmd != "" {
			fmt.Fprintf(b, "%d. `%s` — %s\n", i+1, cmd, st.Primitive)
			continue
		}
		fmt.Fprintf(b, "%d. %s\n", i+1, st.Primitive)
	}
}

// writeContract renders the postconditions. withCommands controls whether the
// bound command is shown alongside each predicate — the line between a band
// that tells you HOW and one that only tells you WHAT.
func writeContract(b *strings.Builder, im Implementation, prefix string, withCommands bool) {
	if len(im.Skill.Contract) == 0 {
		return
	}
	if prefix == "" {
		fmt.Fprintf(b, "Must hold when done:\n")
	}
	for _, chk := range im.Skill.Contract {
		if cmd := boundFor(im, "check-"+chk.Predicate); withCommands && cmd != "" {
			fmt.Fprintf(b, "%s  - %s  (`%s`)\n", prefix, chk.Predicate, cmd)
			continue
		}
		fmt.Fprintf(b, "%s  - %s\n", prefix, chk.Predicate)
	}
}

func writeEffects(b *strings.Builder, im Implementation) {
	if len(im.Skill.EffectCap) == 0 {
		return
	}
	atoms := make([]string, 0, len(im.Skill.EffectCap))
	for _, e := range im.Skill.EffectCap {
		atoms = append(atoms, e.String())
	}
	sort.Strings(atoms)
	fmt.Fprintf(b, "\nMay not exceed: %s\n", strings.Join(atoms, ", "))
}

// stamp is the reproducibility receipt: the inputs a composition was derived
// from, hashed. Given the same stamp the bytes are the same, which is what makes
// a dynamic artifact auditable rather than merely current.
func stamp(c Composition) string {
	h := sha256.New()
	fmt.Fprintf(h, "impl=%s\ncap=%s\nband=%d\ncoord=%s\ngraph=%s\n",
		c.Identity, c.Capability, c.Band, c.Coordinate, c.GraphVersion)
	return "s" + hex.EncodeToString(h.Sum(nil))[:16]
}
