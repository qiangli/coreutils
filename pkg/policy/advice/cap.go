// Copyright (c) 2025 qiangli
// See LICENSE for licensing information

package advice

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/qiangli/coreutils/pkg/atlas"
)

// Cap is a guard's effect cap: the set of dhnt-6 effect atoms a guarded call
// is allowed to exercise. It is spelled in the PROJECTED vocabulary — the
// dhnt-6 lattice atlas.ProjectEffects targets — not the atlas-11, so a skill
// cap and a guard cap read the same. A command's atlas effects are projected
// before the check; atoms the projection drops (pure, exec, cred, priv,
// persist) are outside what a cap can constrain, by the projection's own
// contract — a consumer that needs them reads the atlas effects unprojected.
//
// Caps only ever narrow. WithCap intersects with any cap already on the
// context, so a nested guard cannot widen what an outer guard allowed —
// deny-only, by construction.
type Cap struct {
	set map[string]bool
}

// capVocabulary is the dhnt-6 effect lattice (the codomain of
// atlas.ProjectEffects plus "time", which the lattice defines but no atlas
// atom projects to). TestCapVocabularyCoversProjection pins the codomain
// relationship.
var capVocabulary = map[string]bool{
	"read": true, "write": true, "net": true, "spend": true, "destroy": true, "time": true,
}

// ParseCap parses a comma-separated effect list ("read,net") into a Cap,
// rejecting empty lists and atoms outside the dhnt-6 vocabulary.
func ParseCap(spec string) (Cap, error) {
	set := map[string]bool{}
	for tok := range strings.SplitSeq(spec, ",") {
		tok = strings.TrimSpace(tok)
		if tok == "" {
			continue
		}
		if !capVocabulary[tok] {
			return Cap{}, fmt.Errorf("unknown effect %q (effects: %s)", tok, strings.Join(vocabularySorted(), ", "))
		}
		set[tok] = true
	}
	if len(set) == 0 {
		return Cap{}, fmt.Errorf("empty effect cap (effects: %s)", strings.Join(vocabularySorted(), ", "))
	}
	return Cap{set: set}, nil
}

func vocabularySorted() []string {
	out := make([]string, 0, len(capVocabulary))
	for k := range capVocabulary {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// Effects returns the cap's atoms, sorted.
func (c Cap) Effects() []string {
	out := make([]string, 0, len(c.set))
	for k := range c.set {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// String renders the cap as its canonical comma-joined sorted spelling.
func (c Cap) String() string { return strings.Join(c.Effects(), ",") }

// Intersect returns the atoms in both caps — the effective cap when a guard
// nests inside another. The zero Cap (no atoms) is absorbing: nothing
// projected is allowed under it.
func (c Cap) Intersect(o Cap) Cap {
	set := map[string]bool{}
	for k := range c.set {
		if o.set[k] {
			set[k] = true
		}
	}
	return Cap{set: set}
}

// Exceeded projects atlasEffects onto the dhnt-6 lattice and returns the
// projected atoms the cap does not allow, sorted; empty means the effects
// fit the cap. This is the guard decision: a non-empty return is a deny
// (audit Record.Decision "deny"), naming exactly which effects exceeded.
func (c Cap) Exceeded(atlasEffects []string) []string {
	var out []string
	for _, e := range atlas.ProjectEffects(atlasEffects) {
		if !c.set[e] {
			out = append(out, e) // ProjectEffects output is sorted + deduped
		}
	}
	return out
}

// capKey carries the effective cap on a context. Unexported: the cap rides
// on the ctx handed to Next() via WithCap/CapFrom only, so no handler-context
// field is added anywhere (design non-goal).
type capKey struct{}

// WithCap returns a context carrying c as the effective cap. If ctx already
// carries a cap, the result carries the INTERSECTION — a nested guard can
// only narrow, never widen, what an outer guard allowed.
func WithCap(ctx context.Context, c Cap) context.Context {
	if outer, ok := CapFrom(ctx); ok {
		c = outer.Intersect(c)
	}
	return context.WithValue(ctx, capKey{}, c)
}

// CapFrom returns the effective cap on ctx, if any.
func CapFrom(ctx context.Context) (Cap, bool) {
	c, ok := ctx.Value(capKey{}).(Cap)
	return c, ok
}
