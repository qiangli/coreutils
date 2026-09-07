---
id: 2d10a8cebbb7
kind: task
title: 'Sprint board: show a story''s full markdown body, as the todo file actually holds it'
seq: 91
status: todo
priority: p1
created: 2026-09-07T18:29:57.206156Z
sprint: 135
---

OBSERVED 2026-09-07. A story IS a markdown file — docs/todo/<12-hex>-<slug>.md, YAML frontmatter then a body — and the board reaches it correctly: GET /api/sprint/story/{id} -> handleBoardStory -> board.StoryDetail, which returns board.Story with Body. So the plumbing is there and the endpoint is right.

WHAT IS WRONG IS THE RENDERING. openStory() pushes the body through continuitySections(), the splitter written for a CONDUCTOR'S CONTINUITY BRIEF: split on blank lines, and treat a leading ALL-CAPS token before a colon as a section label. That is the correct shape for a continuity record. A story body is ordinary markdown — headings, lists, fenced code, indented command transcripts, links — and this splitter flattens all of it into label/value pairs, silently mangling any paragraph that happens to begin with capitals and a colon and losing every structure that is not a blank-line break. The story files in this repo carry command output and indented blocks; those are exactly what gets destroyed.

WANTED.
(a) The full body renders as what it is. Preserve whitespace and line structure at minimum; render markdown structure if the implementer can do it without pulling in a UI framework or a large dependency (the sprint's no-new-framework rule holds — a small vendored permissive renderer or a deliberate pre/whitespace treatment are both acceptable answers, a React port is not).
(b) Nothing is truncated. If a body is long the pane scrolls; a story detail that quietly stops mid-record is the defect this sprint exists to remove.
(c) The frontmatter fields the pane already shows (status, priority, assignee, sprint, scope, seq) stay where they are — they come from the record, not from the body, and must not be re-parsed out of the markdown.
(d) Untrusted content is untrusted. Bodies are written by agents; render them as text, never as HTML that executes.

KEEP continuitySections for the CONTINUITY section of the sprint card. It is correct there. This story separates the two callers, it does not delete the splitter.

GATE. verifydom over a fixture story whose body contains a heading, a bulleted list, a fenced code block, an indented transcript, and a paragraph starting "NOTE:" — asserting each survives to the DOM, and asserting the sprint card's continuity disclosure still renders its labelled sections unchanged.
