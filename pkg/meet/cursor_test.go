package meet

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMeetSeenPathIsRoomLocalAndSafe(t *testing.T) {
	st := newTestSession(t)
	p, err := seenPath(st.ID, "../../etc/passwd")
	if err != nil {
		t.Fatal(err)
	}
	wantDir := filepath.Join(os.Getenv("BASHY_MEET_DIR"), st.ID, "seen")
	if filepath.Dir(p) != wantDir {
		t.Fatalf("seenPath escaped room seen dir: %q want dir %q", p, wantDir)
	}
	base := filepath.Base(p)
	if base == "." || base == ".." || strings.Contains(base, string(filepath.Separator)) {
		t.Fatalf("unsafe cursor basename: %q", base)
	}
}

func TestMeetMarkSeenNeverMovesBackwards(t *testing.T) {
	st := newTestSession(t)
	if err := markSeenSeq(st.ID, "codex", 3); err != nil {
		t.Fatal(err)
	}
	if err := markSeenSeq(st.ID, "codex", 1); err != nil {
		t.Fatal(err)
	}
	if got := SeenSeq(st.ID, "codex"); got != 3 {
		t.Fatalf("SeenSeq moved backwards to %d", got)
	}
}

func TestMeetSeedCursorStartsAtHead(t *testing.T) {
	st := newTestSession(t)
	for _, text := range []string{"old one", "old two"} {
		if err := AppendEvent(st.ID, Event{Speaker: "qiangli", Kind: "human", Text: text}); err != nil {
			t.Fatal(err)
		}
	}
	if err := SeedCursor(st.ID, "opencode"); err != nil {
		t.Fatal(err)
	}
	directed, other, older, err := Unread(st.ID, "opencode", DefaultRoomLimit)
	if err != nil {
		t.Fatal(err)
	}
	if len(directed)+len(other)+older != 0 {
		t.Fatalf("new invitee saw backlog: directed=%d other=%d older=%d", len(directed), len(other), older)
	}
	if err := AppendEvent(st.ID, Event{Speaker: "qiangli", Kind: "human", Text: "new"}); err != nil {
		t.Fatal(err)
	}
	_, other, older, err = Unread(st.ID, "opencode", DefaultRoomLimit)
	if err != nil {
		t.Fatal(err)
	}
	if len(other) != 1 || other[0].Text != "new" || older != 0 {
		t.Fatalf("Unread after seed = other %+v older %d", other, older)
	}
}
