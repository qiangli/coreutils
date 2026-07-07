package session

import (
	"os"
	"path/filepath"
	"testing"
)

func TestReadTextRecords(t *testing.T) {
	p := filepath.Join(t.TempDir(), "utmp.txt")
	if err := os.WriteFile(p, []byte("alice tty1 100 host\nbob pts/2 200 remote user\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	records, err := Read(p)
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 2 || records[0].User != "alice" || records[1].Host != "remote" {
		t.Fatalf("records=%#v", records)
	}
	users, err := Users(p)
	if err != nil {
		t.Fatal(err)
	}
	if len(users) != 2 || users[0] != "alice" || users[1] != "bob" {
		t.Fatalf("users=%#v", users)
	}
}
