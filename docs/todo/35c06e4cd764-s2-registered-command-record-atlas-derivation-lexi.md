---
id: 35c06e4cd764
kind: task
title: 'S2: registered command record + atlas derivation + lexicon + binmgr pin'
seq: 140
status: assigned
priority: p0
created: 2026-09-14T19:42:32.778604Z
assignee: voussoir
sprint: 179
---

atlas registered.go (OriginRegistered, SubclassRegistered, RegisteredSpec, RegisteredEntry; commands verb caps destructive+json, effects write/destroy/net); lexicon ExecutorRegistered + Build projection; binmgr ResolveURLPinned; fleet 6th noun commands (Command, CommandDownload, Parse/Validate/Argv/Ensure/AtlasSpec, store/catalog/write/schema/allNames, NewCommandsCmd with WithReservedNames/WithCommandProbe, fleettest env). Gate: go test ./pkg/atlas ./pkg/fleet/... ./pkg/lexicon ./pkg/binmgr + scripts/crossvet.sh
