package role

import (
	"strings"
	"testing"

	"github.com/spf13/pflag"
)

func ownerSet(t *testing.T, title Title, alias, what string) (*pflag.FlagSet, *string) {
	t.Helper()
	f := pflag.NewFlagSet("t", pflag.ContinueOnError)
	var owner string
	AttachOwner(f, &owner, title, what)
	AttachOwnerAlias(f, &owner, alias, title)
	return f, &owner
}

// ONE FLAG. --owner is the spelling every domain shares.
func TestOwnerFlagIsTheSameFieldAsItsAlias(t *testing.T) {
	for _, tc := range []struct{ args []string }{
		{[]string{"--owner", "tenon"}},
		{[]string{"--as", "tenon"}},
	} {
		f, got := ownerSet(t, ProjectManager, "as", "accountable for delivery")
		if err := f.Parse(tc.args); err != nil {
			t.Fatalf("%v: %v", tc.args, err)
		}
		if *got != "tenon" {
			t.Fatalf("%v set owner=%q, want tenon — an alias that does not reach the "+
				"same field turns a rename into an outage", tc.args, *got)
		}
	}
}

// DOMAIN TITLES. The help says what the owner IS here, so one spelling does not
// cost the reader their domain's own word.
func TestOwnerHelpCarriesItsOwnTitleAndNotAnother(t *testing.T) {
	for _, tc := range []struct {
		title Title
		alias string
		other []string
	}{
		{Facilitator, "chair", []string{"project manager", "assignee"}},
		{ProjectManager, "as", []string{"facilitator", "assignee"}},
		{Assignee, "assignee", []string{"facilitator", "project manager"}},
	} {
		f, _ := ownerSet(t, tc.title, tc.alias, "does the thing")
		help := f.FlagUsages()
		if !strings.Contains(help, string(tc.title)) {
			t.Errorf("help for %s omits its own title:\n%s", tc.title, help)
		}
		for _, wrong := range tc.other {
			if strings.Contains(help, wrong) {
				t.Errorf("help for %s leaks another domain's title %q:\n%s", tc.title, wrong, help)
			}
		}
	}
}

// The alias is hidden so --help teaches ONE name, while old invocations keep
// working. A deprecated flag that still advertises itself teaches nothing.
func TestTheAliasIsHiddenButStillAccepted(t *testing.T) {
	f, got := ownerSet(t, Facilitator, "chair", "runs the floor")
	if strings.Contains(f.FlagUsages(), "--chair") {
		t.Error("the deprecated alias is still advertised in --help")
	}
	if err := f.Parse([]string{"--chair", "lintel"}); err != nil || *got != "lintel" {
		t.Fatalf("the hidden alias stopped working: err=%v owner=%q", err, *got)
	}
}

// BOTH SPELLINGS IS AN ERROR, not a preference. If they disagree the caller
// believes something about who is accountable that is not what would happen,
// and accountability is the one field where guessing is worse than stopping.
func TestPassingBothSpellingsIsRefused(t *testing.T) {
	f, _ := ownerSet(t, ProjectManager, "as", "accountable for delivery")
	if err := f.Parse([]string{"--owner", "a", "--as", "b"}); err != nil {
		t.Fatal(err)
	}
	err := OwnerConflict(f, "as", ProjectManager)
	if err == nil {
		t.Fatal("passing --owner and --as together was accepted; they are one field")
	}
	if !strings.Contains(err.Error(), "project manager") {
		t.Errorf("the refusal does not name the domain title: %v", err)
	}
}

func TestOwnerConflictIsSilentWhenOnlyOneIsGiven(t *testing.T) {
	for _, args := range [][]string{{"--owner", "a"}, {"--as", "a"}, {}} {
		f, _ := ownerSet(t, ProjectManager, "as", "accountable")
		if err := f.Parse(args); err != nil {
			t.Fatal(err)
		}
		if err := OwnerConflict(f, "as", ProjectManager); err != nil {
			t.Errorf("%v was refused: %v", args, err)
		}
	}
}
