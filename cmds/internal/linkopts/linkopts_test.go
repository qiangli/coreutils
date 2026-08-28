package linkopts

import (
	"strings"
	"testing"

	"github.com/qiangli/coreutils/cmds/internal/hierwalk"
)

var (
	traversal  = []string{"-H", "-L", "-P"}
	valueFlags = []string{"reference", "from"}
)

func TestScanLastTraversalOptionWins(t *testing.T) {
	cases := []struct {
		args []string
		want hierwalk.Mode
		set  bool
	}{
		{[]string{"-R", "u", "f"}, hierwalk.Physical, false},
		{[]string{"-H", "u", "f"}, hierwalk.CommandLine, true},
		{[]string{"-L", "u", "f"}, hierwalk.Logical, true},
		{[]string{"-P", "u", "f"}, hierwalk.Physical, true},
		{[]string{"-H", "-L", "u", "f"}, hierwalk.Logical, true},
		{[]string{"-L", "-H", "u", "f"}, hierwalk.CommandLine, true},
		{[]string{"-L", "-P", "u", "f"}, hierwalk.Physical, true},
		{[]string{"-P", "-L", "u", "f"}, hierwalk.Logical, true},
		{[]string{"-RLP", "u", "f"}, hierwalk.Physical, true},
		{[]string{"-RPL", "u", "f"}, hierwalk.Logical, true},
		{[]string{"-PL", "-H", "u", "f"}, hierwalk.CommandLine, true},
	}
	for _, tc := range cases {
		_, got := Scan(tc.args, traversal, valueFlags)
		if got.Mode != tc.want || got.ModeSet != tc.set {
			t.Errorf("Scan(%v) = mode %v set %v, want mode %v set %v",
				tc.args, got.Mode, got.ModeSet, tc.want, tc.set)
		}
	}
}

func TestScanStripsOnlyTraversalOptions(t *testing.T) {
	cases := []struct {
		args []string
		want string
	}{
		{[]string{"-RLv", "u", "f"}, "-Rv u f"},
		{[]string{"-L", "u", "f"}, "u f"},
		{[]string{"-hL", "u", "f"}, "-h u f"},
		{[]string{"--reference=r", "-P", "f"}, "--reference=r f"},
		{[]string{"-R", "--", "-L"}, "-R -- -L"},
		{[]string{"-", "f"}, "- f"},
	}
	for _, tc := range cases {
		rest, _ := Scan(tc.args, traversal, valueFlags)
		if got := strings.Join(rest, " "); got != tc.want {
			t.Errorf("Scan(%v) rest = %q, want %q", tc.args, got, tc.want)
		}
	}
}

func TestScanLeavesOptionValuesAlone(t *testing.T) {
	// A separate-argument option value that happens to look like a
	// traversal cluster must survive: it is data, not an option.
	rest, res := Scan([]string{"--from", "-P", "-L", "u", "f"}, traversal, valueFlags)
	if got := strings.Join(rest, " "); got != "--from -P u f" {
		t.Errorf("rest = %q, want %q", got, "--from -P u f")
	}
	if res.Mode != hierwalk.Logical {
		t.Errorf("mode = %v, want Logical", res.Mode)
	}
}

func TestScanLastDereferenceOptionWins(t *testing.T) {
	cases := []struct {
		args []string
		want Deref
	}{
		{[]string{"u", "f"}, DerefUnset},
		{[]string{"-h", "u", "f"}, DerefLink},
		{[]string{"--no-dereference", "u", "f"}, DerefLink},
		{[]string{"--dereference", "u", "f"}, DerefReferent},
		{[]string{"-h", "--dereference", "u", "f"}, DerefReferent},
		{[]string{"--dereference", "-h", "u", "f"}, DerefLink},
		{[]string{"--no-dereference", "--dereference", "u", "f"}, DerefReferent},
		{[]string{"--deref", "u", "f"}, DerefReferent},
		{[]string{"--no-deref", "u", "f"}, DerefLink},
		{[]string{"--no-preserve-root", "u", "f"}, DerefUnset},
		{[]string{"-Rh", "u", "f"}, DerefLink},
	}
	for _, tc := range cases {
		_, got := Scan(tc.args, traversal, valueFlags)
		if got.Deref != tc.want {
			t.Errorf("Scan(%v) deref = %v, want %v", tc.args, got.Deref, tc.want)
		}
	}
}

// Everything after "--" is an operand. A file named "-L" must not
// silently become a traversal mode, in either the returned arguments or
// the resolved group.
func TestScanStopsAtEndOfOptions(t *testing.T) {
	rest, res := Scan([]string{"-P", "--", "-L", "-h"}, traversal, valueFlags)
	if got := strings.Join(rest, " "); got != "-- -L -h" {
		t.Errorf("rest = %q, want %q", got, "-- -L -h")
	}
	if res.Mode != hierwalk.Physical || !res.ModeSet {
		t.Errorf("mode = %v set %v, want Physical set", res.Mode, res.ModeSet)
	}
	if res.Deref != DerefUnset {
		t.Errorf("deref = %v, want unset: -h after -- is an operand", res.Deref)
	}
}

func TestScanStopsAtFirstOperand(t *testing.T) {
	rest, res := Scan([]string{"-P", "owner", "-L", "--dereference"}, traversal, valueFlags)
	if got := strings.Join(rest, " "); got != "owner -L --dereference" {
		t.Fatalf("rest = %q", got)
	}
	if res.Mode != hierwalk.Physical || !res.ModeSet || res.Deref != DerefUnset {
		t.Fatalf("post-operand options changed scan result: %+v", res)
	}
}

// An option value attached with "=" is data even when it is spelled
// like a traversal option.
func TestScanAttachedOptionValueIsNotAnOption(t *testing.T) {
	rest, res := Scan([]string{"--reference=-L", "f"}, traversal, valueFlags)
	if got := strings.Join(rest, " "); got != "--reference=-L f" {
		t.Errorf("rest = %q", got)
	}
	if res.ModeSet {
		t.Errorf("mode was set from an option value: %v", res.Mode)
	}
}

// The two groups are independent: the last traversal option and the
// last dereference option each win on their own, however they
// interleave.
func TestScanResolvesBothGroupsIndependently(t *testing.T) {
	cases := []struct {
		args     []string
		wantMode hierwalk.Mode
		wantDrf  Deref
	}{
		{[]string{"-L", "-h", "-H", "--dereference"}, hierwalk.CommandLine, DerefReferent},
		{[]string{"-H", "--dereference", "-L", "-h"}, hierwalk.Logical, DerefLink},
		{[]string{"-PLh"}, hierwalk.Logical, DerefLink},
		{[]string{"-hLP"}, hierwalk.Physical, DerefLink},
		{[]string{"-h", "-RLP", "--dereference", "-H"}, hierwalk.CommandLine, DerefReferent},
	}
	for _, tc := range cases {
		_, got := Scan(tc.args, traversal, valueFlags)
		if got.Mode != tc.wantMode || got.Deref != tc.wantDrf {
			t.Errorf("Scan(%v) = mode %v deref %v, want mode %v deref %v",
				tc.args, got.Mode, got.Deref, tc.wantMode, tc.wantDrf)
		}
	}
}

// A traversal option the command does not name is not this package's to
// consume: it stays in the arguments so the framework parser reports it
// with the command's own diagnostic.
func TestScanLeavesUnnamedTraversalOptionsForTheParser(t *testing.T) {
	rest, res := Scan([]string{"-L", "-P", "f"}, []string{"-P"}, valueFlags)
	if got := strings.Join(rest, " "); got != "-L f" {
		t.Errorf("rest = %q, want %q", got, "-L f")
	}
	if res.Mode != hierwalk.Physical || !res.ModeSet {
		t.Errorf("mode = %v set %v, want the one option the command names", res.Mode, res.ModeSet)
	}
}
