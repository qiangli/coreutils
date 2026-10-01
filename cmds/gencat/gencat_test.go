package gencatcmd

import (
	"strings"
	"testing"
)

func TestCatalogMergeAndDeletion(t *testing.T) {
	cat := make(catalog)
	if err := mergeSource(cat, strings.NewReader("$set 2\n1 hello\\nworld\n2 keep\n")); err != nil {
		t.Fatal(err)
	}
	encoded, err := encodeCatalog(cat)
	if err != nil {
		t.Fatal(err)
	}
	loaded, err := decodeCatalog(encoded)
	if err != nil {
		t.Fatal(err)
	}
	if loaded[messageKey{2, 1}] != "hello\nworld" || loaded[messageKey{2, 2}] != "keep" {
		t.Fatalf("catalog round trip lost messages: %#v", loaded)
	}
	if err := mergeSource(loaded, strings.NewReader("$set 2\n1 replacement\n2\n")); err != nil {
		t.Fatal(err)
	}
	if loaded[messageKey{2, 1}] != "replacement" || len(loaded) != 1 {
		t.Fatalf("catalog merge/deletion failed: %#v", loaded)
	}
}
