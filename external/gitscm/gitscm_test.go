package gitscm

import "testing"

func TestAppendGitWindowsNonInteractiveEnv(t *testing.T) {
	t.Parallel()

	got := appendGitWindowsNonInteractiveEnv([]string{"PATH=C:\\bin"})
	want := []string{"PATH=C:\\bin", "GIT_TERMINAL_PROMPT=0", "GCM_INTERACTIVE=never"}
	if len(got) != len(want) {
		t.Fatalf("wrong env length: want %d, got %d: %#v", len(want), len(got), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("env[%d]: want %q, got %q", i, want[i], got[i])
		}
	}
}

func TestAppendGitWindowsNonInteractiveEnvPreservesExplicitValues(t *testing.T) {
	t.Parallel()

	env := []string{"GIT_TERMINAL_PROMPT=1", "GCM_INTERACTIVE=always"}
	got := appendGitWindowsNonInteractiveEnv(env)
	if len(got) != len(env) {
		t.Fatalf("explicit values should not be duplicated: %#v", got)
	}
	for i := range env {
		if got[i] != env[i] {
			t.Fatalf("env[%d]: want %q, got %q", i, env[i], got[i])
		}
	}
}
