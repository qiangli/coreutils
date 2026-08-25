//go:build unix

package atcmd

import (
	"context"
	"os"
	"os/user"
	"path/filepath"
	"strings"
	"testing"
)

func setATAccessDirs(t *testing.T, dirs ...string) {
	t.Helper()
	old := atAccessDirs
	atAccessDirs = dirs
	t.Cleanup(func() { atAccessDirs = old })
}

func currentUsername(t *testing.T) string {
	t.Helper()
	current, err := user.Current()
	if err != nil {
		t.Fatal(err)
	}
	return current.Username
}

func TestIssue743AtAccessMalformedPolicyFailsClosed(t *testing.T) {
	setupATState(t)
	name := currentUsername(t)

	// at.allow listing the user but containing a malformed (comment) line:
	// the policy format is one user name per line, so the ambiguity denies.
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "at.allow"), []byte(name+"\n# comment\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	setATAccessDirs(t, dir)
	if _, stderr, code := runATNoStdin(t, context.Background(), "-l"); code != 1 || !strings.Contains(stderr, "not authorized") {
		t.Fatalf("malformed allow: code=%d stderr=%q", code, stderr)
	}

	// at.deny that does not list the user but has a whitespace-bearing line:
	// a malformed deny list must not silently widen to "permitted".
	dir = t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "at.deny"), []byte("someone else\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	setATAccessDirs(t, dir)
	if _, stderr, code := runATNoStdin(t, context.Background(), "-l"); code != 1 || !strings.Contains(stderr, "not authorized") {
		t.Fatalf("malformed deny: code=%d stderr=%q", code, stderr)
	}
}

func TestIssue743AtAccessStatErrorFailsClosed(t *testing.T) {
	setupATState(t)
	name := currentUsername(t)
	blocker := filepath.Join(t.TempDir(), "not-a-directory")
	if err := os.WriteFile(blocker, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	good := t.TempDir()
	if err := os.WriteFile(filepath.Join(good, "at.allow"), []byte(name+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	// A stat failure that is not "does not exist" must deny, not fall through
	// to a later directory whose allow file would authorize the user.
	setATAccessDirs(t, filepath.Join(blocker, "sub"), good)
	if _, stderr, code := runATNoStdin(t, context.Background(), "-l"); code != 1 || !strings.Contains(stderr, "not authorized") {
		t.Fatalf("stat error: code=%d stderr=%q", code, stderr)
	}
}

func TestIssue743AtAccessEmptyDenyPermits(t *testing.T) {
	setupATState(t)
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "at.deny"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	setATAccessDirs(t, dir)
	if _, stderr, code := runATNoStdin(t, context.Background(), "-l"); code != 0 || stderr != "" {
		t.Fatalf("empty deny must permit everyone: code=%d stderr=%q", code, stderr)
	}
}
