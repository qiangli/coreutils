// Copyright (c) 2025 qiangli
// See LICENSE for licensing information

// Package handoff implements portable, cross-tool, cross-machine session
// handoff: pause a live agent session mid-work, capture everything a successor
// needs, and pass it on — to a different agentic tool, to a scheduler, or to
// tomorrow.
//
// # Why this is not just another /resume
//
// Every agentic tool ships a resume of some kind, and every one of them is a
// prison. Claude Code resumes from ~/.claude/projects/…, Codex from its own
// store, each in a proprietary transcript format, on ONE machine, in ONE tool. A
// session is captive to the tool that made it: you cannot resume a Claude
// session in Codex, and you cannot resume your laptop's session on another host.
//
// bashy is already the SHELL underneath every one of those tools (see
// `bashy install-agent` and chat's forced-shell env), which makes it the one
// layer that can see all of them and belongs to none of them. So it is the layer
// that can own a session format that OUTLIVES the tool that created it.
//
// # The rule that makes it portable
//
// THE RECORD IS AN ARTIFACT, NOT A POINTER.
//
// Nothing in a Record may reference a tool's private session store, a transcript
// id, or a path that means something to only one program. Everything a successor
// needs is IN the record: the prose brief, the structured next action, and a
// self-contained working-state bundle (a patch, not a path). A Record is a file.
// Files travel — over the mesh, over scp, in a commit, in an issue comment.
//
// # What it deliberately does NOT reinvent
//
// The prose continuity brief already exists as the sprint's resume record, and
// the isolation + live-control surface already exists in weave (attach, say,
// kill, status, log, and `start --resume -- <any tool>` into the SAME
// workspace). Handoff composes them; it is a front door and a portable record,
// not a fifth orchestration engine. The one thing genuinely missing — and the
// thing that actually broke a session in this project — is that nothing captured
// the IN-FLIGHT WORKING TREE. A successor inherited a narrative, not a diff.
package handoff

import (
	"time"

	"github.com/qiangli/coreutils/pkg/principal"
)

// SchemaVersion is the wire contract. Additive changes only: an older bashy must
// be able to read a newer record's core fields, because the whole point is that
// the record outlives the process — and often the tool, and sometimes the host —
// that wrote it.
const SchemaVersion = "bashy-handoff-v1"

// Disposition says what should happen to the work now.
type Disposition string

const (
	// DispatchAgent hands the work to another agentic tool, now, in an isolated
	// weave workspace seeded with the working state.
	DispatchAgent Disposition = "agent"
	// DispatchSchedule hands the work to a future wake-up: the brief is carried
	// as the scheduled job's prompt, so the agent arrives WITH the task in hand.
	DispatchSchedule Disposition = "schedule"
	// DispatchPark hands the work to nobody. It waits, intact, until someone
	// runs `bashy resume`. This is the "stop for the day" case and the default:
	// a handoff that names no successor is still a handoff.
	DispatchPark Disposition = "park"
)

// Record is one handoff. It is the entire interchange format — if a field is not
// here, a successor on another machine, in another tool, cannot know it.
type Record struct {
	SchemaVersion string    `json:"schema_version"`
	ID            string    `json:"id"`
	CreatedAt     time.Time `json:"created_at"`

	// From is who is handing off: the tool, the agent nickname, the episode, the
	// host. Resolved via principal.Self(), so a human-launched session is
	// attributed too — the gap that made two colliding sessions invisible to one
	// another in the first place.
	From principal.Ref `json:"from"`

	// Project is the SCOPE, as a path set — never a single .git root. A project
	// spans repos: the bug that prompted this work lived in one repo, the gate
	// that would have caught it in a second, and the pin that carried it in a
	// third. Roots are absolute on the writing host and are treated as HINTS by
	// a reader on another machine, which re-resolves its own.
	Project Project `json:"project"`

	// Continuity is the prose brief a successor reads first: what I was doing,
	// why, what I learned, what is next, what is blocked. It is the same brief
	// the sprint board stores — handoff mirrors it INTO the record rather than
	// keeping a second copy of the truth.
	Continuity string `json:"continuity"`

	// NextAction is the one thing to do next, stated so plainly that a cold
	// agent in a different tool can act without re-deriving the plan. The
	// continuity brief explains; this instructs.
	NextAction string   `json:"next_action,omitempty"`
	Blockers   []string `json:"blockers,omitempty"`

	// Role, when set, names a ROLE the successor should ASSUME before touching
	// the work — a skill name like "steward" or "conductor". A plain handoff
	// passes a TASK ("here is what I was doing"); a role handoff passes the SEAT
	// ("you are now the steward"): the successor loads the skill
	// (`bashy skills show <role>`), acts as that role, and DECIDES how to drive —
	// including whether to delegate the work back. Empty = task handoff. This is
	// the distinction that made "handoff your work" ambiguous: work vs. seat.
	//
	// LAUNCH CONSTRAINT (for whoever wires --to). Two axes separate the seats —
	// SCOPE and MODE:
	//   - steward   = HOST-WIDE + INTERACTIVE, always. The human's continuous
	//                 point of contact across EVERY project on the machine. A
	//                 headless `codex exec`/`--print` is deaf to the human and
	//                 cannot steward, so a steward is NEVER launched headless.
	//   - conductor = PROJECT-scoped (one project, possibly a superproject/
	//                 umbrella spanning repos) + HEADLESS OR INTERACTIVE. The
	//                 execution loop (decompose → isolate → gate → converge to a
	//                 verifier); the GATE is its safety, not dialogue, so either
	//                 mode is fine.
	// A steward owns the host and LAUNCHES conductors (one per project); it does
	// not become one.
	Role string `json:"role,omitempty"`

	// Work is the in-flight state — the piece nothing else captured, and the
	// reason a successor used to inherit a narrative instead of a working tree.
	Work WorkingState `json:"work"`

	// Links point back into the local planes (sprint card, weave issue). They
	// are CONVENIENCES, not the substance: a reader that cannot resolve them
	// must still be able to continue from Continuity + Work alone. That is the
	// artifact-not-pointer rule, applied to our own stores.
	Links Links `json:"links,omitempty"`

	// Dispatch is where it went. Recorded so that a monitoring session — in any
	// tool — can find the successor and watch, steer, or take over.
	Dispatch Dispatch `json:"dispatch"`

	// ResumedAt/ResumedBy are stamped when the record is claimed, so a stale
	// handoff cannot be silently picked up twice.
	ResumedAt *time.Time     `json:"resumed_at,omitempty"`
	ResumedBy *principal.Ref `json:"resumed_by,omitempty"`

	// SupersededAt/SupersededBy retire an UNCLAIMED handoff when a newer one of
	// the SAME ROLE is parked for the same project. A seat is singular — only one
	// live steward handoff should exist at a time — so a bare `bashy resume`
	// finds exactly one live seat instead of an ambiguous pile. Superseded, like
	// resumed, drops off the pending list.
	SupersededAt *time.Time `json:"superseded_at,omitempty"`
	SupersededBy string     `json:"superseded_by,omitempty"`
}

// Project is the scope: a set of roots, plus how they were determined.
type Project struct {
	// Name is a stable label (usually the primary root's basename).
	Name string `json:"name"`
	// Primary is the root the session was working in.
	Primary string `json:"primary"`
	// Roots is the full path set — the primary plus every member the resolver
	// found (sibling replaces, submodules, workspace members). Conflict and
	// scope are decided on path-set INTERSECTION, not root equality.
	Roots []string `json:"roots,omitempty"`
	// Inferred records HOW the set was derived (go.mod-replace, gitmodules,
	// go.work, manifest, …) so a reader can tell a confident answer from a guess.
	Inferred []string `json:"inferred,omitempty"`
}

// WorkingState is the in-flight working tree, captured as a SELF-CONTAINED
// bundle. Not a path. Not a stash ref. Not "see my checkout" — a successor may
// be on another machine, and a stash ref would be a pointer into a store it
// cannot read.
type WorkingState struct {
	// Repo is the root this state belongs to (a project may have several).
	Repo   string `json:"repo"`
	Branch string `json:"branch,omitempty"`
	// BaseSHA is the commit the diff applies to. A successor that cannot find
	// this commit knows immediately that it must fetch, rather than applying a
	// patch to the wrong base and producing nonsense.
	BaseSHA string `json:"base_sha,omitempty"`
	// Diff is `git diff HEAD` — staged AND unstaged, as one patch. The
	// staged/unstaged distinction is deliberately NOT preserved: it is a local
	// index detail, and the index is exactly the shared mutable state that let
	// one session sweep another's staged submodule pins into a commit.
	Diff string `json:"diff,omitempty"`
	// Untracked files are carried by content: a patch does not include them, and
	// they are routinely the whole point (a new file the agent just wrote).
	Untracked []UntrackedFile `json:"untracked,omitempty"`
	// Clean is true when there was nothing in flight — an honest, common case
	// worth recording explicitly rather than inferring from empty fields.
	Clean bool `json:"clean"`
}

// UntrackedFile carries a new file by value.
type UntrackedFile struct {
	Path    string `json:"path"`
	Content string `json:"content"`
	Mode    uint32 `json:"mode,omitempty"`
}

// Links are pointers into the local planes. Optional by contract.
type Links struct {
	Sprint     int    `json:"sprint,omitempty"`
	WeaveIssue int    `json:"weave_issue,omitempty"`
	Workspace  string `json:"workspace,omitempty"`
}

// Dispatch records where the work went.
type Dispatch struct {
	Disposition Disposition `json:"disposition"`
	// To is the successor: a tool name (codex, claude, …) for DispatchAgent, a
	// time expression for DispatchSchedule, empty for DispatchPark.
	To string `json:"to,omitempty"`
	// Note is free text shown to whoever picks this up.
	Note string `json:"note,omitempty"`
}
