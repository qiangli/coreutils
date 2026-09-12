---
id: 00ce531cf4ba
kind: task
title: 'S160.1 coreutils: singular canonical nouns in the atlas + cobra roots, plural AliasOf rows, naming ratchet'
seq: 109
status: done
priority: p0
created: 2026-09-12T21:20:51.643803Z
assignee: mortise
sprint: 160
closed: 2026-09-12T21:45:46.520969Z
---

Atlas: add agent/model/tool/skill/secret/app/person as the real rows (same Entry as the plural; Web moves to app); agents/models/tools/skills/secrets/apps/people become AliasOf the singular; issue becomes AliasOf todo; every alias keeps its own Stage + effects (precedent: context/doctor/verify/bootstrap). Census FrontDoor points at the canonical. Cobra root Use: renamed to the singular (fleet/cli.go, principal/people.go, secrets/secrets.go, skills/cli.go, webconsole/cmd.go). Export agentcmd.NewWhoamiCmd. Help/error literals that teach the plural updated (secrets, search, herald, agentctl, agentlaunch, judge, craft, handoff, fleet/cli_write, fleet/principal, lexicon/emit). New pkg/atlas/naming_test.go: TestVerbsAreSingular (VerbNames ending in s or irregular must be AliasOf a non-plural, or in nonPluralEndingInS {bus dks seaweedfs whois}, or pluralListers {commands}); TestAliasMetadataMatchesTarget; TestPackageFrontDoorsAreCanonical. Untouched: store dirs, BASHY_*_DIR, KindPerson, REST, schema strings. Gate: go test ./pkg/atlas/... ./pkg/fleet/... ./pkg/principal/... ./pkg/secrets/... ./pkg/skills/... ./pkg/webconsole/... ./pkg/agentcmd/...
