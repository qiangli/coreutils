package resources

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestResourceAlertUnchangedTransactionSkipsPersistence(t *testing.T) {
	dir := t.TempDir()
	ctx := context.Background()
	first, err := UpdateAlertState(ctx, dir, func(l *AlertLedger) error { l.Entries["one"] = json.RawMessage(`{"value":1}`); return nil })
	if err != nil {
		t.Fatal(err)
	}
	before, err := os.Stat(filepath.Join(dir, "alerts.json"))
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 8; i++ {
		next, err := UpdateAlertState(ctx, dir, func(l *AlertLedger) error { l.Entries["one"] = json.RawMessage(`{"value":1}`); return nil })
		if err != nil {
			t.Fatal(err)
		}
		if next.Revision != first.Revision || !next.UpdatedAt.Equal(first.UpdatedAt) {
			t.Fatal("identical client refresh changed ledger revision")
		}
	}
	after, _ := os.Stat(filepath.Join(dir, "alerts.json"))
	if !os.SameFile(before, after) || !before.ModTime().Equal(after.ModTime()) {
		t.Fatal("identical transaction replaced durable ledger")
	}
}
