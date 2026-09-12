---
id: 7c67f648d3d8
kind: task
title: 'S159.2 coreutils: one ladder per store - skills.DefaultStoreDir, execlog.DefaultRoot; graph reads what bashy wrote'
seq: 108
status: done
priority: p1
created: 2026-09-12T19:37:31.066006Z
sprint: 159
closed: 2026-09-12T19:40:55.690049Z
---

coreutils half of S159.2 (umbrella story 080662f5cfe4; design docs/bashy-inspect-design.md section 8, PRIVATE).

skills.DefaultStoreDir is now the ONE ladder for the skills/craft/space store (BASHY_SKILLS_DIR > BASHY_HOME/skills > ~/.config/bashy/skills); pkg/skills defaultConfigDir, pkg/craft defaultStoreDir and cmds/graph spaceStoreDir all resolve through it. Before, spaceStoreDir ended in ~/.bashy/skills - a directory nothing ever wrote - so graph space and graph reached reported an empty store on every host that had one.

execlog.DefaultRoot is now the ONE ladder for the exec history store; cmds/graph execStoreRoot resolves through it (bashy's execHistDir does too, in its own commit).

Gate: cmds/graph TestStorePathsResolveThroughOwners pins both; go test on cmds/graph, pkg/skills, pkg/craft, pkg/execlog green; scripts/ci-test-gate.sh at baseline.
