// Copyright (c) 2025 qiangli
// See LICENSE for licensing information

package lexicon

// THE ACTION FACET — one projection over everything that can be RUN.
//
// A command, a skill, and an agent binding are three registries, three shapes,
// three execution paths. Ask "what happens if I run this, and how much freedom
// does running it take" and today the answer is in three vocabularies. The
// facet is the one shape that question is answered in — and it is a
// PROJECTION, like everything else in this package: computed per call from the
// registry record the concept was already projected from, never stored, never
// a registry of its own. A tool (the CLI a human names) gets no facet, because
// a tool is the EXECUTOR of an action, not an action.
//
// The facet is a NESTED object under a concept. The word `action` on its own is
// a closed vocabulary elsewhere (activity, llmbudget, audit, steward, browser,
// meet), so an emitted document never carries a top-level or string-valued
// `action` key — TestAction_NeverEmitsBareActionKey pins it.

import (
	"strings"

	"github.com/qiangli/coreutils/pkg/atlas"
	"github.com/qiangli/coreutils/pkg/fleet"
	"github.com/qiangli/coreutils/pkg/skills"
)

// ActionFacet is what running a concept amounts to. Field order is the design's.
type ActionFacet struct {
	Kind      string `json:"kind" yaml:"kind"`                             // command | script | dag-target | agent | skill
	Identity  string `json:"identity,omitempty" yaml:"identity,omitempty"` // verb:<name>, a face hash, tool:model
	Contract  string `json:"contract" yaml:"contract"`                     // none | dhnt | dag | metadata-checks
	Latitude  string `json:"latitude" yaml:"latitude"`                     // exact | judge
	Authority string `json:"authority" yaml:"authority"`                   // deterministic | agentic
	// EffectsDeclared is on the dhnt-6 lattice (read write net spend destroy
	// time) — the one vocabulary a cap can be checked against.
	EffectsDeclared []string `json:"effects_declared,omitempty" yaml:"effects_declared,omitempty"`
	// AtlasEffects carries a command's atlas-11 atoms unprojected, for the
	// consumer that needs exec/cred/priv/persist — distinctions the lattice
	// does not draw. Commands only.
	AtlasEffects []string `json:"atlas_effects,omitempty" yaml:"atlas_effects,omitempty"`
	Executor     string   `json:"executor" yaml:"executor"` // builtin | coreutils | path | verb | dhnt | dag | agentlaunch:<tool>
	Envelope     string   `json:"envelope" yaml:"envelope"` // the result shape a run produces
	Scope        string   `json:"scope" yaml:"scope"`       // generic | host
}

// The facet's closed vocabularies.
const (
	ActionCommand   = "command"
	ActionScript    = "script"
	ActionDagTarget = "dag-target"
	ActionAgent     = "agent"
	ActionSkill     = "skill"

	ContractNone     = "none"
	ContractDhnt     = "dhnt"
	ContractDag      = "dag"
	ContractMetadata = "metadata-checks"

	LatitudeExact = "exact"
	LatitudeJudge = "judge"

	AuthorityDeterministic = "deterministic"
	AuthorityAgentic       = "agentic"

	ExecutorBuiltin   = "builtin"
	ExecutorCoreutils = "coreutils"
	ExecutorPath      = "path"
	ExecutorVerb      = "verb"
	ExecutorDhnt      = "dhnt"
	ExecutorDag       = "dag"

	EnvelopeRun    = "bashy-run-v1"
	EnvelopeAttest = "attest-jsonl"
	EnvelopeChat   = "bashy-chat-v1"
	EnvelopeDag    = "dag-v1"

	ScopeGeneric = "generic"
	ScopeHost    = "host"
)

// commandFacet projects an atlas entry. A command is exact and deterministic by
// construction — the shell runs what was typed — so only the effects and the
// executor vary, and both come straight off the record.
func commandFacet(name string, e atlas.Entry, executor string) *ActionFacet {
	return &ActionFacet{
		Kind:            ActionCommand,
		Identity:        "verb:" + name,
		Contract:        ContractNone,
		Latitude:        LatitudeExact,
		Authority:       AuthorityDeterministic,
		EffectsDeclared: atlas.ProjectEffects(e.Effects),
		AtlasEffects:    append([]string(nil), e.Effects...),
		Executor:        executor,
		Envelope:        EnvelopeRun,
		Scope:           ScopeGeneric,
	}
}

// commandExecutor names what runs an atlas entry, from what the resolver
// already knows rather than from a guess: the shell group is reserved for
// builtins whichever table they sit in, and otherwise the table the resolver
// iterated says which executor it is — the in-process tools table is the
// pure-Go userland, the verbs table is the front door.
func commandExecutor(e atlas.Entry, table string) string {
	if e.Group == atlas.GroupShell {
		return ExecutorBuiltin
	}
	return table
}

// skillFacet projects a catalog skill. The contract is the strongest thing the
// skill actually carries: a valid canonical face, else the check-* bindings its
// prose declares, else nothing. Latitude follows the steps — one judge step
// makes the whole run agentic, since the executor may not run it verbatim.
func skillFacet(sk skills.Skill) *ActionFacet {
	f := &ActionFacet{
		Kind:      ActionSkill,
		Contract:  ContractNone,
		Latitude:  LatitudeExact,
		Authority: AuthorityDeterministic,
		Executor:  ExecutorDhnt,
		Envelope:  EnvelopeAttest,
		Scope:     ScopeGeneric,
	}
	switch {
	case sk.Dhnt.Valid():
		f.Identity = sk.Dhnt.Identity
		f.Contract = ContractDhnt
		f.EffectsDeclared = append([]string(nil), sk.Dhnt.EffectCap...)
		if sk.Dhnt.HasJudgeStep {
			f.Latitude = LatitudeJudge
			f.Authority = AuthorityAgentic
		}
	case hasCheckBinding(sk.Meta):
		f.Contract = ContractMetadata
	}
	return f
}

// hasCheckBinding reports a `check` / `check-*` key in SKILL.md metadata — the
// prose-side contract binding (skills.CheckBindingKey is the writer).
func hasCheckBinding(meta map[string]string) bool {
	for k := range meta {
		if k == "check" || strings.HasPrefix(k, "check-") {
			return true
		}
	}
	return false
}

// bindingFacet projects a fleet agent binding. An agent is judge/agentic by
// definition and host-scoped for the same reason its concept is: the binding
// exists HERE.
func bindingFacet(a fleet.Agent) *ActionFacet {
	return &ActionFacet{
		Kind:      ActionAgent,
		Identity:  a.MatrixKey(),
		Contract:  ContractNone,
		Latitude:  LatitudeJudge,
		Authority: AuthorityAgentic,
		Executor:  "agentlaunch:" + a.Tool,
		Envelope:  EnvelopeChat,
		Scope:     ScopeHost,
	}
}

// AddSkills projects a skill catalog's rows into the store.
//
// Separate from Build for the same reason AddSystem is: the skill ring is
// mounted by the embedding shell (its embedded FS is bashy's, not this
// package's), so the caller that has a catalog passes its rows in.
func (s *Store) AddSkills(rows []skills.Skill, ov Overlay) {
	for _, sk := range rows {
		name := strings.TrimSpace(sk.Name)
		if name == "" {
			continue
		}
		def := sk.Description
		if def == "" {
			def = "a skill in this host's catalog"
		}
		s.add(Concept{
			ID: "skill:" + name, Kind: KindSkill, PrefLabel: name,
			Definition: def,
			ScopeNote: "A skill in the catalog mounted on this host — a capability, run by " +
				"name, not the English word.",
			Use:    "bashy skill run " + name,
			Source: "skills",
			Action: skillFacet(sk),
		}, ov)
	}
	s.reindex()
}
