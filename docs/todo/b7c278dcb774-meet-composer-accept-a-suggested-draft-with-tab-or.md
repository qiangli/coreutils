---
id: b7c278dcb774
kind: task
title: 'Meet composer: accept a suggested draft with Tab or Space instead of pre-filling the box'
seq: 88
status: done
priority: p2
created: 2026-09-07T18:29:05.887551Z
sprint: 135
closed: 2026-09-07T19:21:13.287922Z
---

OBSERVED 2026-09-07 following a New sprint link into the Meet Chat composer.

TODAY. Composer takes `initialDraft` and seeds real state with it: `const [text, setText] = useState(initialDraft)`. The suggestion is therefore INDISTINGUISHABLE FROM TYPING — it is already in the box, already the value that Enter would send, and an operator who wants to write their own message must first select and delete a paragraph somebody else wrote. A suggestion the reader has to erase is a cost, not a help.

WANTED. Render the suggestion as a SUGGESTION and let one keystroke take it.
(a) With the box empty, the suggested text shows as ghost/placeholder text, visually distinct from typed text.
(b) Tab or Space accepts it: the ghost becomes real editable text in the box, cursor at the end, and the suggestion is spent.
(c) Typing any other character overwrites — the ghost disappears and only what was typed remains. No merge, no prefix collision.
(d) Enter on an unaccepted ghost sends NOTHING. The existing rule holds and gets stronger: a draft is never an instruction, and arriving at a page must not be able to send a message.

TRAPS.
- Space is also an ordinary character. The rule must be unambiguous: Space accepts ONLY while the box is empty and a suggestion is showing; after that it types a space like everywhere else.
- Tab is the focus key. Stealing it must not trap keyboard focus in the textarea — when there is no suggestion, Tab must still move focus, and a keyboard-only operator must always be able to leave the composer.
- Accessibility: the ghost is not a value, so a screen reader must be told a suggestion is available and how to take it. Placeholder alone does not carry that.

GATE. A Playwright case in pkg/meet/web/e2e over a composer opened with a draft param: assert the ghost is not the input value, that Tab accepts it, that Space accepts it, that a typed character discards it, and that Enter on an unaccepted ghost posts nothing.
