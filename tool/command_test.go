package tool

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// writeExe creates a regular file at path with an executable bit (or, on
// Windows, a .bat that CreateProcess will run).
func writeExe(t *testing.T, path, content string) {
	t.Helper()
	if runtime.GOOS == "windows" && filepath.Ext(path) == "" {
		path += ".bat"
	}
	if err := os.WriteFile(path, []byte(content), 0o755); err != nil {
		t.Fatal(err)
	}
}

// TestResolveCommandPathUnsetDefaultPath: when PATH is absent from the
// environment, execvp(3) falls back to a default search path containing the
// standard binary directories, so `echo` is still found. (POSIX; GNU providers
// pass this.)
func TestResolveCommandPathUnsetDefaultPath(t *testing.T) {
	rc := &RunContext{Dir: t.TempDir(), Env: []string{"HOME=/tmp"}}
	if got := rc.ResolveCommand("echo"); got == "" {
		t.Fatal("ResolveCommand(echo) with PATH unset returned \"\", want a resolved path (default search path)")
	}
}

// TestResolveCommandExplicitEmptyPathIsCwdOnly: an explicitly empty PATH
// ("PATH=") is a single zero-length element — the working directory — NOT
// "nowhere to look". A command present in the working directory is found.
func TestResolveCommandExplicitEmptyPathIsCwdOnly(t *testing.T) {
	dir := t.TempDir()
	writeExe(t, filepath.Join(dir, "mytool"), "#!/bin/sh\n")
	rc := &RunContext{Dir: dir, Env: []string{"PATH="}}
	if got := rc.ResolveCommand("mytool"); got == "" {
		t.Fatal("ResolveCommand with explicit empty PATH did not find the cwd command")
	}
}

// TestResolveCommandEmptyComponentMeansCwd: a zero-length PATH component
// (leading, trailing, or interior) names the current directory, per execvp(3).
func TestResolveCommandEmptyComponentMeansCwd(t *testing.T) {
	dir := t.TempDir()
	writeExe(t, filepath.Join(dir, "mytool"), "#!/bin/sh\n")
	for _, tc := range []struct{ name, path string }{
		{"leading empty", string(os.PathListSeparator) + "/nonexistent"},
		{"trailing empty", "/nonexistent" + string(os.PathListSeparator)},
		{"interior empty", "/nonexistent" + string(os.PathListSeparator) + string(os.PathListSeparator) + "/nonexistent"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rc := &RunContext{Dir: dir, Env: []string{"PATH=" + tc.path}}
			if got := rc.ResolveCommand("mytool"); got == "" {
				t.Errorf("ResolveCommand with %q did not find the cwd command", tc.path)
			}
		})
	}
}

// TestResolveCommandRelativePathEntryIsWorkdirRelative: a relative PATH entry
// (e.g. "bin") is resolved against rc.Dir, NOT the host process's cwd. This is
// the core bug that produced false "command not found" results.
func TestResolveCommandRelativePathEntryIsWorkdirRelative(t *testing.T) {
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "bin"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeExe(t, filepath.Join(dir, "bin", "mytool"), "#!/bin/sh\n")
	rc := &RunContext{Dir: dir, Env: []string{"PATH=bin"}}
	want := filepath.Join(dir, "bin", "mytool")
	if runtime.GOOS == "windows" {
		want += ".bat"
	}
	if got := rc.ResolveCommand("mytool"); got != want {
		t.Errorf("ResolveCommand with relative PATH=bin = %q, want %q", got, want)
	}
}

// TestResolveCommandSeparatorNameNotSearched: a name containing a separator is
// taken relative to the working directory and never searched in PATH.
func TestResolveCommandSeparatorNameNotSearched(t *testing.T) {
	dir := t.TempDir()
	writeExe(t, filepath.Join(dir, "local"), "#!/bin/sh\n")
	rc := &RunContext{Dir: dir, Env: []string{"PATH=/nonexistent"}}
	if got := rc.ResolveCommand("./local"); got == "" {
		t.Error("ResolveCommand(./local) returned empty for an existing cwd-relative program")
	}
}

// TestResolveCommandFirstExecutableWins: when a name matches a non-executable
// file in an earlier PATH component and an executable one later, the executable
// is returned (execvp continues past non-executable matches).
func TestResolveCommandFirstExecutableWins(t *testing.T) {
	dir := t.TempDir()
	neDir, exDir := filepath.Join(dir, "ne"), filepath.Join(dir, "ex")
	if err := os.MkdirAll(neDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(exDir, 0o755); err != nil {
		t.Fatal(err)
	}
	// Earlier component: a non-executable "tool".
	if err := os.WriteFile(filepath.Join(neDir, "tool"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	// Later component: an executable "tool".
	writeExe(t, filepath.Join(exDir, "tool"), "#!/bin/sh\n")
	path := neDir + string(os.PathListSeparator) + exDir
	rc := &RunContext{Dir: dir, Env: []string{"PATH=" + path}}
	got := rc.ResolveCommand("tool")
	want := filepath.Join(exDir, "tool")
	if runtime.GOOS == "windows" {
		want += ".bat"
	}
	if got != want {
		t.Errorf("ResolveCommand = %q, want the executable %q", got, want)
	}
}

// TestResolveCommandNotFoundReturnsEmpty: nothing in PATH and no separator in
// the name yields "" (caller reports POSIX 127).
func TestResolveCommandNotFoundReturnsEmpty(t *testing.T) {
	rc := &RunContext{Dir: t.TempDir(), Env: []string{"PATH=/nonexistent"}}
	if got := rc.ResolveCommand("no-such-command-xyz-123"); got != "" {
		t.Errorf("ResolveCommand = %q, want \"\" for a command not in PATH", got)
	}
}
