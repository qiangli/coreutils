package meet

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
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

func TestUnreadRecordsAcknowledgesOnlyTheRenderedSnapshot(t *testing.T) {
	st := newRoom(t)
	st.Board = true
	st.Participants = []string{"codex", "opencode"}
	if err := st.save(); err != nil {
		t.Fatal(err)
	}
	if err := AppendEvent(st.ID, Event{Kind: "message", Speaker: "codex", To: "opencode", Text: "first"}); err != nil {
		t.Fatal(err)
	}
	records, _, _, through, err := UnreadRecords(st.ID, "opencode", 0)
	if err != nil || len(records) != 1 || records[0].Seq != 1 {
		t.Fatalf("snapshot=%+v through=%d err=%v", records, through, err)
	}
	if err := AppendEvent(st.ID, Event{Kind: "message", Speaker: "codex", To: "opencode", Text: "second"}); err != nil {
		t.Fatal(err)
	}
	if err := MarkSeenThrough(st.ID, "opencode", through); err != nil {
		t.Fatal(err)
	}
	left, _, _, _, err := UnreadRecords(st.ID, "opencode", 0)
	if err != nil || len(left) != 1 || left[0].Event.Text != "second" {
		t.Fatalf("concurrent arrival was consumed: %+v err=%v", left, err)
	}
}

func TestUnreadRecordsSkipsOwnPostsAndKeepsReaderCursorsIndependent(t *testing.T) {
	st := newRoom(t)
	st.Board = true
	st.Participants = []string{"agent-a", "agent-b"}
	if err := st.save(); err != nil {
		t.Fatal(err)
	}

	if err := AppendEvent(st.ID, Event{Kind: "message", Speaker: "agent-a", Text: "A status"}); err != nil {
		t.Fatal(err)
	}
	directed, other, _, through, err := UnreadRecords(st.ID, "agent-a", 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(directed)+len(other) != 0 {
		t.Fatalf("A received its own outbound post: directed=%+v other=%+v", directed, other)
	}
	if through != 1 {
		t.Fatalf("self-only snapshot through=%d, want 1 so the caller can pass it silently", through)
	}
	if err := MarkSeenThrough(st.ID, "agent-a", through); err != nil {
		t.Fatal(err)
	}
	if got := SeenSeq(st.ID, "agent-a"); got != 1 {
		t.Fatalf("A cursor=%d, want 1", got)
	}
	if got := SeenSeq(st.ID, "agent-b"); got != 0 {
		t.Fatalf("A read advanced B cursor to %d", got)
	}

	if err := AppendEvent(st.ID, Event{Kind: "message", Speaker: "agent-b", Text: "B broadcast"}); err != nil {
		t.Fatal(err)
	}
	if err := AppendEvent(st.ID, Event{Kind: "message", Speaker: "agent-b", To: "agent-a", Text: "B direct"}); err != nil {
		t.Fatal(err)
	}
	directed, other, _, through, err = UnreadRecords(st.ID, "agent-a", 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(directed) != 1 || directed[0].Event.Text != "B direct" {
		t.Fatalf("A directed records=%+v, want B direct", directed)
	}
	if len(other) != 1 || other[0].Event.Text != "B broadcast" {
		t.Fatalf("A broadcast records=%+v, want B broadcast", other)
	}
	if through != 3 {
		t.Fatalf("peer snapshot through=%d, want 3", through)
	}
}

func TestWaitForRoomDoesNotWakeOnReadersOwnPost(t *testing.T) {
	st := newRoom(t)
	st.Board = true
	st.Participants = []string{"agent-a", "agent-b"}
	if err := st.save(); err != nil {
		t.Fatal(err)
	}
	if err := AppendEvent(st.ID, Event{Kind: "message", Speaker: "agent-a", Text: "A status"}); err != nil {
		t.Fatal(err)
	}

	peerPosted := make(chan error, 1)
	go func() {
		time.Sleep(150 * time.Millisecond)
		peerPosted <- AppendEvent(st.ID, Event{Kind: "message", Speaker: "agent-b", To: "agent-a", Text: "B reply"})
	}()
	started := time.Now()
	if err := WaitForRoom(context.Background(), st.ID, "agent-a", 0, time.Second); err != nil {
		t.Fatal(err)
	}
	if err := <-peerPosted; err != nil {
		t.Fatal(err)
	}
	if elapsed := time.Since(started); elapsed < 100*time.Millisecond {
		t.Fatalf("wait woke on A's own post after %s", elapsed)
	}
	directed, other, _, _, err := UnreadRecords(st.ID, "agent-a", 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(directed) != 1 || directed[0].Event.Text != "B reply" || len(other) != 0 {
		t.Fatalf("wait result directed=%+v other=%+v, want only B reply", directed, other)
	}
}
