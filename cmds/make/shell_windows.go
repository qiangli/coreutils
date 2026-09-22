//go:build windows

package makecmd

import (
	"os"
	"path/filepath"
	"strings"
)

// defaultRecipeShell is POSIX make's $(SHELL). Windows has no /bin/sh, and
// a Makefile's recipes are POSIX shell text, so the interpreter is the shell
// this binary belongs to — bashy — which is exactly what an embedded
// invocation (`bashy make`) already runs inside.
const defaultRecipeShell = "/bin/sh"

// recipeShellFallback resolves an unresolvable $(SHELL) on Windows to this
// process's own executable when the name is a POSIX shell spelling. Without
// it, `make` on a Windows host fails every recipe with
// "exec: /bin/sh: file does not exist" — the name is a Unix convention, not
// a file that exists there. A $(SHELL) naming anything else is left alone so
// the failure still names what the Makefile asked for.
func recipeShellFallback(shell string) string {
	base := strings.TrimSuffix(strings.ToLower(filepath.Base(shell)), ".exe")
	switch base {
	case "sh", "bash", "bashy":
		if self, err := os.Executable(); err == nil {
			return self
		}
	}
	return shell
}
