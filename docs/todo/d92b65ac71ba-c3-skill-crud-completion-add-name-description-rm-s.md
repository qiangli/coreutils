---
id: d92b65ac71ba
kind: task
title: 'C3 skill CRUD completion: add <name> --description, rm, set, edit (copy-on-write, embedded immutable)'
seq: 112
status: done
priority: p0
created: 2026-09-12T23:03:30.111336Z
sprint: 161
closed: 2026-09-13T00:33:10.477865Z
---

Skill CRUD completion (today: no rm, no set, no edit; add takes only a dir).
- CREATE without a folder: skill add <name> --description <text> [--requires <expr>] [--body <file>|-] writes the minimal SKILL.md — frontmatter is the mechanism's own format; no template content beyond name/description (rod, not fish).
- skill rm <name>: local ring only; unshadow semantics and wording copied from fleet newRm (cli_write.go:445); an embedded-only name is refused with "shadow it with skill add; embedded entries are immutable".
- skill set <name> [--description] [--requires <expr>] [--binding check-X=<cmd>|step-X=<cmd>]... [--rm-binding X]... [--file <path>=<src>]... [--rm-file <path>]... [--body <file>|-]: materialise from embedded/shared/cloud into local first (copy-on-write, stderr note like newAgentsSet); edit frontmatter via ParseFrontmatter + a new WriteFrontmatter that preserves unknown keys and the body byte-for-byte; re-run the admission gate before save.
- skill edit <name>: $VISUAL/$EDITOR on the materialised SKILL.md (copy fleet newEdit).
- Atlas: if `destructive` is now honest for skill (pkg/atlas/atlas.go:904), align all four nouns (tool/model/agent rows at 865-867 already have rm without it) or none — record which.
Tests: add-from-flags produces a skill that list admits; rm/unshadow; set materialises and preserves unknown frontmatter + body; requires validated via spacetime.ParseRequires; embedded refusal.
Gate: go test ./pkg/skills/... ./pkg/atlas/...
