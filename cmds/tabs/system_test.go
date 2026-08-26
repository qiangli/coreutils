package tabscmd

import (
	"bytes"
	"os"
	"os/exec"
	"testing"

	"github.com/qiangli/coreutils/tool"
)

// The preset column lists and the space-walking render are transcriptions —
// of the standard and of a wire format — so when a reference tabs and a real
// terminfo database are both present, compare the actual bytes.
//
// This is a CROSS-CHECK, not a dependency: it skips cleanly wherever the
// binary or the database is missing (Windows, a scratch container), and every
// other test in the package drives fixtures built in-process.
//
// Only the forms every implementation agrees on are compared. The reference
// implementations differ from POSIX in places this one follows the standard
// instead — BSD tabs rejects the <blank> separator POSIX allows, and treats an
// unsatisfiable margin as a warning rather than an error — so those forms are
// covered by the fixture tests above, where the expected bytes can be stated
// outright.
func TestAgreesWithSystemTabs(t *testing.T) {
	if os.Getenv("COREUTILS_SYSTEM_DIFFERENTIALS") != "1" {
		t.Skip("set COREUTILS_SYSTEM_DIFFERENTIALS=1 to compare with host tabs")
	}
	sys, err := exec.LookPath("tabs")
	if err != nil {
		t.Skip("no system tabs to compare against")
	}

	args := [][]string{
		{"-8"}, {"-4"}, {"-1"}, {"-0"},
		{"-a"}, {"-a2"}, {"-c"}, {"-c2"}, {"-c3"}, {"-f"}, {"-p"}, {"-s"}, {"-u"},
		{"1,5,9"}, {"1,+4,+4"}, {"7,14,21"}, {"1,10,16,36,72"}, {"12"},
	}

	compared := 0
	for _, term := range []string{"xterm", "xterm-256color", "vt100", "ansi", "linux"} {
		if !systemKnows(t, sys, term) {
			continue
		}
		for _, a := range args {
			argv := append([]string{"-T", term}, a...)

			cmd := exec.Command(sys, argv...)
			// Pin the width: the reference reads the environment the same way
			// this implementation does, and an unpinned width makes the
			// repetitive forms depend on the developer's window.
			cmd.Env = append(os.Environ(), "COLUMNS=80")
			var theirs bytes.Buffer
			cmd.Stdout = &theirs
			cmd.Stderr = nil
			if err := cmd.Run(); err != nil {
				continue // an older reference may not know this form
			}

			var ours, errb bytes.Buffer
			rc := &tool.RunContext{
				Dir:   t.TempDir(),
				Env:   []string{"COLUMNS=80"},
				Stdio: tool.Stdio{Out: &ours, Err: &errb},
			}
			if code := run(rc, argv); code != exitOK {
				t.Errorf("%s %v: exit %d, stderr %q", term, a, code, errb.String())
				continue
			}
			compared++
			if ours.String() != theirs.String() {
				t.Errorf("%s %v:\n ours   %q\n theirs %q", term, a, ours.String(), theirs.String())
			}
		}
	}
	if compared == 0 {
		t.Skip("no terminal type resolved from a system database")
	}
	t.Logf("compared %d tab-stop sequences against %s", compared, sys)
}

// systemKnows reports whether the reference implementation resolves term from
// a real database. Comparing against a reference that itself fell back would
// prove nothing.
func systemKnows(t *testing.T, sys, term string) bool {
	t.Helper()
	cmd := exec.Command(sys, "-T", term, "-0")
	cmd.Env = append(os.Environ(), "COLUMNS=80")
	out, err := cmd.Output()
	return err == nil && len(out) > 0
}
