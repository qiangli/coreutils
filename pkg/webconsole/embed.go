// Copyright (c) 2025 qiangli
// See LICENSE for licensing information

package webconsole

import (
	"bytes"
	"embed"
	"io"
	"io/fs"
	"net/http"
	"strings"

	"github.com/qiangli/coreutils/pkg/coopauth"
)

// The start page is embedded UNCONDITIONALLY — no build tag.
//
// pkg/meet keeps its SPA behind -tags meetspa and leaves web/dist untracked;
// the console cannot, because the console is meant to be ALWAYS available, and
// with no tag to hide behind an unbuilt bundle would be a COMPILE error for
// everyone rather than a missing page for the few.
//
// So the embedded bytes live in artifact/, which is TRACKED, while the SPA's own
// build output (web/dist/) stays ignored like meet's. The two are deliberately
// different directories: dist/ is scratch that every local build churns, and
// artifact/ is "these bytes ship" — promoted into the tree by the build script
// as a reviewable act rather than as a side effect of whoever last ran vite.
//
// The cost of tracking is the mirror image of the build break it prevents: a
// stale bundle ships silently. That is what the release verification gate exists
// to catch.
//
//go:embed all:artifact
var spaEmbed embed.FS

var spaFS fs.FS

func init() {
	if sub, err := fs.Sub(spaEmbed, "artifact"); err == nil {
		spaFS = sub
	}
}

// handleSPA serves the start page, falling back to index.html so client-side
// routes survive a reload.
func (s *server) handleSPA(w http.ResponseWriter, r *http.Request) {
	if spaFS == nil {
		http.Error(w, "console UI missing from this build", http.StatusInternalServerError)
		return
	}
	name := strings.TrimPrefix(r.URL.Path, "/")
	if name != "" {
		if f, err := spaFS.Open(name); err == nil {
			defer f.Close()
			if st, err := f.Stat(); err == nil && !st.IsDir() {
				http.ServeFileFS(w, r, spaFS, name)
				return
			}
		}
	}
	s.serveIndex(w, r)
}

// serveIndex writes index.html with <base href> injected for the CURRENT mount.
//
// The href is computed per request, not at build time, because the same binary
// is reached at / on loopback and at /matrix/h/<host>/app/console/ through the
// tunnel — and, when the console mounts a panel, one level deeper again. The
// trailing slash is the entire mechanism: a page that builds every URL as
// new URL(x, document.baseURI) then needs no per-mount configuration.
func (s *server) serveIndex(w http.ResponseWriter, r *http.Request) {
	f, err := spaFS.Open("index.html")
	if err != nil {
		http.Error(w, "console UI missing from this build", http.StatusInternalServerError)
		return
	}
	defer f.Close()
	doc, err := io.ReadAll(f)
	if err != nil {
		http.Error(w, "console UI unreadable", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write(injectBase(doc, coopauth.BaseHref(r)))
}

// injectBase rewrites (or inserts) the document's <base href>.
func injectBase(doc []byte, href string) []byte {
	if i := bytes.Index(doc, []byte("<base href=\"")); i >= 0 {
		start := i + len("<base href=\"")
		if end := bytes.IndexByte(doc[start:], '"'); end >= 0 {
			out := make([]byte, 0, len(doc)+len(href))
			out = append(out, doc[:start]...)
			out = append(out, href...)
			out = append(out, doc[start+end:]...)
			return out
		}
	}
	if i := bytes.Index(doc, []byte("<head>")); i >= 0 {
		at := i + len("<head>")
		tag := "\n    <base href=\"" + href + "\">"
		out := make([]byte, 0, len(doc)+len(tag))
		out = append(out, doc[:at]...)
		out = append(out, tag...)
		out = append(out, doc[at:]...)
		return out
	}
	return doc
}
