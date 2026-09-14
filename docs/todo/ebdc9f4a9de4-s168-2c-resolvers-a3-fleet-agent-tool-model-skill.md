---
id: ebdc9f4a9de4
kind: task
title: 'S168.2c resolvers A3: fleet (agent tool model skill host), principal (person role; whois ref), execlog (episode)'
seq: 133
status: todo
priority: p1
created: 2026-09-14T02:10:17.947199Z
sprint: 168
---

CONTRACT (all Sprint 168 lanes). Read pkg/ref/ref.go FIRST - it is the grammar and the node type; do NOT edit pkg/ref or pkg/kb/links.go (owned by the seam commit f9df88a1). Each store package adds ONE new file resolve.go exposing func RegisterRefs(g *ref.Registry, <the same options its CLI takes>) that registers a ref.ResolverFunc per kind it owns. A resolver returns ref.Node built with ref.NewNode(kind, fullID) and fills Title (the record title or one-line summary), Status (the store status word), Where (path/store/ring), Open (the bashy command that prints the whole record, e.g. "bashy kb show <slug>"). Return ref.ErrNotFound (wrapping is fine) ONLY when the store was readable and the id is absent; any read failure is its own error, never ErrNotFound - the two must stay distinct. Tests are hermetic (t.Setenv BASHY_HOME plus the store dir var) and cover: found, not-found, and the kind-specific edge named below. No new dependencies. Gate before each commit: gofmt, go test ./pkg/<pkg>, scripts/crossvet.sh. COMMIT AS YOU GO (small commits) on the workspace branch with the trailers below as the LAST paragraph; never git stash; no hostnames or /Users paths in code, comments or messages. Plan of record (umbrella, read-only context): docs/sprint-168-master-execution-plan.md D5-D9.

LANE A3 - files: pkg/fleet/resolve.go, pkg/principal/resolve_ref.go (resolve.go exists), pkg/execlog/resolve.go (+ tests).
fleet: RegisterRefs(g, opts ...Option) registers ref.Agent, ref.Tool, ref.Model, ref.Skill, ref.Host against the Catalog the same options build. Title = display/description, Status = the ring (embedded/shared/cloud/local) or live/retired where the entity has one, Where = the yaml path or ring, Open = bashy agent show <name> / tool show / model show / skill show / (host: bashy whois <name>). Models: a bare family alias (opus) resolves to the derived highest-version record and says so in Title. IMPORTANT: pkg/fleet is a leaf (pkg/fleet/leaf_test.go) - pkg/ref is stdlib-only so importing it is fine; import nothing else new. Edge test: a derived family alias resolves; an unknown name is ErrNotFound.
principal: RegisterRefs registers ref.Person and ref.Role, backed by the existing Resolver (whois). Person -> the contact record (Title = display, Where = the source, Open = bashy whois <name>). Role -> the seat (steward, conductor:22): Title = the current holder if any, Status = held/vacant. ALSO: whois text output gains a ref line (ref: person:<handle> / agent:<name> / host:<name> / role:<label>) and the --json Answer gains a ref field, built with ref.Format. Do NOT change principal.URN() emission or ParseURN (plan D3: the legacy spelling is accepted as input by ref.Parse; emission unification is a later sprint). Edge test: role:steward resolves to the seat and reports its holder or vacant.
execlog: RegisterRefs registers ref.Episode. There is no episode store; an episode is OBSERVED: find the shards <root>/<YYYY-MM-DD>/<episode>.jsonl across day dirs (pkg/execlog/doc.go) - Status = observed, Where = the shard paths joined, Title = first/last timestamp and command count, Open = the existing reader for that episode (bashy activity ... or whatever pkg/execlog exposes; name it exactly). Zero shards = ErrNotFound. The root must be relocatable for tests (find how execlog picks its root; if only via BASHY_HOME, use that). Edge test: two day-dirs with the same episode id -> one Node listing both shards.

Trailers for every commit:
Sprint: #168
Story: #<the seq of THIS story, from bashy todo show>
Story-ID: <the 12-hex id of THIS story>
