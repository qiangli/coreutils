package kb

import (
	"bufio"
	"encoding/json"
	"fmt"
	"hash/fnv"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// Query is one deterministic search: substring terms ranked over weighted
// fields, filtered by activation scope. Precision over recall — the default
// K is small on purpose (the ReasoningBank ablation: retrieving more
// memories than apply actively hurts).
type Query struct {
	Terms []string
	Repo  string   // current repo basename; filters pages scoped to other repos
	OS    string   // GOOS; filters pages scoped to other OSes
	Tags  []string // require at least one matching tag when set
	K     int      // max hits (0 = DefaultK)
	All   bool     // include superseded/stale pages
}

// DefaultK caps search results.
const DefaultK = 3

// Terms distills free text (an issue title, a session goal) into search
// terms: lowercased words, punctuation-trimmed, short stopword-ish tokens
// dropped — the shared tokenizer for launchers that query kb on behalf of
// an agent.
func Terms(text string) []string {
	var out []string
	for f := range strings.FieldsSeq(strings.ToLower(text)) {
		f = strings.Trim(f, ".,;:!?()[]{}'\"`")
		if len(f) >= 3 {
			out = append(out, f)
		}
	}
	return out
}

// Hit is one scored search result.
type Hit struct {
	Page    *Page
	Score   float64
	Matched int // how many query terms matched
}

// Search ranks pages against q. Scoring is substring-per-term over weighted
// fields (title 4, description 3, tags 3, slug 2, type 1, body 1), summed,
// then weighted by the validation ladder (validated 1.25, candidate 1.0,
// stale 0.5). Pages matching more distinct terms always rank above pages
// matching fewer. Deterministic: ties break on slug.
func Search(pages []*Page, q Query) []Hit {
	k := q.K
	if k <= 0 {
		k = DefaultK
	}
	var hits []Hit
	for _, p := range pages {
		if p.Status == StatusSuperseded && !q.All {
			continue
		}
		if !scopeMatches(p, q.Repo, q.OS) {
			continue
		}
		if len(q.Tags) > 0 && !hasAnyTag(p, q.Tags) {
			continue
		}
		score, matched := scorePage(p, q.Terms)
		if len(q.Terms) > 0 && matched == 0 {
			continue
		}
		score *= statusWeight(p.Status)
		hits = append(hits, Hit{Page: p, Score: score, Matched: matched})
	}
	sort.SliceStable(hits, func(i, j int) bool {
		if hits[i].Matched != hits[j].Matched {
			return hits[i].Matched > hits[j].Matched
		}
		if hits[i].Score != hits[j].Score {
			return hits[i].Score > hits[j].Score
		}
		return hits[i].Page.Slug < hits[j].Page.Slug
	})
	if len(hits) > k {
		hits = hits[:k]
	}
	return hits
}

func statusWeight(status string) float64 {
	switch status {
	case StatusValidated:
		return 1.25
	case StatusStale:
		return 0.5
	default:
		return 1.0
	}
}

func scopeMatches(p *Page, repo, goos string) bool {
	if p.Scope == nil {
		return true
	}
	if len(p.Scope.Repos) > 0 && repo != "" {
		found := false
		for _, r := range p.Scope.Repos {
			if strings.EqualFold(r, repo) {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	if p.Scope.OS != "" && goos != "" && !strings.EqualFold(p.Scope.OS, goos) {
		return false
	}
	return true
}

func hasAnyTag(p *Page, tags []string) bool {
	for _, want := range tags {
		for _, have := range p.Tags {
			if strings.EqualFold(have, want) {
				return true
			}
		}
	}
	return false
}

func scorePage(p *Page, terms []string) (score float64, matched int) {
	title := strings.ToLower(p.Title)
	desc := strings.ToLower(p.Description)
	body := strings.ToLower(p.Body)
	slug := strings.ToLower(p.Slug)
	typ := strings.ToLower(p.Type)
	var tags []string
	for _, t := range p.Tags {
		tags = append(tags, strings.ToLower(t))
	}
	for _, raw := range terms {
		term := strings.ToLower(strings.TrimSpace(raw))
		if term == "" {
			continue
		}
		s := 0.0
		if strings.Contains(title, term) {
			s += 4
		}
		if strings.Contains(desc, term) {
			s += 3
		}
		for _, t := range tags {
			if strings.Contains(t, term) {
				s += 3
				break
			}
		}
		if strings.Contains(slug, term) {
			s += 2
		}
		if typ == term {
			s += 1
		}
		if strings.Contains(body, term) {
			s += 1
		}
		if s > 0 {
			matched++
			score += s
		}
	}
	return score, matched
}

// --- federated read bridge -----------------------------------------------
//
// kb is the host ring; the repo rings stay where they are. --federate adds
// read-only hits from the CURRENT repo's contribution log
// (.agents/bashy/graph/contrib.jsonl) and its weave campaign memory
// (~/.bashy/weave/<base>-<fnv32a>/memory.jsonl) so an agent gets all three
// rings from one call without any store migration.

// FedHit is one federated (non-kb-page) result.
type FedHit struct {
	Origin string `json:"origin"` // repo-graph | weave-memory
	Text   string `json:"text"`
}

// FederatedSearch matches terms against the repo-scoped stores for the repo
// containing cwd. Missing stores are silently empty — the bridge is
// best-effort by design.
func FederatedSearch(cwd string, terms []string, k int) []FedHit {
	if k <= 0 {
		k = DefaultK
	}
	root := repoRootOf(cwd)
	if root == "" {
		return nil
	}
	var out []FedHit
	out = append(out, contribHits(root, terms, k)...)
	out = append(out, weaveMemoryHits(root, terms, k)...)
	return out
}

// repoRootOf walks up to the nearest .git (same rule as the contrib store,
// so all agents anywhere in a repo see the same ring). "" when not in a repo.
func repoRootOf(start string) string {
	dir := start
	for {
		if _, err := os.Stat(filepath.Join(dir, ".git")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return ""
		}
		dir = parent
	}
}

// contribHits replays the repo contribution log (forgets applied,
// last-writer-wins per id — the bashy-graph-contrib-v1 envelope) and
// substring-matches terms against live notes/observations.
func contribHits(repoRoot string, terms []string, k int) []FedHit {
	type rec struct {
		ID            string `json:"id"`
		Op            string `json:"op"`
		Target        string `json:"target"`
		Text          string `json:"text"`
		Relation      string `json:"relation"`
		Dst           string `json:"dst"`
		Kind          string `json:"kind"`
		Outcome       string `json:"outcome"`
		ForgetID      string `json:"forget_id"`
		ForgetTarget  string `json:"forget_target"`
		ForgetEpisode string `json:"forget_episode"`
		Episode       string `json:"episode"`
	}
	path := filepath.Join(repoRoot, ".agents", "bashy", "graph", "contrib.jsonl")
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()
	var all []rec
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		var r rec
		if json.Unmarshal([]byte(line), &r) != nil {
			continue
		}
		all = append(all, r)
	}
	if sc.Err() != nil {
		return nil
	}
	var forgets []rec
	latest := map[string]rec{}
	var order []string
	for _, r := range all {
		if r.Op == "forget" {
			forgets = append(forgets, r)
			continue
		}
		if _, seen := latest[r.ID]; !seen {
			order = append(order, r.ID)
		}
		latest[r.ID] = r
	}
	dead := func(r rec) bool {
		for _, f := range forgets {
			if (f.ForgetID != "" && f.ForgetID == r.ID) ||
				(f.ForgetTarget != "" && f.ForgetTarget == r.Target) ||
				(f.ForgetEpisode != "" && f.ForgetEpisode == r.Episode) {
				return true
			}
		}
		return false
	}
	var out []FedHit
	for _, id := range order {
		if len(out) >= k {
			break
		}
		r := latest[id]
		if dead(r) {
			continue
		}
		var text string
		switch r.Op {
		case "note":
			text = "note " + r.Target + ": " + r.Text
		case "link":
			text = "link " + r.Target + " " + r.Relation + " " + r.Dst
		case "observe":
			text = "observe " + r.Kind + "/" + r.Outcome + " " + r.Target + ": " + r.Text
		default:
			continue
		}
		if matchesAny(text, terms) {
			out = append(out, FedHit{Origin: "repo-graph", Text: text})
		}
	}
	return out
}

// weaveMemoryHits reads the repo's weave campaign memory. The queue dir tag
// mirrors weaveQueueDir: <base>-<fnv32a(repoRoot) hex> under ~/.bashy/weave.
func weaveMemoryHits(repoRoot string, terms []string, k int) []FedHit {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil
	}
	h := fnv.New32a()
	_, _ = h.Write([]byte(repoRoot))
	tag := fmt.Sprintf("%s-%08x", filepath.Base(repoRoot), h.Sum32())
	path := filepath.Join(home, ".bashy", "weave", tag, "memory.jsonl")
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()
	type obs struct {
		IssueID          int64    `json:"issue_id"`
		Title            string   `json:"title"`
		Tool             string   `json:"tool"`
		Outcome          string   `json:"outcome"`
		Summary          string   `json:"summary"`
		FailedApproaches []string `json:"failed_approaches"`
		Tags             []string `json:"tags"`
	}
	var out []FedHit
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	for sc.Scan() {
		if len(out) >= k {
			break
		}
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		var o obs
		if json.Unmarshal([]byte(line), &o) != nil {
			continue
		}
		blob := strings.Join(append([]string{o.Title, o.Summary, strings.Join(o.Tags, " ")}, o.FailedApproaches...), " ")
		if matchesAny(blob, terms) {
			text := "issue #" + strconv.FormatInt(o.IssueID, 10) + " " + o.Outcome
			if o.Tool != "" {
				text += " [" + o.Tool + "]"
			}
			if o.Title != "" {
				text += " " + o.Title
			}
			if o.Summary != "" {
				text += ": " + o.Summary
			}
			out = append(out, FedHit{Origin: "weave-memory", Text: text})
		}
	}
	if sc.Err() != nil {
		return nil
	}
	return out
}

func matchesAny(text string, terms []string) bool {
	if len(terms) == 0 {
		return true
	}
	low := strings.ToLower(text)
	for _, t := range terms {
		t = strings.ToLower(strings.TrimSpace(t))
		if t != "" && strings.Contains(low, t) {
			return true
		}
	}
	return false
}
