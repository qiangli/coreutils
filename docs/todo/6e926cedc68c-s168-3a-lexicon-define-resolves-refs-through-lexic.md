---
id: 6e926cedc68c
kind: task
title: 'S168.3a lexicon: define resolves refs through lexicon.RefResolvers (ref.Registry hook); exit codes; --json'
seq: 134
status: done
priority: p1
created: 2026-09-14T02:10:17.978658Z
weave: 24
assignee: qiangli
sprint: 168
closed: 2026-09-14T02:52:09.52548Z
resolution: fixed
closed_by: claude-opus5-x
---

CONTRACT (all Sprint 168 lanes). Read pkg/ref/ref.go FIRST - it is the grammar and the node type; do NOT edit pkg/ref or pkg/kb/links.go (owned by the seam commit f9df88a1). Each store package adds ONE new file resolve.go exposing func RegisterRefs(g *ref.Registry, <the same options its CLI takes>) that registers a ref.ResolverFunc per kind it owns. A resolver returns ref.Node built with ref.NewNode(kind, fullID) and fills Title (the record title or one-line summary), Status (the store status word), Where (path/store/ring), Open (the bashy command that prints the whole record, e.g. "bashy kb show <slug>"). Return ref.ErrNotFound (wrapping is fine) ONLY when the store was readable and the id is absent; any read failure is its own error, never ErrNotFound - the two must stay distinct. Tests are hermetic (t.Setenv BASHY_HOME plus the store dir var) and cover: found, not-found, and the kind-specific edge named below. No new dependencies. Gate before each commit: gofmt, go test ./pkg/<pkg>, scripts/crossvet.sh. COMMIT AS YOU GO (small commits) on the workspace branch with the trailers below as the LAST paragraph; never git stash; no hostnames or /Users paths in code, comments or messages. Plan of record (umbrella, read-only context): docs/sprint-168-master-execution-plan.md D5-D9.

LANE L-B - files: pkg/lexicon/refs.go (new), small edits in pkg/lexicon/cli.go NewDefineCmd (+ tests with FAKE resolvers only - lexicon imports NO store package; the embedding shell fills the registry).
Add: var RefResolvers = ref.NewRegistry() (exported hook, same pattern as Synopses/KnownCommands/RecordDiscovery/SkillSource - see the wireLexicon comment quoted in cli.go about hooks left nil). In NewDefineCmd RunE, BEFORE the existing term path: r, err := ref.Parse(term). Cases:
 (1) err == nil -> RefResolvers.Resolve(term): found -> print the node (text: '<ref>  <title>' then indented status/where/open/successor lines; --json: the ref.Node) exit 0. ref.ErrNotFound -> exit 1 with '<ref>: no <kind> with id <id> here' plus the store listing hint (the Open verb minus the id). ref.ErrNoResolver -> exit 1 with a DISTINCT message 'kind <kind>: no resolver wired on this build' (this is the nil-hook state; it must never read as unknown). Any other error -> exit 1 with the store error verbatim.
 (2) errors.Is(err, ref.ErrUnknownKind) (urn:dhnt:<x>: or dhnt:<x>/) -> exit 1: '<term>: <x> is not a kind; kinds: ' + strings.Join(ref.KindNames(), ' ').
 (3) ref.ErrEmptyID -> exit 1 usage-style message.
 (4) ref.ErrNotRef -> fall through to the EXISTING term path unchanged (bare word, codex:gpt5.6-sol binding, note: x) - exit 0 'unknown here' stays exactly as documented.
Exit 1 must be a returned error or an explicit os.Exit path consistent with how this cobra tree reports errors (check how other exit-1 paths in pkg/lexicon do it; keep stderr/stdout split). --list-kinds should ALSO print the ref vocabulary as a second group (heading 'refs:' then ref.KindNames()). Long/Example text: add 'bashy define kb:deploy-runbook' and 'bashy define urn:dhnt:sprint:168' examples and one paragraph on refs. TestDefineCmd_HasNoSubcommands stays green; add tests: found(0), not-found(1 + hint), unwired kind(1, distinct text), unknown kind(1 + vocabulary), bare word(0, unchanged), 'codex:gpt5.6-sol'(0, existing path), urn spelling for every ref.Kinds() entry against a fake registry.

Trailers for every commit:
Sprint: #168
Story: #<the seq of THIS story, from bashy todo show>
Story-ID: <the 12-hex id of THIS story>
