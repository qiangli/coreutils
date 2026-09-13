---
id: 27c9a78708f5
kind: task
title: 'sprint goal unlink is missing: goal rm can only retire a whole item'
seq: 79
status: todo
priority: p3
created: 2026-09-06T11:30:33.92722Z
sprint: 130
---

sprint goal link adds a story to a goal item; nothing removes one. sprint goal
rm now retires a whole checklist item (story b1f8e659957c), which was the
blocking case, but it is the blunt instrument: a goal covering three stories,
one of which moved to a successor sprint, can only be retired whole or left
permanently unchecked.

Not encountered as a blocker on 2026-09-04..06 - every stranded goal in sprints
123 and 126 had all of its open stories move together, so retiring the item was
the honest operation in each case. Filed because the asymmetry is real and the
next carried-over sprint may not be so tidy.

DO: sprint goal unlink <sprint> <goal> --story <id> [--repo], the exact inverse
of link. It must NOT require it.Sprint == id the way link does - a story that
has moved away is the whole reason to unlink it - and it should resolve a story
that no longer exists at all, so a deleted story can be detached rather than
stranding the item.

Same boundary as goal rm: unlinking says the story no longer covers this
outcome, never that the outcome was met.
