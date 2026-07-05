package skills

// Agent detection: which agentic tool is driving this process, from the
// env markers each agent sets (the CI=true analog of the agent world).
// Detection is free and write-less — the bottom rung of the
// advertisement ladder.

import "os"

// agentMarkers maps environment markers to canonical agent names, per
// the 2026-07 survey: CLAUDECODE (Claude Code), GEMINI_CLI (documented
// for script detection), CODEX_* (Codex CLI sandbox/thread), CURSOR_*
// (Cursor IDE/CLI), GOOSE_TERMINAL, CLINE_ACTIVE. The name-valued
// AGENT / AI_AGENT conventions are handled separately (value = name).
var agentMarkers = []struct{ env, name string }{
	{"CLAUDECODE", "claude"},
	{"CLAUDE_CODE_ENTRYPOINT", "claude"},
	{"CODEX_SANDBOX", "codex"},
	{"CODEX_THREAD_ID", "codex"},
	{"GEMINI_CLI", "gemini"},
	{"CURSOR_AGENT", "cursor"},
	{"CURSOR_TRACE_ID", "cursor"},
	{"GOOSE_TERMINAL", "goose"},
	{"OPENCODE_CLIENT", "opencode"},
	{"CLINE_ACTIVE", "cline"},
}

// DetectAgent reports the agentic tool driving this process, if any.
func DetectAgent() (name string, ok bool) {
	for _, m := range agentMarkers {
		if os.Getenv(m.env) != "" {
			return m.name, true
		}
	}
	// Name-valued conventions (AGENT=goose, AGENT=amp; Vercel's AI_AGENT).
	for _, env := range []string{"AGENT", "AI_AGENT"} {
		if v := os.Getenv(env); v != "" {
			return v, true
		}
	}
	return "", false
}
