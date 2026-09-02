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

// A ROLE ROOM DECLARES ITS SEAT AS THE DEFAULT ADDRESSEE.
//
// The room is the place a sprint advertises for questions, so an unaddressed
// message there is not addressed to nobody — it belongs to whoever is
// accountable. The label is stored, never the holder: the room outlives its
// conductor by design, and mail that named the person holding the lease when it
// was written would go with them at exactly the handoff the room exists for.
func TestRoleRoomDeclaresItsSeatAsDefaultAddressee(t *testing.T) {
	var got meet.CreateOptions
	prev := createRoom
	createRoom = func(opts meet.CreateOptions) (*meet.State, error) {
		got = opts
		return &meet.State{ID: "room-1", Room: 1}, nil
	}
	t.Cleanup(func() { createRoom = prev })

	if _, err := Assume(role.Assignment{Kind: role.Conductor, Ref: "99"}, "trestle"); err != nil {
		t.Fatal(err)
	}
	if got.DefaultTo != "conductor:99" {
		t.Fatalf("DefaultTo = %q, want the seat label a person types", got.DefaultTo)
	}
	if got.DefaultTo == "trestle" {
		t.Fatal("the room stored its holder — mail would not survive a handover")
	}
}

// A ROOM THAT PREDATES THE FIELD MUST STILL GET ONE.
//
// DefaultTo was added to a codebase that already had rooms. Set only at Create,
// it is inert on every existing room — on this host that was all of them,
// including the single sprint room the feature was written for, so the shipped
// behavior was observably a no-op where it mattered most.
func TestEnsureDefaultToDeclaresTheSeatOnAnExistingRoom(t *testing.T) {
	var gotRef, gotLabel string
	prev := setRoomDefaultTo
	setRoomDefaultTo = func(ref, label string) error {
		gotRef, gotLabel = ref, label
		return nil
	}
	t.Cleanup(func() { setRoomDefaultTo = prev })

	c := &role.Contact{Kind: "meet", Ref: "room-1"}
	if err := EnsureDefaultTo(c, role.Assignment{Kind: role.Conductor, Ref: "99"}); err != nil {
		t.Fatal(err)
	}
	if gotRef != "room-1" || gotLabel != "conductor:99" {
		t.Fatalf("EnsureDefaultTo set (%q, %q), want the room and its seat label", gotRef, gotLabel)
	}
}

// No contact is not an error. A sprint with no room is an ordinary state, and
// healing must never be the thing that reports it.
func TestEnsureDefaultToIsAQuietNoOpWithoutARoom(t *testing.T) {
	prev := setRoomDefaultTo
	setRoomDefaultTo = func(string, string) error {
		t.Fatal("healed a room that does not exist")
		return nil
	}
	t.Cleanup(func() { setRoomDefaultTo = prev })

	for _, c := range []*role.Contact{nil, {Kind: "meet"}, {Kind: "meet", Ref: "  "}} {
		if err := EnsureDefaultTo(c, role.Assignment{Kind: role.Conductor, Ref: "99"}); err != nil {
			t.Fatalf("EnsureDefaultTo(%+v) = %v, want nil", c, err)
		}
	}
}
