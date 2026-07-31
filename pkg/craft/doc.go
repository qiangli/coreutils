// Package craft is the living skill graph: what a host has LEARNED from
// running skills, as opposed to which skills it has.
//
// The division of labour with pkg/skills is the point of the package
// existing at all:
//
//	skills   the CATALOG — an Agent Skills-compatible store you list,
//	         show, add, verify, run, promote, and export. It answers
//	         "what procedures do I have, and do they apply here?"
//	craft    what the catalog ACCUMULATES INTO — evidence gathered
//	         across runs, coordinates, and implementations. It answers
//	         "what has actually held, where, and how often?"
//
// One is the shelf; the other is what the practitioner learned working it.
// craft depends on skills, never the reverse.
//
// # Two keys, two questions
//
// Evidence is indexed both ways, because they answer different things.
//
// By NAME, evidence describes one skill: this procedure, this history.
//
// By CAPABILITY — skills.CapabilityKey, a content address over a skill's
// contract and effect cap with its name and steps projected away — evidence
// describes a PROMISE, pooled across every implementation that makes it. Two
// differently-named skills guaranteeing the same postconditions under the
// same effect bound are the same capability, so their runs are evidence about
// the same claim and belong in one pool rather than two thin ones.
//
// That second key is also what stops a catalog degrading as it grows.
// Semantically-overlapping skills competing as peers at selection time is a
// measured failure mode, not a hypothetical one; skills sharing a capability
// key are alternatives under one heading rather than rivals in one list.
//
// # No new store
//
// craft reads the append-only JSONL that pkg/skills already writes on every
// run. It adds no store of its own, and the derived index is rebuildable in
// full from those logs at any time. A tree that already carries several
// disjoint outcome records does not need another one; what it needed was a
// reader, because the receipts were being written and never read back.
//
// # It reports; it does not decide
//
// Contribution rates are computed and displayed. Nothing is retired,
// elected, or reranked here. Those are policy, they require an evidence
// floor a single host does not reach alone, and acting on a handful of runs
// would discard a sound skill on noise.
package craft
