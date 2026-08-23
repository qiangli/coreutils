//go:build unix

package newgrpcmd

import (
	"path/filepath"
	"testing"
)

func TestPasswdShell(t *testing.T) {
	dir := t.TempDir()
	path := write(t, dir, "passwd", "root:x:0:0:root:/root:/bin/sh\nalice:x:1000:1000:Alice:/home/alice:/bin/zsh\ntruncated:x:1:1:no shell field\n", 0o644)

	old := passwdFile
	passwdFile = path
	t.Cleanup(func() { passwdFile = old })

	if got := passwdShell("alice"); got != "/bin/zsh" {
		t.Errorf("passwdShell(alice) = %q, want /bin/zsh", got)
	}
	// A short line has no shell field; skipping it is right, and returning ""
	// lets the caller fall through to its default rather than inventing one.
	if got := passwdShell("truncated"); got != "" {
		t.Errorf("passwdShell(truncated) = %q, want the empty fallback", got)
	}
	if got := passwdShell("nobody-here"); got != "" {
		t.Errorf("passwdShell(missing) = %q, want the empty fallback", got)
	}

	// A directory-service account has no line at all, and an unreadable file
	// is the same situation from newgrp's point of view: no answer, not an
	// error, because $SHELL and /bin/sh still stand behind it.
	passwdFile = filepath.Join(dir, "does-not-exist")
	if got := passwdShell("alice"); got != "" {
		t.Errorf("passwdShell with no file = %q, want the empty fallback", got)
	}
}
