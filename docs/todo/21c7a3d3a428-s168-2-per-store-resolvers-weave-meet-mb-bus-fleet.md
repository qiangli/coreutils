---
id: 21c7a3d3a428
kind: task
title: 'S168.2 per-store resolvers: weave/meet/mb/bus/fleet expose Resolve(id); meet show --links, mb show --links'
seq: 130
status: todo
priority: p1
created: 2026-09-13T22:56:21.706423Z
sprint: 168
---

S168.2 (A1/A2/A3, file-disjoint, after W0 types are pushed). Plan: dhnt docs/sprint-168-master-execution-plan.md, D5/D6/D7/D8/D9, traps 2+3+8+9.
Every store returns ref.Node (NOT kb.LinkNode - fleet/bus/principal cannot import kb). One resolve.go per package:
A1: pkg/kb (superseded slug -> status superseded + successor), pkg/todo (12-hex or unique prefix), pkg/weave (sprint:<n>; run:<repo-base>-<n>: current queue first then every queue, TWO hits = error naming both repo paths).
A2: pkg/meet (by State.ID; room number only via existing resolveMeeting; meet show --links over transcript bodies), pkg/bus (mb:<seq> and bus:<seq> from live file AND archive/; NEW mb show <seq> [--links] [--json], Why: a post is citable but has no single-record read).
A3: pkg/fleet (agent tool model skill host), pkg/principal (person role; whois gains ref in text + JSON), pkg/execlog (episode: shards <root>/<date>/<episode>.jsonl + kb journal rows, status observed).
Each Resolve: unit test on a scratch store (BASHY_HOME + the store dir var) for found / not-found / the kind edge. A resolver that cannot enumerate says status unknown-store, never not-found.
