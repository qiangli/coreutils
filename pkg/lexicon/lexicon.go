// Copyright (c) 2025 qiangli
// See LICENSE for licensing information

// Package lexicon teaches a fleet of agentic tools a team's jargon.
//
// # The problem
//
// Mid-session a user says: "handoff this to codex." Neither word means what the
// dictionary says. "handoff" is a verb with precise semantics; "codex" is an
// agent binding ON THIS HOST — a CLI tool plus a bound model — and on another
// machine the same word denotes something different. Inside the circle this needs
// no explanation. Outside it, it is meaningless or, worse, plausibly wrong.
//
// # The shape of the answer
//
// The problem is 20% glossary and 80% PRECEDENCE + LOOKUP.
//
//   - Precedence: "in this workspace, these words are never their English senses."
//     One sentence, in the always-on tier every tool reads.
//   - Lookup: a name is RESOLVED, never memorised. `lexicon resolve codex` asks
//     the live registry what that word denotes HERE.
//
// # This package STORES NOTHING
//
// That is the design constraint, and it is the test this package must keep
// passing. A hand-written glossary is the known dead end — stale the moment a
// host's registry changes; the entire data-catalog industry exists because that
// failed. So the term base is PROJECTED from registries that already exist and are
// already maintained:
//
//   - verbs           → the Command Atlas (pkg/atlas): names, SDLC stage, effects
//   - agent bindings  → the fleet registry (pkg/fleet): Name + Aliases + Tool/Model
//   - capabilities    → skills
//
// Only two fields are hand-written, and they are the two a machine cannot infer:
// the ALT LABELS (what the team actually says out loud) and the SCOPE NOTE (the
// precedence rule). Everything that can go stale is generated.
//
// If this package ever starts storing vocabulary rather than projecting it, it has
// become the glossary we said not to build.
//
// # The marker is [[term]], and it belongs in the ARTIFACTS
//
// A jargon word must stand out — but a required sigil in the CONVERSATION rebuilds
// slash commands and breaks the very property that makes jargon useful: inside the
// circle it is used UNMARKED. Nobody says "slash-handoff" in a standup.
//
//	Marked in writing → recognised unmarked in speech.
//
// So [[term]] lives in the artifacts an agent ingests (docs, skills, kb pages,
// briefs, issues), where it TEACHES the term set and makes every mention
// machine-detectable. In conversation the word is plain. `[[term]]` remains
// available as an escape hatch when a human needs to force a resolution —
// optional emphasis, never required syntax.
//
// [[ ]] is not invented here: the kb and the agent memory system already link with
// it, and — decisively — it is the one bracket form that does not collide with
// shell syntax. bashy IS a shell: / # $ @ ! % all mean something to bash.
package lexicon

import (
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/qiangli/coreutils/pkg/atlas"
	"github.com/qiangli/coreutils/pkg/fleet"
)

// Kind is what a concept IS. Borrowed from TBX's concept-orientation, and it is
// what dissolves the dynamic-referent problem: the CONCEPT is "an agent binding
// (tool × model, host-scoped)"; "codex" is merely a TERM that denotes it here.
type Kind string

const (
	KindVerb    Kind = "verb"          // a bashy command
	KindTool    Kind = "agent-tool"    // the CLI a human NAMES ("codex")
	KindBinding Kind = "agent-binding" // what that name DENOTES here: tool × model
	KindSkill   Kind = "skill"         // a capability
)

// The two scope notes that carry the precedence rule for the fleet half. They are
// separate because the failure mode is different: a TOOL name is mistaken for a
// vendor's product, while a BINDING name is mistaken for something portable.
const (
	toolScopeNote = "The CLI on THIS host, with the model bound to it here — not a vendor's " +
		"product in the abstract. On another machine the same word denotes a different binding."
	bindingScopeNote = "An agent binding ON THIS HOST (a CLI tool plus a bound model). " +
		"Host-scoped: the same name may not exist, or may mean something else, elsewhere."
)

// Concept is one entry. The field names are SKOS's on purpose — that vocabulary is
// twenty years old and solved this problem before LLMs existed.
type Concept struct {
	ID         string   `json:"id"`                   // stable: verb:handoff, agent:codex
	Kind       Kind     `json:"kind"`                 //
	PrefLabel  string   `json:"pref_label"`           // the canonical form
	AltLabels  []string `json:"alt_labels,omitempty"` // what people actually say
	Definition string   `json:"definition,omitempty"` //
	ScopeNote  string   `json:"scope_note,omitempty"` // "here, this is NEVER the English word"
	Use        string   `json:"use,omitempty"`        // the command that acts on it
	Source     string   `json:"source"`               // which registry it was projected from
	Host       string   `json:"host,omitempty"`       // bindings are host-scoped; say so
}

// Store is the projected lexicon. It is rebuilt on every call — there is no
// persistence, deliberately.
//
// byTerm maps a term to an INDEX, not a *Concept. That is not a style choice: an
// earlier version stored pointers into s.Concepts, and `append` reallocates the
// backing array — so every pointer taken before a growth silently aliased the OLD
// array. It resolved "codex" to "bashy yarn". Indexes are stable across append;
// pointers into a growing slice are not.
type Store struct {
	Concepts []Concept `json:"concepts"`
	byTerm   map[string]int
}

// Overlay is the ONLY hand-written input: the colloquialisms a team says, and the
// precedence rules. Keyed by concept ID.
type Overlay struct {
	AltLabels  map[string][]string `json:"alt_labels,omitempty" yaml:"alt_labels,omitempty"`
	ScopeNotes map[string]string   `json:"scope_notes,omitempty" yaml:"scope_notes,omitempty"`
}

// DefaultScopeNote is the precedence rule — the sentence that does most of the work
// in this entire feature. Everything else is machinery to keep it true.
const DefaultScopeNote = "In this workspace this is a bashy term, never its English sense. " +
	"Resolve it (`bashy lexicon resolve <term>`) rather than assuming."

// Build projects the registries into a lexicon. It reads; it never writes.
//
// synopses carries the verb one-liners, which live in the embedding shell rather
// than the atlas (the atlas holds classification, not prose). Passing them in keeps
// this package usable by any project, not just bashy.
func Build(cat *fleet.Catalog, synopses map[string]string, host string, ov Overlay) *Store {
	s := &Store{byTerm: map[string]int{}}

	// Verbs — from the Command Atlas.
	for _, name := range atlas.VerbNames() {
		e, ok := atlas.Lookup(name)
		if !ok {
			continue
		}
		if e.AliasOf != "" {
			continue // an alias is a TERM, not a concept; it is attached below
		}
		id := "verb:" + name
		c := Concept{
			ID:         id,
			Kind:       KindVerb,
			PrefLabel:  "bashy " + name,
			AltLabels:  []string{name},
			Definition: synopses[name],
			ScopeNote:  DefaultScopeNote,
			Use:        "bashy " + name,
			Source:     "atlas",
		}
		if e.Stage != "" {
			c.Definition = strings.TrimSpace(c.Definition + " (" + e.Stage + " stage)")
		}
		s.add(c, ov)
	}
	// Atlas aliases (chat → invoke, verify → conform, docker → podman) are extra
	// TERMS for an existing concept, not concepts of their own. Recording them this
	// way is what lets an agent hear the OLD word and resolve the NEW thing.
	for _, name := range atlas.VerbNames() {
		e, ok := atlas.Lookup(name)
		if !ok || e.AliasOf == "" {
			continue
		}
		if i := s.indexOf("verb:" + e.AliasOf); i >= 0 {
			s.Concepts[i].AltLabels = appendUnique(s.Concepts[i].AltLabels, name)
			s.byTerm[strings.ToLower(name)] = i
		}
	}

	// Agent bindings and TOOLS — from the fleet registry. This is the half that is
	// HOST-SPECIFIC, and the reason the lexicon cannot be a checked-in file.
	//
	// Both layers matter, and projecting only one was the first bug this feature
	// hit when dogfooded. The registry names a binding `codex-gpt-5.5`
	// (tool=codex, model=gpt-5.5) — but a human says **"codex"**, meaning "the
	// codex tool, with whatever model is bound to it HERE". The word people
	// actually say names the TOOL; the thing it denotes is the BINDING. A lexicon
	// that projects only the binding fails on the exact sentence it was built for:
	// "handoff this to codex".
	if cat != nil {
		agents, _ := cat.Agents()

		// Which bindings use each tool? This is what turns "codex" into an answer.
		byTool := map[string][]fleet.Agent{}
		for _, a := range agents {
			byTool[a.Tool] = append(byTool[a.Tool], a)
		}

		for _, a := range agents {
			def := a.Description
			if def == "" {
				def = fmt.Sprintf("agent binding: tool=%s model=%s", a.Tool, a.Model)
			}
			s.add(Concept{
				ID:         "agent:" + a.Name,
				Kind:       KindBinding,
				PrefLabel:  a.Name,
				AltLabels:  a.Aliases,
				Definition: def,
				ScopeNote:  bindingScopeNote,
				Use:        "bashy handoff --to " + a.Name,
				Source:     "fleet-registry",
				Host:       host,
			}, ov)
		}

		// The tool layer: the word a human actually says.
		tools, _ := cat.Tools(true)
		for _, t := range tools {
			bindings := byTool[t.Name]
			if len(bindings) == 0 {
				continue // a tool with no binding here denotes nothing here
			}
			names := make([]string, 0, len(bindings))
			for _, b := range bindings {
				names = append(names, fmt.Sprintf("%s (%s)", b.Name, b.Model))
			}
			def := fmt.Sprintf("the %s CLI on this host; bound as: %s", t.Name, strings.Join(names, ", "))
			s.add(Concept{
				ID:         "tool:" + t.Name,
				Kind:       KindTool,
				PrefLabel:  t.Name,
				AltLabels:  t.Aliases,
				Definition: def,
				ScopeNote:  toolScopeNote,
				Use:        "bashy handoff --to " + bindings[0].Name,
				Source:     "fleet-registry",
				Host:       host,
			}, ov)
		}
	}

	// Sorting REORDERS the slice, which invalidates every index in byTerm. Rebuild
	// the map afterwards. (The pointer version of this code had the same hazard and
	// hid it: sort would have left the pointers aimed at the wrong concepts.)
	sort.Slice(s.Concepts, func(i, j int) bool { return s.Concepts[i].ID < s.Concepts[j].ID })
	s.reindex()
	return s
}

// reindex rebuilds the term map from the concept slice. First writer wins, in
// slice order, so the result is deterministic.
func (s *Store) reindex() {
	s.byTerm = map[string]int{}
	for i := range s.Concepts {
		c := &s.Concepts[i]
		if _, taken := s.byTerm[strings.ToLower(c.PrefLabel)]; !taken {
			s.byTerm[strings.ToLower(c.PrefLabel)] = i
		}
		for _, alt := range c.AltLabels {
			if _, taken := s.byTerm[strings.ToLower(alt)]; !taken {
				s.byTerm[strings.ToLower(alt)] = i
			}
		}
	}
}

func (s *Store) add(c Concept, ov Overlay) {
	if extra, ok := ov.AltLabels[c.ID]; ok {
		c.AltLabels = appendUnique(c.AltLabels, extra...)
	}
	if note, ok := ov.ScopeNotes[c.ID]; ok {
		c.ScopeNote = note
	}
	s.Concepts = append(s.Concepts, c)
	i := len(s.Concepts) - 1
	s.byTerm[strings.ToLower(c.PrefLabel)] = i
	for _, alt := range c.AltLabels {
		// First writer wins: a verb's own name must not be stolen by a later
		// binding that happens to share it.
		if _, taken := s.byTerm[strings.ToLower(alt)]; !taken {
			s.byTerm[strings.ToLower(alt)] = i
		}
	}
}

func (s *Store) indexOf(id string) int {
	for i := range s.Concepts {
		if s.Concepts[i].ID == id {
			return i
		}
	}
	return -1
}

func (s *Store) byID(id string) *Concept {
	if i := s.indexOf(id); i >= 0 {
		return &s.Concepts[i]
	}
	return nil
}

// Resolve answers "what does this word mean HERE?" — the 80% of the problem.
// It accepts the bare term, the [[marked]] form, and a namespaced form
// ([[agent:codex]]), because all three appear in the wild.
func (s *Store) Resolve(term string) (*Concept, bool) {
	t := strings.ToLower(strings.TrimSpace(term))
	t = strings.TrimPrefix(t, "[[")
	t = strings.TrimSuffix(t, "]]")
	t = strings.TrimSpace(t)
	if i, ok := s.byTerm[t]; ok {
		return &s.Concepts[i], true
	}
	// Namespaced: agent:codex / verb:handoff
	if c := s.byID(t); c != nil {
		return c, true
	}
	if i := strings.IndexByte(t, ':'); i > 0 {
		if j, ok := s.byTerm[t[i+1:]]; ok {
			return &s.Concepts[j], true
		}
	}
	return nil, false
}

// Terms lists every term that resolves, sorted. This is what an agent's working
// memory gets seeded with — and it must stay SHORT. Tool-selection accuracy
// degrades past ~15-20 items in active rotation, so the always-on projection is a
// selection, never a dump.
func (s *Store) Terms() []string {
	out := make([]string, 0, len(s.byTerm))
	for t := range s.byTerm {
		out = append(out, t)
	}
	sort.Strings(out)
	return out
}

// markerRe finds [[term]] in an artifact. The marker's job is to TEACH: it makes a
// mention machine-detectable in the corpus an agent already reads, so the term set
// is derived from how the team actually writes rather than from a list someone
// maintains.
//
// The token is deliberately strict — letter-initial, no spaces, at most one
// namespace — because the loose version was WRONG and the scan caught it on the
// first run. The claim was "[[ ]] does not collide with shell syntax". True of
// bash parsing prose; FALSE of a shell project's DOCS, which are full of bash:
//
//	[[:alpha:]]     a POSIX character class
//	[[ -z "$x" ]]   a test expression
//	[[match]]       a TOML section header in an example
//
// A leading ':' or '-' now cannot match, and code is skipped entirely (below).
var markerRe = regexp.MustCompile(`\[\[([a-zA-Z][a-zA-Z0-9_-]{0,48}(?::[a-zA-Z][a-zA-Z0-9_-]{0,48})?)\]\]`)

// fencedRe and codeSpanRe strip code before scanning. A [[term]] inside a code
// block is a code sample, not a mention — and in a project whose subject matter IS
// the shell, that distinction is the difference between a useful scan and noise.
var (
	fencedRe   = regexp.MustCompile("(?s)```.*?```")
	codeSpanRe = regexp.MustCompile("`[^`\n]*`")
	indentedRe = regexp.MustCompile(`(?m)^(?:\t|    ).*$`)
)

// stripCode removes fenced blocks, indented blocks, and inline code spans.
func stripCode(text string) string {
	text = fencedRe.ReplaceAllString(text, "")
	text = indentedRe.ReplaceAllString(text, "")
	return codeSpanRe.ReplaceAllString(text, "")
}

// Marked extracts every [[term]] from a text, ignoring anything inside code.
func Marked(text string) []string {
	var out []string
	for _, m := range markerRe.FindAllStringSubmatch(stripCode(text), -1) {
		out = appendUnique(out, strings.TrimSpace(m[1]))
	}
	return out
}

// Unresolved reports marked terms that resolve to nothing — i.e. BROKEN LINKS.
//
// This is what makes the lexicon falsifiable, and it is the property a prose
// glossary can never have: a prose glossary rots silently, a linked one cannot.
func (s *Store) Unresolved(text string) []string {
	var out []string
	for _, t := range Marked(text) {
		if _, ok := s.Resolve(t); !ok {
			out = append(out, t)
		}
	}
	return out
}

func appendUnique(dst []string, vals ...string) []string {
	for _, v := range vals {
		v = strings.TrimSpace(v)
		if v == "" {
			continue
		}
		found := false
		for _, e := range dst {
			if strings.EqualFold(e, v) {
				found = true
				break
			}
		}
		if !found {
			dst = append(dst, v)
		}
	}
	return dst
}
