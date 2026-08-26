package who

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestRegisterReadPruneAndClose(t *testing.T) {
	path := filepath.Join(t.TempDir(), "who", "sessions")
	h, err := RegisterFile(path, Record{
		Name: "agent-w14", PID: os.Getpid(), Started: time.Unix(1700000000, 0),
		Surfaces: []string{"meet", "mb", "mb"},
	})
	if err != nil {
		t.Fatal(err)
	}
	deadPID := 1 << 30
	if deadPID == os.Getpid() {
		deadPID++
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = f.WriteString("stale pty/stale 1700000001 mb user id=stale pid=" + strconv.Itoa(deadPID) + "\n")
	_ = f.Close()

	b, err := ReadLive(path)
	if err != nil {
		t.Fatal(err)
	}
	text := string(b)
	if !strings.Contains(text, "agent-w14 pty/agent-w14 1700000000 mb,meet user") {
		t.Fatalf("live record missing or unstable: %q", text)
	}
	if strings.Contains(text, "stale") {
		t.Fatalf("dead PID was not pruned: %q", text)
	}

	plan := filepath.Join(filepath.Dir(path), "agent-w14", ".plan")
	page, err := os.ReadFile(plan)
	if err != nil || !strings.Contains(string(page), "status: ONLINE") {
		t.Fatalf("generated live page: data=%q err=%v", page, err)
	}
	if err := h.Close(); err != nil {
		t.Fatal(err)
	}
	if err := h.Close(); err != nil {
		t.Fatalf("second Close must be inert: %v", err)
	}
	b, err = os.ReadFile(path)
	if err != nil || len(b) != 0 {
		t.Fatalf("record survived Close: data=%q err=%v", b, err)
	}
	page, err = os.ReadFile(plan)
	if err != nil || !strings.Contains(string(page), "status: OFFLINE") {
		t.Fatalf("durable page was not marked offline: data=%q err=%v", page, err)
	}
}

func TestSelfPublishedPlanIsNeverRewritten(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sessions")
	dir := filepath.Join(filepath.Dir(path), "alice")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	const own = "written by alice\ncontact: elsewhere\n"
	if err := os.WriteFile(filepath.Join(dir, ".plan"), []byte(own), 0o600); err != nil {
		t.Fatal(err)
	}
	h, err := RegisterFile(path, Record{Name: "alice", PID: os.Getpid(), Surfaces: []string{"mb"}})
	if err != nil {
		t.Fatal(err)
	}
	if err := h.Close(); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(filepath.Join(dir, ".plan"))
	if err != nil || string(got) != own {
		t.Fatalf("self-published plan changed: got=%q err=%v", got, err)
	}
}

func TestRegisterRejectsUnsafeNamesAndUnreachableRows(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sessions")
	for _, name := range []string{"../escape", "a/b", "a\\b", "two words"} {
		if _, err := RegisterFile(path, Record{Name: name, PID: os.Getpid(), Surfaces: []string{"mb"}}); err == nil {
			t.Errorf("unsafe name %q accepted", name)
		}
	}
	if _, err := RegisterFile(path, Record{Name: "silent", PID: os.Getpid()}); err == nil {
		t.Fatal("a session with no live surface was registered")
	}
}
