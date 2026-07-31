// Copyright (c) 2025 qiangli
// See LICENSE for licensing information

// The PACKAGE ratchet — the half of the coverage discipline that the name-based
// ratchet structurally could not do.
//
// atlas_coverage_test.go asserts the atlas is in sync with the live registries:
// every registered tool has an entry, every entry is a registered tool. It works,
// and it is why `foreman` has been correctly filed as orchestration/code since
// the stage axis landed (atlas.go's stageTools call, and TestForemanIsRegistered
// below pins it).
//
// But it can only ever check things that HAVE a name in a registry. A capability
// that ships as a package and never acquires a verb is not "uncovered" by that
// test — it is outside its domain entirely. pkg/room (five importers, no verb)
// and pkg/acp (no importers, no verb) both sat in that blind spot. Nothing was
// broken; nothing could have caught them.
//
// So this file ratchets the other axis: the filesystem. Every directory under
// pkg/ must be classified in the census, and the two roles that mean "no verb of
// my own" must say why. Adding a package now fails HERE, by name, until someone
// writes that sentence.
package atlas_test

import (
	"os"
	"strings"
	"testing"

	"github.com/qiangli/coreutils/pkg/atlas"
)

// pkgDirs lists the directories under pkg/ — the ground truth the census is
// ratcheted against. The test runs with cwd = pkg/atlas, so pkg/ is "..".
func pkgDirs(t *testing.T) []string {
	t.Helper()
	entries, err := os.ReadDir("..")
	if err != nil {
		t.Fatalf("read pkg/: %v", err)
	}
	var out []string
	for _, e := range entries {
		name := e.Name()
		if !e.IsDir() || strings.HasPrefix(name, ".") || strings.HasPrefix(name, "_") ||
			name == "testdata" {
			continue
		}
		out = append(out, name)
	}
	if len(out) == 0 {
		t.Fatal("no package directories found under pkg/ — the ratchet would pass vacuously")
	}
	return out
}

// TestCensusCoversEveryPackage is the gap-closer. A capability that never became
// a verb used to be invisible to every check in this package; now it fails here.
func TestCensusCoversEveryPackage(t *testing.T) {
	dirs := pkgDirs(t)
	censused := map[string]bool{}
	for _, n := range atlas.PackageNames() {
		censused[n] = true
	}

	for _, d := range dirs {
		if censused[d] {
			continue
		}
		t.Errorf(`pkg/%s has no atlas census entry.

Every package under pkg/ is classified in pkg/atlas/packages.go, because a
capability that never acquires a verb is otherwise invisible to the one mechanism
designed to ask it "which SDLC stage do you serve that nothing else already
does?" — the question that retired `+"`bashy fanout`"+`. That is exactly how pkg/room
(the substrate under chat/bus/coach, five importers, no verb) and pkg/acp (a
complete client, no importers at all) both went unnoticed.

Add pkg/%s with one of:
  cmdPkg("<verb>")            — it IS a command; the command's Stage is its answer
  libPkg("<verb>", "why")     — real capability, no verb of its own; say why
  supPkg("why")               — plumbing, nothing to place on the spine
  {Role: RoleUnwired, Note:}  — reaches nothing yet; say so out loud`, d, d)
	}

	onDisk := map[string]bool{}
	for _, d := range dirs {
		onDisk[d] = true
	}
	for _, n := range atlas.PackageNames() {
		if !onDisk[n] {
			t.Errorf("atlas census entry %q is stale: no pkg/%s directory", n, n)
		}
	}
}

// TestCensusRecordsAreWellFormed guards the VALUES. The init() self-check panics
// on a malformed record, so this asserts the part init cannot: that a declared
// front door is a command an agent can actually resolve in the atlas.
func TestCensusRecordsAreWellFormed(t *testing.T) {
	roles := sliceSet(atlas.Roles())
	for _, n := range atlas.PackageNames() {
		p, ok := atlas.LookupPackage(n)
		if !ok {
			t.Fatalf("LookupPackage(%q) missing for listed name", n)
		}
		if !roles[p.Role] {
			t.Errorf("pkg/%s: role %q not in vocabulary %v", n, p.Role, atlas.Roles())
		}
		switch p.Role {
		case atlas.RoleCommand, atlas.RoleLibrary:
			if p.FrontDoor == "" {
				t.Errorf("pkg/%s (%s): no front-door command named", n, p.Role)
				continue
			}
			// A front door has to be a name you can type. This is what stops the
			// census from becoming aspirational documentation: you cannot claim
			// a package is reachable through a command that does not exist.
			if _, ok := atlas.Lookup(p.FrontDoor); !ok {
				t.Errorf("pkg/%s: front door %q does not resolve in the atlas — "+
					"a package cannot be surfaced by a command nobody can run", n, p.FrontDoor)
			}
		default:
			if p.FrontDoor != "" {
				t.Errorf("pkg/%s (%s): must not name a front door, has %q", n, p.Role, p.FrontDoor)
			}
		}
		// library/unwired owe the §2.2a answer in prose, since they have no
		// Stage field to carry it.
		if p.Role != atlas.RoleCommand && strings.TrimSpace(p.Note) == "" {
			t.Errorf("pkg/%s (%s): no note. Why is there no command of its own?", n, p.Role)
		}
	}
}

// TestRoomIsRecordedAsALibrary pins the decision this issue made, so a later
// reader finds a claim rather than an absence.
//
// room is the discovery-and-connection substrate under `chat sessions`,
// `chat timeline`, `chat steer`, `coach attach`, bus and weave. It is recorded as
// a LIBRARY whose front door is `chat`, not registered as a verb, because the
// verb table is asserted against live cobra dispatch: a `room` entry with no
// `bashy room` behind it would advertise a command nobody can type. Growing that
// surface (room list / room observe / room timeline) is P0.5 of
// docs/agent-room-mesh-design.md and is a separate change.
func TestRoomIsRecordedAsALibrary(t *testing.T) {
	p, ok := atlas.LookupPackage("room")
	if !ok {
		t.Fatal("pkg/room has no census entry — the substrate under chat/bus/coach is invisible again")
	}
	if p.Role != atlas.RoleLibrary {
		t.Errorf("pkg/room role = %q, want %q. If room has grown a verb of its own, register it in "+
			"the verb table and update this test deliberately.", p.Role, atlas.RoleLibrary)
	}
	if p.FrontDoor != "chat" {
		t.Errorf("pkg/room front door = %q, want \"chat\"", p.FrontDoor)
	}
	if _, ok := atlas.Lookup("room"); ok {
		t.Error(`"room" resolves as a command in the atlas.

room was deliberately recorded as a library, not a verb: the atlas's verb table is
asserted against live cobra dispatch, and a name in the atlas is a promise you can
type it. If ` + "`bashy room`" + ` now genuinely exists (P0.5 of
docs/agent-room-mesh-design.md), delete this check in the same diff that ships it.`)
	}
}

// TestForemanIsRegistered settles a recurring claim: foreman is NOT missing from
// the atlas. It is registered through the TOOL table rather than the verb table —
// pkg/foreman reaches the surface as cmds/foreman, a registered tool.Tool, because
// importing it from cmds/all would form an import cycle (pkg/foreman → pkg/dag,
// whose tests blank-import cmds/all), so bashy registers it directly.
//
// That indirection is what makes it look absent: it is in addTools, not addVerb,
// and addTools files everything as StageCross. atlas.go's stageTools call already
// corrects that to StageCode. This test pins the classification so the next reader
// checking "is foreman registered?" gets an answer instead of a search.
func TestForemanIsRegistered(t *testing.T) {
	e, ok := atlas.Lookup("foreman")
	if !ok {
		t.Fatal("foreman has no atlas entry")
	}
	if e.Group != atlas.GroupOrch {
		t.Errorf("foreman group = %q, want %q", e.Group, atlas.GroupOrch)
	}
	// §2.2a: foreman drives a LIVE, STEERABLE agent session — invoke runs one
	// agent once and returns, weave isolates and merges workspaces, supervise
	// conducts. foreman is the one that keeps a session alive and takes steering
	// while it runs. That is Code, and it is not StageCross: a tool-table entry
	// defaulting to cross is exactly the misfiling stageTools exists to fix.
	if e.Stage != atlas.StageCode {
		t.Errorf("foreman stage = %q, want %q (it drives a live agent session, it is not a "+
			"cross-cutting userland utility)", e.Stage, atlas.StageCode)
	}
	if p, ok := atlas.LookupPackage("foreman"); !ok || p.Role != atlas.RoleCommand || p.FrontDoor != "foreman" {
		t.Errorf("pkg/foreman census = %+v (ok=%v), want command/foreman", p, ok)
	}
}
