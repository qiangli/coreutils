package meetroom

import (
	"testing"

	"github.com/qiangli/coreutils/pkg/meet"
	"github.com/qiangli/coreutils/pkg/role"
)

// A ROLE ROOM MUST NEVER PUBLISH MINUTES INTO THE REPO.
//
// meet's default sends minutes to <repo>/docs/meetings/, which suits a meeting a
// person convened. A role room is not that: pkg/role opens one on every assume,
// so the default left a pair of empty minutes in the source tree per lease
// acquire — 28 files in one afternoon in coreutils.
//
// The churn is not why this is pinned. Minutes name their attendees, so each
// file carried a real hostname and a real OS user, and coreutils ships as public
// MIT source. Nothing in the flow prompts anyone to read a generated file before
// `git add`, so the leak is silent by construction — which is exactly the kind
// that needs a test rather than a convention.
func TestAssume_MinutesNeverLandInTheRepo(t *testing.T) {
	if meet.OutStore == "" {
		t.Fatal("OutStore sentinel is empty — minutes would fall back to the repo")
	}
	for _, k := range []role.Kind{role.Steward, role.Conductor} {
		a := role.Assignment{Kind: k, Ref: "r1", Title: "t"}
		c, err := Assume(a, "agent-1")
		if err != nil {
			// Assume needs a usable meet store; skip rather than fail where it
			// cannot run, but never let that hide the sentinel check above.
			t.Skipf("meet unavailable for %v: %v", k, err)
		}
		if c == nil || c.Ref == "" {
			t.Fatalf("%v: no contact returned", k)
		}
		st, _, rerr := meet.Room(c.Ref)
		if rerr != nil || st == nil {
			t.Skipf("cannot read back room: %v", rerr)
		}
		if st.Out != meet.OutStore {
			t.Errorf("%v: Out = %q, want %q — minutes would land in the repo",
				k, st.Out, meet.OutStore)
		}
		_ = Release(c, "agent-1")
	}
}
