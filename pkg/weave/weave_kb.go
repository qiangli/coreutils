package weave

// The host-kb bridge: weave does the check-before-task step FOR the worker.
// At spawn, the top host-kb pages matching the issue are dropped into the
// workspace as KB.md (beside WEAVE_MEMORY.md — memory is this repo's run
// history, kb is the host-wide wiki across all repos and all agent tools),
// and KB.md carries the write-back instruction so the retro half of the
// loop reaches every fleet CLI without any per-tool integration. Best
// effort throughout: a missing or empty kb never blocks a launch.

import (
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"

	"github.com/qiangli/coreutils/pkg/kb"
)

// weaveKBFileName is the workspace drop (gitignored via .git/info/exclude,
// like WEAVE_MEMORY.md — it must never merge).
const weaveKBFileName = "KB.md"

// weaveKBBodyCap keeps the injected page bodies token-lean; the worker can
// `bashy kb show <slug>` for the rest (progressive disclosure).
const weaveKBBodyCap = 400

// weaveInjectKBFile writes KB.md into the workspace: the top host-kb
// matches for this issue plus the retro write-back instruction.
func weaveInjectKBFile(dir, workspace string, it *weaveItem) error {
	if it == nil || workspace == "" {
		return nil
	}
	store := kb.Open("")
	pages, err := store.List()
	if err != nil {
		return err
	}
	q := kb.Query{
		Terms: kb.Terms(it.Title),
		Repo:  weaveRepoNameFromQueueDir(dir),
		OS:    runtime.GOOS,
	}
	var hits []kb.Hit
	if len(pages) > 0 {
		hits = kb.Search(pages, q)
	}
	var b strings.Builder
	b.WriteString("# KB — host knowledge base (shared by all agents on this host, across repos)\n\n")
	if len(hits) == 0 {
		// "Nothing matched" is not the same as "nothing is here", and the
		// worker cannot tell the difference from a bare sentence. The search
		// ran on the issue TITLE alone, so a miss is as likely to be the
		// query's wording as the corpus's silence — the report says which,
		// and hands over the vocabulary to re-ask with.
		if len(pages) > 0 {
			b.WriteString("This search used the issue title only, and nothing matched:\n\n```\n")
			b.WriteString(kb.Diagnose(pages, q).Text())
			b.WriteString("```\n\nRe-ask in the kb's own words before assuming it is empty: `bashy kb search <query>`.\n")
			b.WriteString("If the work still teaches something durable, contribute it when you finish:\n")
		} else {
			b.WriteString("The host kb is empty. If the work teaches something durable, contribute it when you finish:\n")
		}
	} else {
		b.WriteString("Check these before you start — they may save you a failed approach:\n\n")
		b.WriteString(kb.Renderer{
			Resolution: kb.ResFull, Bullet: "- ", Sep: " ", BodyCap: weaveKBBodyCap,
		}.Hits(hits))
		b.WriteString("\nMore: `bashy kb search <query>` (or `bashy kb show <slug>`).\n")
	}
	b.WriteString("\nAFTER this issue is done, close the loop: `bashy kb retro <a few words on what you did>`\n")
	b.WriteString("— validate/update/supersede what you consulted, or add the new lesson (distilled, never a transcript). NOOP is fine when nothing durable was learned.\n")
	if err := os.WriteFile(filepath.Join(workspace, weaveKBFileName), []byte(b.String()), 0o644); err != nil {
		return err
	}
	return weaveExcludeWorkspaceFile(workspace, weaveKBFileName)
}

// weaveRepoNameFromQueueDir recovers the repo basename from the queue dir
// tag (<base>-<8-hex fnv32a>), for kb's repo-scope filter.
var weaveQueueTagHash = regexp.MustCompile(`-[0-9a-f]{8}$`)

func weaveRepoNameFromQueueDir(dir string) string {
	return weaveQueueTagHash.ReplaceAllString(filepath.Base(dir), "")
}
