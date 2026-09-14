---
id: 0c3aec1f3e1d
kind: task
title: 'S168.2a resolvers A1: kb (superseded -> successor), todo (prefix), weave sprint + run (two-queue ambiguity = error)'
seq: 131
status: done
priority: p1
created: 2026-09-14T02:10:17.881301Z
weave: 21
assignee: qiangli
sprint: 168
closed: 2026-09-14T02:52:09.435923Z
resolution: fixed
closed_by: claude-opus4.8-u
---

CONTRACT (all Sprint 168 lanes). Read pkg/ref/ref.go FIRST - it is the grammar and the node type; do NOT edit pkg/ref or pkg/kb/links.go (owned by the seam commit f9df88a1). Each store package adds ONE new file resolve.go exposing func RegisterRefs(g *ref.Registry, <the same options its CLI takes>) that registers a ref.ResolverFunc per kind it owns. A resolver returns ref.Node built with ref.NewNode(kind, fullID) and fills Title (the record title or one-line summary), Status (the store status word), Where (path/store/ring), Open (the bashy command that prints the whole record, e.g. "bashy kb show <slug>"). Return ref.ErrNotFound (wrapping is fine) ONLY when the store was readable and the id is absent; any read failure is its own error, never ErrNotFound - the two must stay distinct. Tests are hermetic (t.Setenv BASHY_HOME plus the store dir var) and cover: found, not-found, and the kind-specific edge named below. No new dependencies. Gate before each commit: gofmt, go test ./pkg/<pkg>, scripts/crossvet.sh. COMMIT AS YOU GO (small commits) on the workspace branch with the trailers below as the LAST paragraph; never git stash; no hostnames or /Users paths in code, comments or messages. Plan of record (umbrella, read-only context): docs/sprint-168-master-execution-plan.md D5-D9.

LANE A1 - files: pkg/kb/resolve.go, pkg/todo/resolve.go, pkg/weave/resolve.go (+ _test.go each).
kb: RegisterRefs registers ref.KB. Resolve by slug across the rings the caller can see the way kb show does (repo ring of the cwd, then host). A superseded page still resolves: Status=superseded and Node.Successor = kb:<replacement-slug> (find how supersede records the replacement; publishSuperseded in pkg/kb/bus.go shows the pair). Where = the ring + dir. Open = bashy kb show <slug>. Edge test: superseded page resolves with Successor set.
todo: RegisterRefs registers ref.Todo. Resolve by full 12-hex id or a unique prefix, git-style (the store already does this for todo show); an AMBIGUOUS prefix is an error naming the candidates, not ErrNotFound. Node.ID is the FULL id even when a prefix was given. Status = the todo status. Where = the store dir. Open = bashy todo show <id>. Edge test: prefix resolves to the full id; ambiguous prefix errors.
weave: RegisterRefs registers ref.Sprint and ref.Run. sprint:<n> -> the sprint card (title, stage as Status, Open = bashy sprint show <n>). run:<repo-basename>-<n> -> the weave run: look in the CURRENT checkout queue first, then every queue on the machine (what weave list --all enumerates); exactly one hit resolves (Status = the run state, Where = the repo path, Open = bashy weave status <n>); TWO hits is an error that names both repo paths and never picks one; zero is ErrNotFound. Edge test: two scratch queues whose repos share a basename -> the ambiguity error listing both paths.

Trailers for every commit:
Sprint: #168
Story: #<the seq of THIS story, from bashy todo show>
Story-ID: <the 12-hex id of THIS story>
