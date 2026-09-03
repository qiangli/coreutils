// The ONE accountability flag every durable object shares, kept here so meet,
// sprint and todo cannot drift apart on how an owner is named.
package role

import (
	"fmt"

	"github.com/spf13/pflag"
)

// Title is what an owner is CALLED in the domain that owns it. The flag is one
// word everywhere so an agent learns it once; the title is the domain's own
// word so the help still reads like the thing it describes.
type Title string

const (
	// Facilitator runs a meeting's floor and answers for the room.
	Facilitator Title = "facilitator"
	// ProjectManager is accountable for a sprint's delivery start to end.
	ProjectManager Title = "project manager"
	// Assignee is who is working an item.
	Assignee Title = "assignee"
)

// AttachOwner registers --owner as the primary spelling and `deprecated` as an
// accepted alias for it.
//
// ONE FLAG, DOMAIN TITLES. `--owner` is the single spelling across meet, sprint
// and todo; the help text says facilitator, project manager or assignee, so the
// word a human reads is still the word their domain uses. `--as` keeps its own
// separate meaning everywhere — who I am acting AS right now, for authorship
// and read cursors — and the two are never merged.
//
// The alias is kept rather than removed because every existing script, skill
// and doc spells it the old way, and a flag that vanishes turns a rename into
// an outage. Passing BOTH is an error: silently preferring one would make two
// spellings mean two different things in the same command, which is the exact
// confusion this consolidates away.
func AttachOwner(f *pflag.FlagSet, target *string, title Title, what string) {
	f.StringVar(target, "owner", "", fmt.Sprintf("%s — %s", title, what))
}

// AttachOwnerAlias adds the legacy spelling for the same target and hides it,
// so `--help` teaches one name while old invocations keep working.
// The alias writes the SAME target, so an old invocation keeps working with no
// change at the call site. Only one of the two is ever passed in practice;
// OwnerConflict is what refuses the case where both are.
func AttachOwnerAlias(f *pflag.FlagSet, target *string, deprecated string, title Title) {
	f.StringVar(target, deprecated, "", fmt.Sprintf("deprecated alias for --owner (the %s)", title))
	_ = f.MarkHidden(deprecated)
}

// OwnerConflict refuses a command that passed both spellings. Attach it as a
// PreRunE: if the two disagree the caller believes something about who is
// accountable that is not what would happen, and accountability is the one
// field where guessing is worse than stopping.
func OwnerConflict(f *pflag.FlagSet, deprecated string, title Title) error {
	if f.Changed("owner") && f.Changed(deprecated) {
		return fmt.Errorf("--owner and --%s are the same field (the %s); pass one", deprecated, title)
	}
	return nil
}
