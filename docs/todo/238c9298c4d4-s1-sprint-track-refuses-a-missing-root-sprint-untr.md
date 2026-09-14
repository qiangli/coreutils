---
id: 238c9298c4d4
kind: task
title: 'S1: sprint track refuses a missing root; sprint untrack'
seq: 141
status: assigned
priority: p1
created: 2026-09-14T20:26:06.997818Z
assignee: joist
sprint: 180
---

In coreutils/pkg/weave/weave_story_goal.go: normalizeStoryRoot returns an error when the resolved root is not an existing directory (so --repo --plain fails loudly instead of recording <cwd>/--plain); add sprint untrack <sprint> [--repo] as the exact inverse of track (removes the root from StoryRoots, appends a thread note). Tests for both. Gate: go test ./pkg/weave -run Track
