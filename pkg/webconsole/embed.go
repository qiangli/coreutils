// Copyright (c) 2025 qiangli
// See LICENSE for licensing information

package webconsole

import (
	"bytes"
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"io"
	"io/fs"
	"net/http"
	"regexp"
	"strings"
	"sync"

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

// assetRef matches a local relative src=/href= in the page markup.
var assetRef = regexp.MustCompile(`(src|href)="([A-Za-z0-9_./-]+\.(?:js|css))"`)

// versionAssets stamps every local asset reference with a content version.
//
// The ETag alone is not enough to rescue a browser that ALREADY cached an asset
// back when the server sent no validator at all: with nothing to revalidate
// against it may not even ask, and it keeps running the old script past any
// number of reloads. The HTML itself is no-store, so a version in the query
// string is fetched fresh every time and pins the exact bytes the page expects.
//
// This is why a fixed bug can keep reproducing on the reporter's machine while
// every server-side check says the fix shipped.
func versionAssets(doc []byte) []byte {
	return assetRef.ReplaceAllFunc(doc, func(m []byte) []byte {
		g := assetRef.FindSubmatch(m)
		name := string(g[2])
		tag := strings.Trim(etagFor(name), `"`)
		if tag == "0" {
			return m // not an embedded asset; leave it alone
		}
		return []byte(string(g[1]) + `="` + name + `?v=` + tag + `"`)
	})
}

// etagFor returns a strong ETag over an embedded file's bytes, computed once.
//
// Keyed on content rather than a build stamp so a rebuild that does not change
// an asset still answers 304, and any change to it always busts the cache.
func etagFor(name string) string {
	etagOnce.Do(func() { etags = map[string]string{} })
	etagMu.Lock()
	defer etagMu.Unlock()
	if tag, ok := etags[name]; ok {
		return tag
	}
	tag := `"0"`
	if b, err := fs.ReadFile(spaFS, name); err == nil {
		sum := sha256.Sum256(b)
		tag = `"` + hex.EncodeToString(sum[:8]) + `"`
	}
	etags[name] = tag
	return tag
}

var (
	etagOnce sync.Once
	etagMu   sync.Mutex
	etags    map[string]string
)

// handleTermPage serves the standalone terminal page.
//
// It is served from the LAUNCHER's root rather than from under the /term mount
// so its <base href> is the launcher's: one place holds the vendored xterm
// bundle, and the page can address the socket as "term/ws" and the launcher as
// "./" without knowing how deep it is mounted.
func (s *server) handleTermPage(w http.ResponseWriter, r *http.Request) {
	s.servePageFile(w, r, "term.html")
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
				// Embedded files carry a ZERO modtime, so without an explicit
				// validator a browser has nothing to revalidate against and
				// applies heuristic caching — it keeps serving the previous
				// build's script forever. That is not a cosmetic staleness: the
				// launcher shipped a version whose tiles opened apps in an
				// iframe, and after the fix the old script kept running from
				// cache, so the bug looked unfixed.
				//
				// no-cache means "revalidate every time", not "do not store":
				// with the content ETag below an unchanged asset costs a 304.
				w.Header().Set("ETag", etagFor(name))
				w.Header().Set("Cache-Control", "no-cache")
				http.ServeFileFS(w, r, spaFS, name)
				return
			}
		}
	}
	s.servePageFile(w, r, "index.html")
}

// servePageFile writes an embedded HTML page with <base href> injected for the
// CURRENT mount.
//
// The href is computed per request, not at build time, because the same binary
// is reached at / on loopback and at /matrix/h/<host>/app/console/ through the
// tunnel — and, when the console mounts a panel, one level deeper again. The
// trailing slash is the entire mechanism: a page that builds every URL as
// new URL(x, document.baseURI) then needs no per-mount configuration.
func (s *server) servePageFile(w http.ResponseWriter, r *http.Request, name string) {
	if spaFS == nil {
		http.Error(w, "console UI missing from this build", http.StatusInternalServerError)
		return
	}
	f, err := spaFS.Open(name)
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
	_, _ = w.Write(versionAssets(injectBase(doc, coopauth.BaseHref(r))))
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
