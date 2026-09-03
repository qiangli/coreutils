package weave

import (
	"context"
	"fmt"
	"strings"

	"github.com/qiangli/coreutils/pkg/room"
)

// StartSprintOwner brings up a durable session for a sprint's owner, so the
// seat is REACHABLE from the moment the sprint starts rather than from whenever
// somebody first happens to write to it.
//
// # Why a seam and not an import
//
// The session is a foreman session (pkg/foreman), and foreman pulls pkg/dag.
// meet already solves this exact shape with injected host seams —
// StartRoomSecretary, StartPermanentRole, ValidateRoomSecretary — wired once in
// bashy's agentos. Following that keeps weave's import graph where it is and
// puts the composition in the one place that already does composition.
//
// nil means the host wired nothing, which is a legitimate configuration: the
// `bash` drop-in links none of this, and a test does not want a model launched.
// A nil seam is reported as "not wired", never treated as a failed launch.
var StartSprintOwner func(context.Context, SprintOwnerRequest) error

// StopSprintOwner tears that session down. An owner session outliving its
// sprint is a leak with a model attached.
var StopSprintOwner func(context.Context, SprintOwnerRequest) error

// SprintOwnerRequest is what the host needs to run, or stop, an owner.
type SprintOwnerRequest struct {
	Sprint int64
	// Owner is the fleet agent name. It is already validated as registered by
	// the time this is called.
	Owner string
	// Brief is the operator's opening instruction, if one was given. Empty is
	// normal: an owner with no brief still needs to exist, it just starts with
	// the sprint's own goal.
	Brief string
	Cwd   string
}

// ensureSprintOwnerSession makes the owner reachable, and reports what it did in
// a fragment meant to be appended to the caller's own line.
//
// IT DOES NOT REFUSE. That is deliberate and it is the whole sequencing lesson
// of this story: an admission gate here already existed once and was reverted,
// because it demanded a transport the owner had no way to supply and stood in
// the way of this project's own conductor five times in one session
// (validateSprintClaimant's comment records it). The gate may return only once
// a refused claim can be REPAIRED automatically — which needs this to work
// first, and needs boot reconciliation after it.
//
// So every outcome here is a note, never an error: already reachable, started,
// not wired, or failed-to-start. The last one is reported in full rather than
// swallowed, because a seat that could not be brought up reads exactly like one
// nobody has written to yet, and the difference matters the moment mail arrives.
func ensureSprintOwnerSession(ctx context.Context, id int64, owner, brief, cwd string) string {
	owner = strings.TrimSpace(owner)
	if owner == "" {
		return ""
	}
	if transport, _ := room.OwnerTransportFor(owner); transport.Deliverable() {
		return fmt.Sprintf("; %s reachable (%s)", owner, transport)
	}
	if StartSprintOwner == nil {
		return fmt.Sprintf("; %s has no live session and no launcher is wired", owner)
	}
	req := SprintOwnerRequest{Sprint: id, Owner: owner, Brief: brief, Cwd: cwd}
	if err := StartSprintOwner(ctx, req); err != nil {
		return fmt.Sprintf("; could not start %s (%v)", owner, err)
	}
	// Report what is TRUE now, not what was attempted. A launcher that returned
	// nil having published nothing would otherwise be recorded as a reachable
	// seat, which is the absence-of-evidence failure this sprint exists to stop.
	if transport, _ := room.OwnerTransportFor(owner); transport.Deliverable() {
		return fmt.Sprintf("; started %s (%s)", owner, transport)
	}
	return fmt.Sprintf("; started %s but it advertises no delivery path yet", owner)
}

// releaseSprintOwnerSession stops the owner's session when the sprint is over.
// Best effort by design: a sprint must be able to end even if the teardown
// cannot, and a failure to stop is reported rather than blocking the end.
func releaseSprintOwnerSession(ctx context.Context, id int64, owner, cwd string) string {
	owner = strings.TrimSpace(owner)
	if owner == "" || StopSprintOwner == nil {
		return ""
	}
	if err := StopSprintOwner(ctx, SprintOwnerRequest{Sprint: id, Owner: owner, Cwd: cwd}); err != nil {
		return fmt.Sprintf("; could not stop %s's session (%v)", owner, err)
	}
	return fmt.Sprintf("; stopped %s's session", owner)
}
