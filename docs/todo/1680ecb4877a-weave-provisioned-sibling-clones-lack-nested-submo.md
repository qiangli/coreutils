---
id: 1680ecb4877a
kind: bug
title: 'weave: provisioned sibling clones lack nested submodules (yoke/external/*/src), so ycode runs fail their verify gate'
seq: 160
status: todo
priority: p2
labels:
    - weave
created: 2026-09-30T16:01:22.923929Z
---

Sprint 340 conductor 2026-09-30: ycode weave #2 verify exited 1 with 'reading ../yoke/external/ollama/src/go.mod: no such file' - the workspace sibling clone of yoke has empty external/{ollama,podman}/src (nested submodules not initialised). The worker had to use a temp GOWORK; the conductor symlinked the umbrella's hydrated dirs into the workspace sibling and ran weave reverify. Fix: when provisioning a replace-sibling, initialise (or link from the source checkout) the nested submodules that the target's go.mod replace chain resolves into. Red/green: weave start in ycode, then go build ./... in the workspace succeeds.
