package weave

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// ToolProfile is the persistent profile + track record for one agentic CLI,
// stored at <weaveStateRoot>/tools/<tool>.json (system-wide, cross-queue). It is
// the conductor's source of truth for HOW to launch a tool unattended and how
// well it has performed per role. Bootstrapped by `weave fleet interview`;
// consumed by the conductor when assembling a `weave start` invocation.
type ToolProfile struct {
	Tool    string `json:"tool"`
	Bin     string `json:"bin,omitempty"`     // resolved path (from PATH)
	Version string `json:"version,omitempty"` // first line of `<tool> --version`

	// Launch contract — the headless/unattended invocation. The conductor runs:
	//   weave start --issue N … -- <tool> <HeadlessArgs...> "<body>"
	// i.e. HeadlessArgs are passed verbatim and the issue body is appended as the
	// final prompt argument. A BARE launch (no HeadlessArgs) hangs at the tool's
	// interactive/trust prompt — that is the failure this profile prevents.
	HeadlessArgs         []string `json:"headless_args"`
	TrustClear           string   `json:"trust_clear,omitempty"` // e.g. "say:1" — how to clear a per-dir trust prompt
	SupportsSay          bool     `json:"supports_say"`          // mid-run `weave say` injection reaches it (steerable)
	SupportsGracefulQuit bool     `json:"supports_graceful_quit"`
	Notes                string   `json:"notes,omitempty"`

	// Track record — accrued per role (conductor, coder, qa, doc, …).
	Roles         map[string]*RoleRecord `json:"roles,omitempty"`
	InterviewedAt time.Time              `json:"interviewed_at,omitempty"`
	UpdatedAt     time.Time              `json:"updated_at"`
}

// RoleRecord is one tool's accrued performance in one role, plus a qualitative
// conductor/operator review (rating + one-line verdict + rationale notes).
type RoleRecord struct {
	Runs       int       `json:"runs"`
	Passed     int       `json:"passed"`
	Failed     int       `json:"failed"`
	Rating     int       `json:"rating,omitempty"`  // 0–5 suitability for this role
	Verdict    string    `json:"verdict,omitempty"` // one-line review
	Notes      []string  `json:"notes,omitempty"`   // rationale / outcome notes (capped at 10)
	ReviewedAt time.Time `json:"reviewed_at,omitempty"`
}

// setRoleReview records (or updates) a qualitative review of a tool's fitness
// for a role into its persistent profile — the system-wide track record a
// conductor consults when routing. Preserves accrued run counts.
func setRoleReview(dir, tool, role string, rating int, verdict string, notes []string, now time.Time) error {
	p, ok := loadToolProfile(dir, tool)
	if !ok {
		seed := seededLaunchContracts[tool]
		p = &seed
		p.Tool = tool
	}
	if p.Roles == nil {
		p.Roles = map[string]*RoleRecord{}
	}
	r := p.Roles[role]
	if r == nil {
		r = &RoleRecord{}
		p.Roles[role] = r
	}
	r.Rating, r.Verdict, r.ReviewedAt = rating, verdict, now
	for _, n := range notes {
		if n != "" {
			r.Notes = append(r.Notes, n)
		}
	}
	if len(r.Notes) > 10 {
		r.Notes = r.Notes[len(r.Notes)-10:]
	}
	return saveToolProfile(dir, p)
}

// seededLaunchContracts are the known-good headless invocations, kept in sync
// with the weave-orchestration skill cheat-sheet. `fleet interview` starts from
// these for a tool it has not seen, then refreshes bin/version live.
var seededLaunchContracts = map[string]ToolProfile{
	"claude": {
		HeadlessArgs: []string{"--dangerously-skip-permissions"},
		TrustClear:   "say:1", SupportsSay: true, SupportsGracefulQuit: true,
		Notes: "TUI is steerable; per-dir trust prompt needs `weave say <N> \"1\"` (or pre-seed ~/.claude.json).",
	},
	"codex": {
		HeadlessArgs: []string{"exec", "--skip-git-repo-check", "--workspace", "workspace-write"},
		SupportsSay:  false, SupportsGracefulQuit: true,
		Notes: "headless `exec`; --full-auto is deprecated and fails 'not a trusted dir'. Not steerable.",
	},
	"agy": {
		HeadlessArgs: []string{"--dangerously-skip-permissions", "--print-timeout", "40m", "-p"},
		SupportsSay:  false, SupportsGracefulQuit: true,
		Notes: "antigravity; headless `-p` (use `-i` for a steerable TUI). Bump --print-timeout (default 5m).",
	},
	"opencode": {
		HeadlessArgs: []string{"run"},
		SupportsSay:  false, SupportsGracefulQuit: true,
		Notes: "headless `run`; judge by committed artifacts, not exit code.",
	},
	"aider": {
		HeadlessArgs: []string{"--yes-always", "--no-check-update", "--message"},
		SupportsSay:  false, SupportsGracefulQuit: true,
		Notes: "headless; auto-commits. Shares opencode's DeepSeek backend — NOT an independent vote.",
	},
}

func weaveToolsDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(weaveStateRoot(home), "tools")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	return dir, nil
}

func toolProfilePath(dir, tool string) string { return filepath.Join(dir, tool+".json") }

func loadToolProfile(dir, tool string) (*ToolProfile, bool) {
	b, err := os.ReadFile(toolProfilePath(dir, tool))
	if err != nil {
		return nil, false
	}
	var p ToolProfile
	if json.Unmarshal(b, &p) != nil {
		return nil, false
	}
	return &p, true
}

func saveToolProfile(dir string, p *ToolProfile) error {
	p.UpdatedAt = time.Now()
	b, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(toolProfilePath(dir, p.Tool), b, 0o644)
}

// interviewTool resolves a tool's binary + version and records its launch
// contract into a profile (seeding from the known contracts for a first
// interview, preserving any accrued track record). This is the lightweight
// one-on-one calibration: it makes the unattended launch contract explicit and
// discoverable. (A deeper live capability run — launch on a fixed micro-task and
// gate-verify the result — is the next layer, and `fleet tournament` will rank
// tools per role using the same RoleRecord.)
func interviewTool(dir, tool string, now time.Time) (*ToolProfile, error) {
	p, ok := loadToolProfile(dir, tool)
	if !ok {
		seed := seededLaunchContracts[tool] // zero value if unknown
		p = &seed
		p.Tool = tool
	}
	if path, err := exec.LookPath(tool); err == nil {
		p.Bin = path
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		if out, verr := exec.CommandContext(ctx, path, "--version").CombinedOutput(); verr == nil {
			if line := strings.TrimSpace(strings.SplitN(string(out), "\n", 2)[0]); line != "" {
				p.Version = line
			}
		}
		cancel()
	}
	p.InterviewedAt = now
	if err := saveToolProfile(dir, p); err != nil {
		return nil, err
	}
	return p, nil
}

// recordToolOutcome appends a role outcome to a tool's track record (used by the
// conductor at converge time, and by `fleet tournament`).
func recordToolOutcome(dir, tool, role string, passed bool, note string) error {
	p, ok := loadToolProfile(dir, tool)
	if !ok {
		seed := seededLaunchContracts[tool]
		p = &seed
		p.Tool = tool
	}
	if p.Roles == nil {
		p.Roles = map[string]*RoleRecord{}
	}
	r := p.Roles[role]
	if r == nil {
		r = &RoleRecord{}
		p.Roles[role] = r
	}
	r.Runs++
	if passed {
		r.Passed++
	} else {
		r.Failed++
	}
	if note != "" {
		r.Notes = append(r.Notes, note)
		if len(r.Notes) > 10 {
			r.Notes = r.Notes[len(r.Notes)-10:]
		}
	}
	return saveToolProfile(dir, p)
}

func sortedRoleNames(p *ToolProfile) []string {
	names := make([]string, 0, len(p.Roles))
	for k := range p.Roles {
		names = append(names, k)
	}
	sort.Strings(names)
	return names
}
