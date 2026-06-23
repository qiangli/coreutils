package weave

import (
	"testing"
)

func TestSessionPointerRoundTrip(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	repoRoot := t.TempDir()

	want := &SessionPointer{
		TaskID:       "task-1",
		CloudboxBase: "https://cloudbox.example",
		TokenRef:     "keychain:cloudbox",
	}
	if err := WriteSessionPointer(repoRoot, want); err != nil {
		t.Fatal(err)
	}
	got, err := ReadSessionPointer(repoRoot)
	if err != nil {
		t.Fatal(err)
	}
	if got == nil || *got != *want {
		t.Fatalf("pointer = %+v, want %+v", got, want)
	}
}

func TestReadSessionPointerMissing(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	repoRoot := t.TempDir()

	got, err := ReadSessionPointer(repoRoot)
	if err != nil {
		t.Fatal(err)
	}
	if got != nil {
		t.Fatalf("pointer = %+v, want nil", got)
	}
}
