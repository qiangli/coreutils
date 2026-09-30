---
id: 1ecc71124d47
kind: feature
title: 'pure-Go m4 (2/2): diversions, files, system, exit; switch the multicall to the Go applet'
seq: 154
status: assigned
priority: p1
labels:
    - posix-cert
created: 2026-09-30T15:34:39.171598Z
weave: 21
assignee: s340-m4-astra
sprint: 340
sprint_id: 07abaf0d-4c54-57e7-b324-8c926b4c300f
sprint_title: Pure-Go m4, localedef and lp; listing view for the optional external POSIX tools
---

S340 m4 part2 correction8pt/25m. Preserve run21 51c676b2 candidate and prior commits. Independent grade failed exactly TestDifferentialCorpus/diversion-transfer.m4/-s (syncline markers differ from /usr/bin/m4, directives must travel with diversion) and TestAdminHelp stale expectation lp is Apache-2.0 after provider removal. Reproduce and correct both preserving assertions; compare GNU m4 reference if available and identify reference version (Darwin /usr/bin may differ). POSIX -s and diversion semantics decide, no deceptive test weakening. Preserve pure-Go syscmd, rc.Dir/Env, output ordering, wraps/m4exit. Scope cmds/m4 and related provider metadata/test corrections only; no lp/localedef implementation edits. Existing intended combined lp+m4 counts94/14/8 availability86/22/8 selection; lp implementation integrated separately by manager first. Acceptance go test ./cmds/m4 ./cmds/posixgate ./cmds/posixproviders ./pkg/posixprovider, differential corpus actually runs (no silent skip claim), race m4, scripts manifest/matrix/required commands and cross-platform vet. POSIX_BASHY_EVIDENCE_ROOT canonical bashy allowed. Clean room POSIX spec and black-box only, no GNU/glibc/CUPS source copying/translation/paraphrase. Assigned cache only; no hook/sandbox bypass. Actual s340-m4-sol identity Sprint:340 Story:154 Story-ID:1ecc71124d47. No merge/push/install/shared edits; commit partial and exact blockers at25m cap.
