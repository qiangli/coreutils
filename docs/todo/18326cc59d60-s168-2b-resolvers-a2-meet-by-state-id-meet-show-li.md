---
id: 18326cc59d60
kind: task
title: 'S168.2b resolvers A2: meet (by State.ID; meet show --links), bus (mb + bus incl. archive; NEW mb show <seq>)'
seq: 132
status: done
priority: p1
created: 2026-09-14T02:10:17.916357Z
weave: 22
assignee: qiangli
sprint: 168
closed: 2026-09-14T02:52:09.467116Z
resolution: fixed
closed_by: codex-gpt-5.5-v
---

CONTRACT (all Sprint 168 lanes). Read pkg/ref/ref.go FIRST - it is the grammar and the node type; do NOT edit pkg/ref or pkg/kb/links.go (owned by the seam commit f9df88a1). Each store package adds ONE new file resolve.go exposing func RegisterRefs(g *ref.Registry, <the same options its CLI takes>) that registers a ref.ResolverFunc per kind it owns. A resolver returns ref.Node built with ref.NewNode(kind, fullID) and fills Title (the record title or one-line summary), Status (the store status word), Where (path/store/ring), Open (the bashy command that prints the whole record, e.g. "bashy kb show <slug>"). Return ref.ErrNotFound (wrapping is fine) ONLY when the store was readable and the id is absent; any read failure is its own error, never ErrNotFound - the two must stay distinct. Tests are hermetic (t.Setenv BASHY_HOME plus the store dir var) and cover: found, not-found, and the kind-specific edge named below. No new dependencies. Gate before each commit: gofmt, go test ./pkg/<pkg>, scripts/crossvet.sh. COMMIT AS YOU GO (small commits) on the workspace branch with the trailers below as the LAST paragraph; never git stash; no hostnames or /Users paths in code, comments or messages. Plan of record (umbrella, read-only context): docs/sprint-168-master-execution-plan.md D5-D9.

LANE A2 - files: pkg/meet/resolve.go, pkg/bus/resolve.go, plus the two CLI additions below (+ tests).
meet: RegisterRefs registers ref.Meet. The identity is State.ID (pkg/meet/session.go); the short Room number is a reused POINTER and must never be the emitted ref - accept it as input only through the existing resolveMeeting so meet:7 and meet show 7 agree, but Node.ID/Ref carry the durable id. Status = open/closed (whatever the state records), Title = State.Name or the topic, Where = the meet store dir (BASHY_MEET_DIR), Open = bashy meet show <id>. Add --links to meet show: parse every transcript body with kb.ParseLinks and print refs with l.Status() semantics (resolved against the cwd repo kb+todo nodes like pkg/todo/cli.go resolveLinks does; external/unknown/dangling otherwise); --json gains a links array with {ref,title,status}. Edge test: a room resolved by its short number yields the same Node as by id.
bus: RegisterRefs registers ref.MB and ref.Bus. mb:<seq> -> the board post (pkg/bus/board.go Post.Seq): Title = first line of the body, Status = read/unread is per-reader so use posted, Where = BoardDir(), Open = bashy mb show <seq>. bus:<seq> -> the room.Event on the host timeline (pkg/room, Seq): Title = type + subject/body first line, Where = room.Dir(), Open = bashy bus watch --json --from <seq> or the closest existing reader. BOTH must also search the archive/ rotation directories (pkg/bus/archive.go, pkg/room/archive.go) - a ref is stable for the record life, so a rotated seq still resolves. NEW subcommand: mb show <seq> [--links] [--json] printing one post (Why, recorded in the command Long text: a post is citable as [[mb:N]] but had no single-record read; --history is the whole board). It adopts the comms canon: --json and --links mean what they mean on todo/sprint show; no new meaning for -n or --all. Edge test: a post/event moved into archive/ still resolves.

Trailers for every commit:
Sprint: #168
Story: #<the seq of THIS story, from bashy todo show>
Story-ID: <the 12-hex id of THIS story>
