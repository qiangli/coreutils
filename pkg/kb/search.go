package kb

import (
	"bufio"
	"encoding/json"
	"fmt"
	"hash/fnv"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"
)

// Query is one deterministic search: BM25 over weighted fields, filtered by
// activation scope. Precision over recall — the default K is small on purpose
// (the ReasoningBank ablation: retrieving more memories than apply actively
// hurts).
type Query struct {
	Terms []string
	Repo  string   // current repo basename; filters pages scoped to other repos
	OS    string   // GOOS; filters pages scoped to other OSes
	Tags  []string // require at least one matching tag when set
	K     int      // max hits (0 = DefaultK)
	All   bool     // include superseded/stale pages

	// MinCoverage gates the EMPTY RESULT. When > 0, a query whose best page
	// matches a smaller fraction of its terms than this returns nothing at
	// all. Zero (the default) keeps the historical behaviour of always
	// answering, because enabling the gate costs recall on its own: measured
	// on the real host store, coverage 0.5 took hit@3 from 100% to 86% while
	// taking abstention from 0% to 100%. Combined with a semantic tier the
	// recall cost disappears, which is why this is a knob and not a constant.
	// See dhnt/docs/memory-eval-plan.md §4g.
	MinCoverage float64

	// Use is per-slug use history (Store.UseHistory()). When set together with
	// UseWeight, ranking adds ACT-R's base-level term: a page opened often and
	// recently outranks an equally-matching page nobody has touched. Nil (the
	// default) leaves ranking purely lexical — the term is opt-in because it
	// changes what a store returns based on what its readers did, and that
	// deserves to be a caller's decision rather than a silent one.
	Use map[string]*Use
	// UseWeight scales the base-level term. 0 disables it.
	UseWeight float64
	// Now is the clock for decay; zero means time.Now(). Injectable so tests
	// and the eval harness are deterministic.
	Now time.Time
}

// DefaultK caps search results.
const DefaultK = 3

// BM25 parameters. The standard defaults; they were not tuned, and the
// measured gain comes from having length normalisation and IDF at all rather
// than from their values.
const (
	bm25K1 = 1.2
	bm25B  = 0.75
)

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

// Search ranks pages against q with BM25 over weighted fields (title 4,
// description 3, tags 3, slug 2, type 1, body 1 — applied as term repetition),
// then weights by the validation ladder (validated 1.25, candidate 1.0, stale
// 0.5). Deterministic: ties break on slug.
//
// Ranking is by SCORE ALONE, not by match count first. The substring scorer
// this replaced sorted on distinct-terms-matched before score, which is right
// when scores are raw occurrence counts and wrong for BM25: the IDF sum already
// rewards covering more of the query, so a hard tie-break on count overrides a
// better-scored page. Measured on the real host store: match-count-first gives
// MRR 0.881 / nDCG@3 0.906, score-only gives 0.929 / 0.942 at identical hit@3.
// Hit.Matched is still reported — callers use it, and it is the coverage input
// to MinCoverage — it just no longer dominates the order.
//
// Why BM25 and not the substring scorer this replaced: the substring version
// counted raw occurrences with no length normalisation and matched INSIDE
// words, which produced three measured defects — long pages became attractors
// for unrelated queries (on a 36-page store the top hit for a query about
// nothing was in the 87th percentile by length), a term could score inside an
// unrelated word ("rust" inside "trust"), and a rarer term counted the same as
// a ubiquitous one. IDF is also exactly the fan penalty a spreading-activation
// model asks for: a term carried by many records contributes less.
//
// Measured on the real host store (dhnt/docs/memory-eval-plan.md §4c):
// hit@3 93% -> 100%, MRR 0.821 -> 0.929, nDCG@3 0.849 -> 0.942, tokens
// 417 -> 117. On paraphrased queries +43 points; the shipped substring scorer
// was already at 100% on queries that use a page's own vocabulary, so the gain
// is entirely on the queries a person actually types.
func Search(pages []*Page, q Query) []Hit {
	k := q.K
	if k <= 0 {
		k = DefaultK
	}

	// Eligible set first: BM25's IDF and average length are corpus statistics,
	// so they must be computed over the pages that could actually be returned.
	// Including filtered-out pages would let another OS's records change this
	// OS's ranking.
	var elig []*Page
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
		elig = append(elig, p)
	}
	if len(elig) == 0 {
		return nil
	}

	terms := queryTerms(q.Terms)
	tf := make([]map[string]int, len(elig))
	dl := make([]float64, len(elig))
	var total float64
	for i, p := range elig {
		tf[i] = map[string]int{}
		for _, tok := range pageTokens(p) {
			tf[i][tok]++
			dl[i]++
		}
		total += dl[i]
	}
	avgdl := total / float64(len(elig))
	if avgdl == 0 {
		avgdl = 1
	}

	df := make(map[string]int, len(terms))
	for _, t := range terms {
		for i := range elig {
			if tf[i][t] > 0 {
				df[t]++
			}
		}
	}

	n := float64(len(elig))
	var hits []Hit
	var bestCoverage float64
	for i, p := range elig {
		var score float64
		matched := 0
		for _, t := range terms {
			f := float64(tf[i][t])
			if f == 0 {
				continue
			}
			matched++
			idf := math.Log(1 + (n-float64(df[t])+0.5)/(float64(df[t])+0.5))
			score += idf * (f * (bm25K1 + 1)) / (f + bm25K1*(1-bm25B+bm25B*dl[i]/avgdl))
		}
		if len(terms) > 0 {
			if cov := float64(matched) / float64(len(terms)); cov > bestCoverage {
				bestCoverage = cov
			}
		}
		if len(terms) > 0 && matched == 0 {
			continue
		}
		score *= statusWeight(p.Status)
		if q.UseWeight != 0 && q.Use != nil {
			if u := q.Use[p.Slug]; u != nil {
				now := q.Now
				if now.IsZero() {
					now = time.Now()
				}
				score += q.UseWeight * u.BaseLevel(now, DefaultDecay)
			}
		}
		hits = append(hits, Hit{Page: p, Score: score, Matched: matched})
	}

	// The empty-result path. A page matching one of four query terms has not
	// answered the question, it has recognised one incidental word.
	if q.MinCoverage > 0 && len(terms) > 0 && bestCoverage < q.MinCoverage {
		return nil
	}

	sort.SliceStable(hits, func(i, j int) bool {
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

// queryTerms normalises caller-supplied terms the same way pageTokens
// normalises page text — word boundaries, lowercase, stopwords dropped — so a
// term can never match inside an unrelated word. Duplicates collapse: asking
// twice is not evidence.
func queryTerms(raw []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, r := range raw {
		for _, tok := range tokenize(r) {
			if !seen[tok] {
				seen[tok] = true
				out = append(out, tok)
			}
		}
	}
	return out
}

// pageTokens is the weighted token bag: field weight = how many times the
// field's tokens are repeated, which is how field weighting composes with
// BM25's term-frequency saturation without a second scoring pass.
func pageTokens(p *Page) []string {
	var out []string
	add := func(text string, weight int) {
		toks := tokenize(text)
		for i := 0; i < weight; i++ {
			out = append(out, toks...)
		}
	}
	add(p.Title, 4)
	add(p.Description, 3)
	add(strings.Join(p.Tags, " "), 3)
	add(strings.ReplaceAll(p.Slug, "-", " "), 2)
	add(p.Type, 1)
	add(p.Body, 1)
	return out
}

// tokenize splits on non-alphanumeric runs, lowercases, drops tokens shorter
// than 3 runes, and drops stopwords. The stopword list exists because a
// natural-language query ("how do I stop a stuck process") is mostly words
// that carry no retrieval signal but do carry BM25 weight.
func tokenize(text string) []string {
	var out []string
	for _, f := range strings.FieldsFunc(strings.ToLower(text), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	}) {
		if len(f) >= 3 && !stopword(f) {
			out = append(out, f)
		}
	}
	return out
}

var stopwords = map[string]bool{
	"the": true, "and": true, "for": true, "are": true, "but": true, "not": true,
	"you": true, "all": true, "can": true, "has": true, "had": true, "was": true,
	"its": true, "out": true, "how": true, "why": true, "who": true, "what": true,
	"when": true, "does": true, "did": true, "with": true, "from": true, "into": true,
	"onto": true, "that": true, "this": true, "there": true, "then": true, "than": true,
	"they": true, "them": true, "will": true, "would": true, "should": true,
	"could": true, "about": true, "after": true, "before": true, "over": true,
	"under": true, "very": true, "some": true, "any": true, "our": true, "your": true,
	"their": true, "his": true, "her": true, "one": true, "two": true, "use": true,
	"used": true, "using": true, "make": true, "makes": true, "made": true,
	"get": true, "gets": true, "got": true, "see": true, "say": true, "says": true,
	"way": true, "per": true, "via": true, "off": true, "yes": true, "also": true,
	"only": true, "just": true, "even": true, "much": true, "many": true,
	"need": true, "needs": true, "most": true, "more": true,
}

func stopword(w string) bool { return stopwords[w] }

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
