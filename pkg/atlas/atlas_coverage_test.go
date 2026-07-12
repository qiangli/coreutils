// Copyright (c) 2025 qiangli
// See LICENSE for licensing information

// The atlas coverage ratchet: the tool table must stay exactly in sync with
// the live tool registry (cmds/all + the bashy-only cmds/graph and
// cmds/foreman), vocabularies are closed, and idioms reference only known
// commands. Adding a tool without an atlas entry — or leaving a stale entry
// behind — fails here by name.
package atlas_test

import (
	"sort"
	"testing"

	_ "github.com/qiangli/coreutils/cmds/all"
	_ "github.com/qiangli/coreutils/cmds/foreman"
	_ "github.com/qiangli/coreutils/cmds/graph"

	"github.com/qiangli/coreutils/pkg/atlas"
	"github.com/qiangli/coreutils/tool"
)

func sliceSet(items []string) map[string]bool {
	out := make(map[string]bool, len(items))
	for _, s := range items {
		out[s] = true
	}
	return out
}

func TestToolTableMatchesRegistry(t *testing.T) {
	registered := tool.Names()
	inAtlas := sliceSet(atlas.ToolNames())
	regSet := sliceSet(registered)

	for _, n := range registered {
		if !inAtlas[n] {
			t.Errorf("registered tool %q has no atlas entry (add it to pkg/atlas)", n)
		}
	}
	for _, n := range atlas.ToolNames() {
		if !regSet[n] {
			t.Errorf("atlas tool entry %q is stale: no such registered tool", n)
		}
	}
}

func TestClosedVocabularies(t *testing.T) {
	groups := sliceSet(atlas.Groups())
	tiers := sliceSet(atlas.Tiers())
	stages := sliceSet(atlas.Stages())
	caps := sliceSet(atlas.Capabilities())
	effects := sliceSet(atlas.Effects())

	check := func(names []string) {
		for _, n := range names {
			e, ok := atlas.Lookup(n)
			if !ok {
				t.Fatalf("Lookup(%q) missing for listed name", n)
			}
			if !groups[e.Group] {
				t.Errorf("%s: group %q not in vocabulary", n, e.Group)
			}
			if !tiers[e.Tier] {
				t.Errorf("%s: tier %q not in vocabulary", n, e.Tier)
			}
			// Every entry sits on the SDLC spine. addVerb already panics at init
			// on a missing stage, so this guards the value, not the presence.
			if !stages[e.Stage] {
				t.Errorf("%s: sdlc stage %q not in vocabulary %v", n, e.Stage, atlas.Stages())
			}
			if !sort.StringsAreSorted(e.Caps) {
				t.Errorf("%s: caps not sorted: %v", n, e.Caps)
			}
			seen := map[string]bool{}
			for _, c := range e.Caps {
				if !caps[c] {
					t.Errorf("%s: cap %q not in vocabulary", n, c)
				}
				if seen[c] {
					t.Errorf("%s: duplicate cap %q", n, c)
				}
				seen[c] = true
			}

			// Security-effect classification is MANDATORY. A command with no
			// declared effect is unclassified, and unclassified must fail the
			// build, not fall through as harmless — this is the whole point of
			// the axis. EffPure is the explicit "considered, no governed effect"
			// declaration for the genuinely benign ones (true, echo, seq).
			if len(e.Effects) == 0 {
				t.Errorf("%s: no security effect declared (classify it in pkg/atlas; use %q if genuinely benign)", n, atlas.EffPure)
			}
			if !sort.StringsAreSorted(e.Effects) {
				t.Errorf("%s: effects not sorted: %v", n, e.Effects)
			}
			seenEff := map[string]bool{}
			for _, ef := range e.Effects {
				if !effects[ef] {
					t.Errorf("%s: effect %q not in vocabulary", n, ef)
				}
				if seenEff[ef] {
					t.Errorf("%s: duplicate effect %q", n, ef)
				}
				seenEff[ef] = true
			}
			// EffPure is exclusive: a command is benign OR it has real effects,
			// never both. Catching this keeps "pure" meaningful.
			if seenEff[atlas.EffPure] && len(e.Effects) > 1 {
				t.Errorf("%s: %q cannot be combined with other effects: %v", n, atlas.EffPure, e.Effects)
			}
		}
	}
	check(atlas.ToolNames())
	check(atlas.VerbNames())

	// Tools are userland by definition; tiers beyond it belong to verbs.
	for _, n := range atlas.ToolNames() {
		if e, _ := atlas.Lookup(n); e.Tier != atlas.TierUserland {
			t.Errorf("tool %s: tier %q (tools are userland by definition)", n, e.Tier)
		}
	}
}

func TestAliasTargetsExist(t *testing.T) {
	for _, names := range [][]string{atlas.ToolNames(), atlas.VerbNames()} {
		for _, n := range names {
			e, _ := atlas.Lookup(n)
			if e.AliasOf == "" {
				continue
			}
			if _, ok := atlas.Lookup(e.AliasOf); !ok {
				t.Errorf("%s: alias_of %q does not resolve", n, e.AliasOf)
			}
		}
	}
}

func TestIdiomsReferenceKnownCommands(t *testing.T) {
	// Shell builtins are the embedding shell's to contribute; idioms may
	// name these few without an atlas entry.
	builtinOK := map[string]bool{"cd": true, "trap": true}
	tiers := sliceSet(atlas.Tiers())
	seen := map[string]bool{}
	for _, id := range atlas.Idioms() {
		if id.ID == "" || id.Pattern == "" || id.Note == "" {
			t.Errorf("idiom %+v: id/pattern/note are required", id)
		}
		if seen[id.ID] {
			t.Errorf("duplicate idiom id %q", id.ID)
		}
		seen[id.ID] = true
		if !tiers[id.Tier] {
			t.Errorf("idiom %s: tier %q not in vocabulary", id.ID, id.Tier)
		}
		if len(id.Commands) == 0 {
			t.Errorf("idiom %s: empty commands", id.ID)
		}
		for _, c := range id.Commands {
			if _, ok := atlas.Lookup(c); !ok && !builtinOK[c] {
				t.Errorf("idiom %s: command %q not in atlas", id.ID, c)
			}
		}
	}
}

func TestRegistryEntryDerivation(t *testing.T) {
	e := atlas.RegistryEntry(6)
	if e.Tier != atlas.TierCloud || e.Group != atlas.GroupClusterCloud ||
		e.Subclass != atlas.SubclassManagedExternal {
		t.Errorf("RegistryEntry(6) = %+v, want cloud/cluster-cloud/managed-external", e)
	}
	// A cloud/cluster CLI acts on another host — remote; a tier-2 local tool
	// (ripgrep) is downloaded and run but stays on this box — not remote.
	if !sliceSet(e.Effects)[atlas.EffRemote] {
		t.Errorf("RegistryEntry(6) effects %v missing %q", e.Effects, atlas.EffRemote)
	}
	if sliceSet(atlas.RegistryEntry(2).Effects)[atlas.EffRemote] {
		t.Errorf("RegistryEntry(2) (local tool) must not be %q: %v", atlas.EffRemote, atlas.RegistryEntry(2).Effects)
	}
	if got := atlas.TierName(5); got != atlas.TierCluster {
		t.Errorf("TierName(5) = %q, want cluster", got)
	}
	if got := atlas.TierName(0); got != atlas.TierUserland {
		t.Errorf("TierName(0) = %q, want userland (default)", got)
	}
}
