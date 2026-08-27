// Package patch is a clean-room, pure-Go implementation of the parsing and
// application rules described by POSIX.1-2016 patch(1p)
// (https://pubs.opengroup.org/onlinepubs/9699919799/utilities/patch.html)
// and, for GNU extensions, the GNU patch manual. No code in this package is
// derived from GNU patch (GPLv3) or any other copyleft source; see
// docs/reference-policy.md for the reference hierarchy and
// docs/patch-continuation-ledger.md for exactly which parts of those
// documents this package implements.
//
// The package is split into two layers on purpose: parsing (this file,
// parse.go) turns raw patch text into an in-memory, format-independent
// description of the edits, and apply.go turns that description plus a
// slice of "before" lines into a slice of "after" lines. Neither layer
// touches a filesystem — cmds/patch owns every read, write, rename, and
// backup, through tool.RunContext, so this package stays trivially unit
// testable and reusable outside the CLI.
package patch

// Format identifies which of the three diff notations a FilePatch was
// written in. All three are documented by POSIX.1-2016 patch(1p) (which
// calls them "context" and describes patch as also accepting the unified
// and "normal"/ed-listing forms diff(1) can produce).
type Format int

const (
	FormatUnified Format = iota
	FormatContext
	FormatNormal
)

func (f Format) String() string {
	switch f {
	case FormatUnified:
		return "unified"
	case FormatContext:
		return "context"
	case FormatNormal:
		return "normal"
	default:
		return "unknown"
	}
}

// LineKind classifies one line inside a Hunk.
type LineKind int

const (
	LineContext LineKind = iota
	LineDelete
	LineAdd
)

// HunkLine is one line of a hunk body, tagged with how it participates in
// the edit and whether the source file it came from lacked a trailing
// newline on that line (the "\ No newline at end of file" marker).
type HunkLine struct {
	Kind  LineKind
	Text  string
	NoEOL bool
}

// Hunk is one contiguous edit, translated from whichever source notation
// into a single canonical shape: a run of context/delete/add lines
// addressed by 0-based old/new starting lines and line counts.
type Hunk struct {
	OldStart, OldCount int
	NewStart, NewCount int
	Lines              []HunkLine
}

// FilePatch is every hunk that targets one logical file, plus the file
// identity taken from the patch headers.
//
// Unsupported is non-empty when this package recognized the file section
// but deliberately does not implement applying it (a binary patch, or a
// git-style pure rename/mode-change with no textual hunk) — see
// docs/patch-continuation-ledger.md. A FilePatch with Unsupported set has
// no Hunks and must be reported to the user, never silently skipped.
type FilePatch struct {
	OldName string
	NewName string
	// IndexName is the pathname from the nearest preceding "Index:" line.
	// Traditional normal diffs have no file headers, so this is often their
	// only non-interactive target candidate.
	IndexName string
	Format    Format
	Hunks     []Hunk

	// RenameFrom/RenameTo are set for a git-style rename header
	// ("rename from"/"rename to") even when Unsupported is also set,
	// so a caller can at least name what could not be applied.
	RenameFrom, RenameTo string

	Unsupported string
}

// Patch is the parsed form of one patch input, in file order.
type Patch struct {
	Files []FilePatch
}

// IsCreate reports whether this file patch creates a new file (the old
// side is /dev/null).
func (fp *FilePatch) IsCreate() bool { return fp.OldName == DevNull && fp.NewName != DevNull }

// IsDelete reports whether this file patch deletes a file (the new side is
// /dev/null).
func (fp *FilePatch) IsDelete() bool { return fp.NewName == DevNull && fp.OldName != DevNull }

// DevNull is the sentinel old/new name unified and context diffs use for a
// side of the edit that does not exist (a pure creation or deletion).
const DevNull = "/dev/null"
