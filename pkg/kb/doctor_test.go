// Copyright (c) 2025 qiangli
// See LICENSE for licensing information

package kb

import "testing"

// TestNoteFormNeedsNoDescription: a note is the memo shape — `kb add --form
// note --title --body` is one command, and doctor does not call the missing
// description a hygiene problem. A page still needs its routing surface.
func TestNoteFormNeedsNoDescription(t *testing.T) {
	dir := t.TempDir()
	st := Open(dir)
	note := &Page{Form: FormNote, Type: TypeLesson, Title: "Memo", Slug: "memo", Body: "just a memo", Status: StatusCandidate}
	if err := st.Write(note, "add"); err != nil {
		t.Fatalf("write note: %v", err)
	}
	page := &Page{Form: FormPage, Type: TypeLesson, Title: "Page", Slug: "page", Body: "x", Status: StatusCandidate}
	if err := st.Write(page, "add"); err != nil {
		t.Fatalf("write page: %v", err)
	}
	pages, err := st.List()
	if err != nil {
		t.Fatal(err)
	}
	rep := Doctor(pages, st, nil, false)
	for _, slug := range rep.MissingDescription {
		if slug == "memo" {
			t.Errorf("doctor flagged the note for a missing description")
		}
	}
	found := false
	for _, slug := range rep.MissingDescription {
		if slug == "page" {
			found = true
		}
	}
	if !found {
		t.Errorf("doctor must still flag a page with no description")
	}
	// The CLI path: buildPage refuses a page without --description, accepts a note.
	f := &pageFlags{form: FormPage, typ: TypeLesson, title: "P", body: "x"}
	if _, err := f.buildPage(nil); err == nil {
		t.Errorf("page without description must be refused")
	}
	f = &pageFlags{form: FormNote, typ: TypeLesson, title: "N", body: "x"}
	if _, err := f.buildPage(nil); err != nil {
		t.Errorf("note without description must be accepted: %v", err)
	}
}
