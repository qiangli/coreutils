---
id: 1cfe33f6cd02
kind: task
title: 'S159.1 coreutils: inspect atlas row + alias rows + exported path accessors'
seq: 107
status: todo
priority: p0
created: 2026-09-12T19:13:34.264956Z
sprint: 159
---

coreutils half of S159.1 (umbrella story f1d46cf5c54f; design docs/bashy-inspect-design.md in the umbrella, PRIVATE).

Atlas: addVerb("inspect", diagnostics/cross/[json,read-only]/{read}) with the section 2.2a Why in its comment; doctor/context/audit rows gain AliasOf: "inspect" (the upgrade -> self precedent); inspect joins the read-effect list.

Exported accessors so a read-only resource map can name every store WITHOUT recomputing a path (one line each over the existing private function): meet.BaseDir, weave.StateRoot, weave.SprintStoreDir, secrets.TokenFilePath, chat.SessionsRoot (non-creating split of bashyDir), issue.Store.Dir, kb.RepoContribPath (collapses two literal sites), craft.AttestDir (used by ReadLedger).

Gate: go build ./... and go test on pkg/atlas, meet, weave, secrets, chat, issue, kb, craft green; no behaviour change (every accessor returns what its private twin returned).
