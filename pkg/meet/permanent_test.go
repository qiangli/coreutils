package meet

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func permanentTestStore(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("BASHY_MEET_DIR", dir)
	t.Setenv("BASHY_MEET_ROOMS_FILE", "")
	t.Setenv("USER", "tester")
	return dir
}

func TestConfiguredPermanentRoomsIncludeStewardByDefault(t *testing.T) {
	permanentTestStore(t)
	rooms, err := ConfiguredPermanentRooms()
	if err != nil {
		t.Fatal(err)
	}
	if len(rooms) != 1 || rooms[0].Name != "steward" || rooms[0].Topic != "Steward" {
		t.Fatalf("default rooms = %+v, want steward", rooms)
	}
}

func TestConfiguredPermanentRoomsMergeHostConfig(t *testing.T) {
	dir := permanentTestStore(t)
	p := filepath.Join(dir, "rooms.json")
	b, _ := json.Marshal(permanentRoomsFile{
		Schema: permanentRoomsSchema,
		Rooms: []PermanentRoomConfig{
			{Name: "steward", Topic: "Dragon steward"},
			{Name: "release", Topic: "Release room", Agenda: []string{"release gate"}},
		},
	})
	if err := os.WriteFile(p, b, 0o600); err != nil {
		t.Fatal(err)
	}
	rooms, err := ConfiguredPermanentRooms()
	if err != nil {
		t.Fatal(err)
	}
	if len(rooms) != 2 || rooms[0].Name != "release" || rooms[1].Topic != "Dragon steward" {
		t.Fatalf("merged rooms = %+v", rooms)
	}
}

func TestEnsurePermanentRoomIsStableNamedAndCannotClose(t *testing.T) {
	permanentTestStore(t)
	first, err := EnsurePermanentRoom("steward", CreateOptions{Topic: "Steward", Out: OutStore})
	if err != nil {
		t.Fatal(err)
	}
	second, err := EnsurePermanentRoom("@steward", CreateOptions{Topic: "Host steward", Out: OutStore})
	if err != nil {
		t.Fatal(err)
	}
	if first.ID != second.ID {
		t.Fatalf("ensure created two identities: %s and %s", first.ID, second.ID)
	}
	if !second.Permanent || second.Name != "steward" || second.Topic != "Host steward" {
		t.Fatalf("permanent state = %+v", second)
	}
	for _, ref := range []string{"steward", "@steward"} {
		got, _, err := Room(ref)
		if err != nil || got.ID != first.ID {
			t.Fatalf("Room(%q) = %+v, %v", ref, got, err)
		}
	}
	if err := Close("steward", "tester"); !errors.Is(err, ErrPermanentRoom) {
		t.Fatalf("Close(steward) = %v, want ErrPermanentRoom", err)
	}
	if _, err := closeMeeting(t.Context(), second, closeOptions{}, nil); !errors.Is(err, ErrPermanentRoom) {
		t.Fatalf("direct closeMeeting = %v, want ErrPermanentRoom", err)
	}
	still, _, err := Room("steward")
	if err != nil || still.Status != "open" {
		t.Fatalf("permanent room did not remain open: %+v, %v", still, err)
	}
}

func TestEnsureConfiguredPermanentRoomsMaterializesAllOnce(t *testing.T) {
	dir := permanentTestStore(t)
	b, _ := json.Marshal(permanentRoomsFile{
		Schema: permanentRoomsSchema,
		Rooms:  []PermanentRoomConfig{{Name: "operations", Topic: "Operations"}},
	})
	if err := os.WriteFile(filepath.Join(dir, "rooms.json"), b, 0o600); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		if rooms, err := EnsureConfiguredPermanentRooms(); err != nil || len(rooms) != 2 {
			t.Fatalf("ensure %d = %+v, %v", i, rooms, err)
		}
	}
	sessions, err := listSessions()
	if err != nil {
		t.Fatal(err)
	}
	if len(sessions) != 2 {
		t.Fatalf("sessions = %d, want one identity per configured room", len(sessions))
	}
}

func TestPermanentRoomConfigFailsClosed(t *testing.T) {
	dir := permanentTestStore(t)
	if err := os.WriteFile(filepath.Join(dir, "rooms.json"), []byte(`{"schema":"wrong"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := EnsureConfiguredPermanentRooms(); err == nil {
		t.Fatal("wrong schema was accepted")
	}
}
