//go:build windows

package makecmd

import (
	"os"
	"testing"
)

// Windows has no /bin/sh: a Makefile's recipes run through the shell this
// binary belongs to rather than failing with "file does not exist".
func TestRecipeShellFallbackIsThisShell(t *testing.T) {
	self, err := os.Executable()
	if err != nil {
		t.Skip(err)
	}
	for _, name := range []string{"/bin/sh", "sh", "bash", `C:\tools\bash.exe`} {
		if got := recipeShellFallback(name); got != self {
			t.Errorf("recipeShellFallback(%q) = %q, want %q", name, got, self)
		}
	}
	for _, name := range []string{"cmd.exe", "powershell", "/usr/bin/perl"} {
		if got := recipeShellFallback(name); got != name {
			t.Errorf("recipeShellFallback(%q) = %q, want it unchanged", name, got)
		}
	}
}
