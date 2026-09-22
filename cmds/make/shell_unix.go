//go:build !windows

package makecmd

// defaultRecipeShell is POSIX make's $(SHELL): the command interpreter that
// runs every recipe line.
const defaultRecipeShell = "/bin/sh"

// recipeShellFallback is what make runs when $(SHELL) does not resolve on
// PATH: on a POSIX host the name itself, so execve reports the failure the
// way make always has.
func recipeShellFallback(shell string) string { return shell }
