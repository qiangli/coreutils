// Copyright (c) 2025 qiangli
// See LICENSE for licensing information

// Package ref is the ONE grammar for naming anything bashy can address.
//
// Every addressable record — a kb page, a todo, a sprint, a weave run, a meet
// room, a board post, a bus event, an agent, a person, a host, a tool, a model,
// a skill, an episode, a role seat — has one canonical ref, `<kind>:<id>`, that
// is the same string in prose (`[[kb:deploy-runbook]]`), on the command line
// (`bashy define kb:deploy-runbook`), in JSON (`"ref": "kb:deploy-runbook"`)
// and in telemetry. `urn:dhnt:<kind>:<id>` is the fully-qualified spelling for
// text that leaves bashy; every parser accepts it, nothing emits it by default.
//
// This package is a LEAF: standard library only, by construction and by test.
// It has to be. The stores that own the kinds (fleet, bus, principal, weave,
// meet, todo, kb, execlog) import each other in a partial order — pkg/kb
// already reaches fleet, bus and principal — so the type they all return from
// Resolve has to sit below every one of them, or the first resolver added to
// fleet is an import cycle. pkg/kb consumes this vocabulary for the prose
// parser and keeps resolving only its own kinds; each store resolves its own;
// the embedding shell (bashy) is the only place that registers all of them.
// Design of record: dhnt docs/uniform-ref-addressing.md; plan:
// docs/sprint-168-master-execution-plan.md (D1–D9).
//
// The vocabulary is CLOSED and ratcheted (TestKindsIsTheVocabulary): adding a
// kind is one table row here plus the owning store's resolver, and nothing
// else. A token whose prefix is not in the table is not a ref — it is a word,
// or a `tool:model` binding such as `codex:gpt5.6-sol`, and the caller keeps
// treating it as one.
package ref

import (
	"errors"
	"fmt"
	"strings"
)

// Kind is the namespace half of a ref: which store owns the id.
type Kind string

// The vocabulary. Order is the canonical listing order (the order the design
// note gives them, then role, added by D2 because the addresser already writes
// to seats and whois already resolves them).
const (
	KB      Kind = "kb"      // a knowledge page, by slug
	Todo    Kind = "todo"    // a todo/issue, by 12-hex id or unique prefix
	Sprint  Kind = "sprint"  // a sprint card, by number
	Run     Kind = "run"     // a weave run, by <repo-basename>-<n>
	Meet    Kind = "meet"    // a meeting/room, by its durable id (never the short room number)
	MB      Kind = "mb"      // a message-board post, by seq
	Bus     Kind = "bus"     // a bus event, by seq
	Agent   Kind = "agent"   // a fleet agent, by name
	Person  Kind = "person"  // a person, by handle
	Host    Kind = "host"    // a host, by name
	Tool    Kind = "tool"    // an agentic CLI, by name
	Model   Kind = "model"   // a model, by canonical name
	Skill   Kind = "skill"   // a skill, by name
	Episode Kind = "episode" // one continuing conversation, by its BASHY_EPISODE id
	Role    Kind = "role"    // an addressable seat (steward, conductor:22), not its holder

	// Unknown is what a parser files a `<scheme>:<rest>` under when the scheme
	// is not in the vocabulary. It is NEVER a member of Kinds() and never
	// resolvable; it exists so a misspelled or foreign scheme is REPORTED
	// rather than dropped or misfiled as a slug.
	Unknown Kind = "unknown"
)

// kinds is the closed table. Kinds() copies it; Known() indexes it.
var kinds = []Kind{KB, Todo, Sprint, Run, Meet, MB, Bus, Agent, Person, Host, Tool, Model, Skill, Episode, Role}

var known = func() map[Kind]bool {
	m := make(map[Kind]bool, len(kinds))
	for _, k := range kinds {
		m[k] = true
	}
	return m
}()

// Kinds returns the closed vocabulary in canonical order. The slice is a copy.
func Kinds() []Kind {
	out := make([]Kind, len(kinds))
	copy(out, kinds)
	return out
}

// KindNames is Kinds() as strings, for help text and error messages.
func KindNames() []string {
	out := make([]string, len(kinds))
	for i, k := range kinds {
		out[i] = string(k)
	}
	return out
}

// Known reports whether k is in the vocabulary. Unknown is not.
func Known(k Kind) bool { return known[k] }

// URNPrefix is the fully-qualified spelling's prefix. NOT bare `urn:<kind>:` —
// there is no registered NID and `urn:sprint:1` is a collision waiting to
// happen (design note D2).
const URNPrefix = "urn:dhnt:"

// legacyPrefix is the spelling pkg/principal has emitted since before this
// grammar existed: `dhnt:<kind>/<name>[@<owner>]`. It is stored in inbox rows
// and read back by weave, so it is accepted as INPUT here and its emission is
// left alone in Sprint 168 (plan D3). Nothing new should write it.
const legacyPrefix = "dhnt:"

// Ref is one parsed reference.
type Ref struct {
	Kind Kind   `json:"kind"`
	ID   string `json:"id"`
	// Qualifier carries what a spelling attached beyond kind+id — today only
	// the legacy `@<owner>` suffix. The canonical form has no place for it and
	// String() drops it; resolvers that care (principal) read it here.
	Qualifier string `json:"qualifier,omitempty"`
	// Raw is the text that was parsed, for reporting. Never used to resolve.
	Raw string `json:"raw,omitempty"`
}

// String is the canonical ref, `<kind>:<id>`.
func (r Ref) String() string { return string(r.Kind) + ":" + r.ID }

// URN is the fully-qualified spelling, `urn:dhnt:<kind>:<id>`.
func (r Ref) URN() string { return URNPrefix + r.String() }

// Format builds a canonical ref from its parts.
func Format(kind Kind, id string) string { return Ref{Kind: kind, ID: id}.String() }

// URN builds the fully-qualified spelling from its parts.
func URN(kind Kind, id string) string { return Ref{Kind: kind, ID: id}.URN() }

// Parse errors. A caller that only wants "is this a ref?" checks the bool from
// Is; a caller that must explain (define) distinguishes the three.
var (
	// ErrNotRef: the token is not shaped like a ref at all — a bare word, or
	// `<x>:<y>` whose prefix is not in the vocabulary. The caller treats it as
	// a word. This is deliberately NOT an error about the token; a
	// `tool:model` binding is a perfectly good token.
	ErrNotRef = errors.New("ref: not a ref")
	// ErrUnknownKind: the token CLAIMED to be a ref (it carried the urn:dhnt:
	// or legacy dhnt: prefix) but named a kind outside the vocabulary. This
	// one is a mistake worth reporting, with the vocabulary.
	ErrUnknownKind = errors.New("ref: unknown kind")
	// ErrEmptyID: a known kind with nothing after the colon.
	ErrEmptyID = errors.New("ref: empty id")
)

// Parse reads one of the three accepted spellings:
//
//	<kind>:<id>                  canonical
//	urn:dhnt:<kind>:<id>         fully qualified
//	dhnt:<kind>/<name>[@owner]   legacy (pkg/principal), input only
//
// A `#` before a numeric-ish id (`todo:#a5f5`, `sprint:#168`) is tolerated the
// way the prose parser always has. Whitespace around the token is trimmed.
//
// Parse never guesses: a prefix that is not in the vocabulary is ErrNotRef, so
// `codex:gpt5.6-sol` stays a binding and `note: remember this` stays prose.
func Parse(s string) (Ref, error) {
	raw := s
	s = strings.TrimSpace(s)
	if s == "" {
		return Ref{}, ErrNotRef
	}
	if rest, ok := strings.CutPrefix(s, URNPrefix); ok {
		return parseCanonical(rest, raw, true)
	}
	if rest, ok := strings.CutPrefix(s, legacyPrefix); ok && strings.Contains(rest, "/") {
		return parseLegacy(rest, raw)
	}
	return parseCanonical(s, raw, false)
}

// parseCanonical handles `<kind>:<id>`. qualified says the caller saw the
// urn:dhnt: prefix, which turns an unknown kind from "not a ref" into an error.
func parseCanonical(s, raw string, qualified bool) (Ref, error) {
	scheme, id, ok := strings.Cut(s, ":")
	if !ok {
		return Ref{}, ErrNotRef
	}
	k := Kind(strings.TrimSpace(scheme))
	if !Known(k) {
		if qualified {
			return Ref{Kind: Unknown, ID: s, Raw: raw}, ErrUnknownKind
		}
		return Ref{}, ErrNotRef
	}
	id = cleanID(k, id)
	if id == "" {
		return Ref{Kind: k, Raw: raw}, ErrEmptyID
	}
	return Ref{Kind: k, ID: id, Raw: raw}, nil
}

// parseLegacy handles `<kind>/<name>[@owner]` after the dhnt: prefix.
func parseLegacy(s, raw string) (Ref, error) {
	scheme, rest, _ := strings.Cut(s, "/")
	k := Kind(strings.TrimSpace(scheme))
	if !Known(k) {
		return Ref{Kind: Unknown, ID: s, Raw: raw}, ErrUnknownKind
	}
	name, owner, _ := strings.Cut(rest, "@")
	name = strings.TrimSpace(name)
	if name == "" {
		return Ref{Kind: k, Raw: raw}, ErrEmptyID
	}
	return Ref{Kind: k, ID: name, Qualifier: strings.TrimSpace(owner), Raw: raw}, nil
}

// cleanID trims and strips the tolerated `#` on the kinds humans number.
func cleanID(k Kind, id string) string {
	id = strings.TrimSpace(id)
	switch k {
	case Todo, Sprint, MB, Bus:
		id = strings.TrimPrefix(id, "#")
	}
	return id
}

// Is reports whether s parses as a ref of a known kind. The convenience for
// callers that only branch on it.
func Is(s string) bool {
	_, err := Parse(s)
	return err == nil
}

// Node is what a resolver returns: enough for a reader to know what the ref
// names, whether it is live, where it lives and how to open it — and nothing
// store-specific, so every kind renders through one printer.
type Node struct {
	Kind   Kind   `json:"kind"`
	ID     string `json:"id"`
	Ref    string `json:"ref"`              // canonical <kind>:<id>, always full
	Title  string `json:"title,omitempty"`  // the record's own title or one-line summary
	Status string `json:"status,omitempty"` // the store's status word (open, done, superseded, live, closed, observed, …)
	Where  string `json:"where,omitempty"`  // where the record lives — a path, a store, a host
	Open   string `json:"open,omitempty"`   // the command that shows the whole record
	// Successor is set when the record was superseded: the ref that replaced
	// it. A superseded ref still resolves (a ref is stable for the record's
	// life, design note D6); this is the pointer forward.
	Successor string `json:"successor,omitempty"`
}

// NewNode fills the redundant Ref field so callers cannot get it wrong.
func NewNode(kind Kind, id string) Node {
	return Node{Kind: kind, ID: id, Ref: Format(kind, id)}
}

// ErrNotFound is what a resolver returns when the store is readable and the id
// is not in it. Any OTHER error means the store could not be read — and the
// two must stay distinct: "not found" is a fact about the record, "cannot
// read" is a fact about this host, and a caller that conflates them reaches a
// confident wrong answer through the absence of evidence.
var ErrNotFound = errors.New("ref: not found")

// Resolver answers for ONE kind. The id is already parsed and cleaned.
type Resolver interface {
	Resolve(id string) (Node, error)
}

// ResolverFunc adapts a function to Resolver.
type ResolverFunc func(id string) (Node, error)

// Resolve implements Resolver.
func (f ResolverFunc) Resolve(id string) (Node, error) { return f(id) }

// Registry maps kinds to their resolvers. The embedding shell fills it; a
// glossary or a store only ever reads it. It is small on purpose — this is not
// an index of refs (the design's non-goal), only of who to ask.
type Registry struct {
	m map[Kind]Resolver
}

// NewRegistry returns an empty registry.
func NewRegistry() *Registry { return &Registry{m: map[Kind]Resolver{}} }

// Register installs r for kind k. Registering an unknown kind is a programming
// error and panics: the vocabulary is closed, and a resolver for a kind the
// parser cannot produce would never be reached.
func (g *Registry) Register(k Kind, r Resolver) {
	if !Known(k) {
		panic(fmt.Sprintf("ref: Register(%q): not in the vocabulary %v", k, KindNames()))
	}
	if g.m == nil {
		g.m = map[Kind]Resolver{}
	}
	g.m[k] = r
}

// Lookup returns the resolver for k, if one is registered.
func (g *Registry) Lookup(k Kind) (Resolver, bool) {
	if g == nil || g.m == nil {
		return nil, false
	}
	r, ok := g.m[k]
	return r, ok
}

// Missing lists the vocabulary kinds with no resolver, in canonical order. The
// embedding shell's coverage test asserts this is empty; define reports a
// missing kind as "no resolver wired", which is a different answer from "not
// found" — a hook left nil must never read as an empty store.
func (g *Registry) Missing() []Kind {
	var out []Kind
	for _, k := range kinds {
		if _, ok := g.Lookup(k); !ok {
			out = append(out, k)
		}
	}
	return out
}

// Resolve parses s and asks the registered resolver. The error tells the
// caller which of the four things happened: not a ref (ErrNotRef), a ref of an
// unknown kind (ErrUnknownKind), a known kind nobody is wired for
// (ErrNoResolver), or the resolver's own answer (ErrNotFound or a store error).
func (g *Registry) Resolve(s string) (Node, error) {
	r, err := Parse(s)
	if err != nil {
		return Node{}, err
	}
	res, ok := g.Lookup(r.Kind)
	if !ok {
		return Node{}, fmt.Errorf("%w: %s", ErrNoResolver, r.Kind)
	}
	n, err := res.Resolve(r.ID)
	if err != nil {
		return Node{}, err
	}
	if n.Ref == "" {
		n.Ref = Format(n.Kind, n.ID)
	}
	return n, nil
}

// ErrNoResolver: the kind is in the vocabulary but this build registered no
// resolver for it.
var ErrNoResolver = errors.New("ref: no resolver wired for kind")
